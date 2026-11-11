package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"iperf3-gui/internal/history"
	"iperf3-gui/internal/iperf"
	"iperf3-gui/internal/logging"
	"iperf3-gui/internal/model"
)

const (
	rawOutputLimit  = 256 * 1024
	stderrLimit     = 4 * 1024
	chartPointLimit = 300
	serverLogLimit  = 500
)

type Emitter func(event string, data ...interface{})

type Manager struct {
	mu           sync.Mutex
	enginePath   string
	capabilities model.EngineCapabilities
	store        *history.Store
	logger       *logging.Logger
	emit         Emitter
	runs         map[string]*clientRun
	activeRunID  string
	servers      map[string]*serverRun
	serverKeys   map[string]string
	closed       bool
}

type clientRun struct {
	mu              sync.Mutex
	id              string
	config          model.ClientConfig
	status          model.RunStatus
	startedAt       time.Time
	endedAt         time.Time
	sequence        int
	latest          []model.Interval
	summary         *model.Summary
	exitCode        *int
	appErr          *model.AppError
	message         string
	raw             strings.Builder
	rawTruncated    bool
	stderr          strings.Builder
	stderrTruncated bool
	parseErrors     int
	cancel          context.CancelFunc
	cmd             *exec.Cmd
	stopped         bool
	historyID       string
	pidFile         string
	outputMode      string
	parserVersion   string
	done            chan struct{}
}

func New(enginePath string, capabilities model.EngineCapabilities, store *history.Store, emit Emitter, loggers ...*logging.Logger) *Manager {
	if emit == nil {
		emit = func(string, ...interface{}) {}
	}
	var logger *logging.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &Manager{
		enginePath:   enginePath,
		capabilities: capabilities,
		store:        store,
		logger:       logger,
		emit:         emit,
		runs:         make(map[string]*clientRun),
		servers:      make(map[string]*serverRun),
		serverKeys:   make(map[string]string),
	}
}

func (m *Manager) logf(format string, args ...interface{}) {
	if m.logger != nil {
		m.logger.Printf(format, args...)
	}
}

func (m *Manager) StartClientTest(cfg model.ClientConfig) model.StartRunResult {
	if appErr := iperf.ValidateClientConfig(&cfg); appErr != nil {
		return model.StartRunResult{Error: appErr}
	}
	if !iperf.DirectionSupported(m.capabilities, cfg.Protocol, cfg.Direction) {
		return model.StartRunResult{Error: iperf.UnsupportedDirectionError(cfg.Protocol)}
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return model.StartRunResult{Error: &model.AppError{
			Code: model.ErrStateConflict, Stage: "session", Message: "应用正在退出，无法启动新测试",
		}}
	}
	if m.activeRunID != "" {
		m.mu.Unlock()
		return model.StartRunResult{Error: &model.AppError{
			Code: model.ErrRunAlreadyActive, Stage: "session", Message: "已有测试正在运行，请先停止或等待结束",
		}}
	}
	runID := newID()
	now := time.Now()
	pidFile := m.newPIDFile(runID)
	run := &clientRun{
		id:            runID,
		config:        cfg,
		status:        model.RunStatusStarting,
		startedAt:     now,
		historyID:     runID,
		pidFile:       pidFile,
		outputMode:    outputMode(m.capabilities),
		parserVersion: iperf.ParserVersion,
		done:          make(chan struct{}),
	}
	m.runs[runID] = run
	m.activeRunID = runID
	m.mu.Unlock()

	m.logf(
		"run %s start host=%s port=%d protocol=%s direction=%s duration=%ds interval=%ds streams=%d",
		runID, cfg.Host, cfg.Port, cfg.Protocol, cfg.Direction,
		cfg.DurationSeconds, cfg.IntervalSeconds, cfg.ParallelStreams,
	)

	if m.store != nil {
		record := model.RunRecord{
			ID:            runID,
			CreatedAt:     formatTime(now),
			StartedAt:     formatTime(now),
			Status:        model.RunStatusStarting,
			Protocol:      cfg.Protocol,
			Direction:     cfg.Direction,
			Host:          cfg.Host,
			Port:          cfg.Port,
			Config:        cfg,
			OutputMode:    run.outputMode,
			ParserVersion: run.parserVersion,
		}
		if err := m.store.CreateRun(record); err != nil {
			run.mu.Lock()
			run.historyID = ""
			run.message = "历史记录写入失败，本次结果仍会显示但不会保存"
			run.mu.Unlock()
			m.logf("run %s history create failed: %v", runID, err)
		}
	}

	go m.runClient(run)
	return model.StartRunResult{RunID: runID}
}

func (m *Manager) StopClientTest(runID string) model.StopResult {
	run := m.getRun(runID)
	if run == nil {
		return model.StopResult{Error: &model.AppError{
			Code: model.ErrRunNotFound, Stage: "session", Message: "测试会话不存在",
		}}
	}

	run.mu.Lock()
	if isTerminal(run.status) {
		status := run.status
		run.mu.Unlock()
		return model.StopResult{Status: status}
	}
	run.stopped = true
	run.status = model.RunStatusStopping
	sequence := run.nextSequenceLocked()
	event := model.RunStateEvent{
		RunID:     run.id,
		Sequence:  sequence,
		Status:    model.RunStatusStopping,
		StartedAt: formatTime(run.startedAt),
		Message:   "正在停止测试",
	}
	cancel := run.cancel
	cmd := run.cmd
	pidFile := run.pidFile
	done := run.done
	run.mu.Unlock()

	m.emit("run:state", event)
	m.logf("run %s stop requested", run.id)
	if err := m.interruptProcess(pidFile); err != nil {
		m.logf("run %s graceful stop failed: %v", run.id, err)
		if cancel != nil {
			cancel()
		}
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	} else {
		go m.fallbackStop(done, cancel, cmd, 3*time.Second)
	}
	return model.StopResult{Status: model.RunStatusStopping}
}

func (m *Manager) GetRunSnapshot(runID string) model.RunSnapshot {
	run := m.getRun(runID)
	if run == nil {
		return model.RunSnapshot{
			RunID: runID,
			Error: &model.AppError{Code: model.ErrRunNotFound, Stage: "session", Message: "测试会话不存在"},
		}
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	snapshot := model.RunSnapshot{
		RunID:           run.id,
		Status:          run.status,
		Config:          run.config,
		StartedAt:       formatTime(run.startedAt),
		EndedAt:         formatTime(run.endedAt),
		LastSequence:    run.sequence,
		Summary:         copySummary(run.summary),
		ExitCode:        copyInt(run.exitCode),
		Error:           copyAppError(run.appErr),
		Message:         run.message,
		RawOutput:       run.raw.String(),
		RawTruncated:    run.rawTruncated,
		StderrSummary:   run.stderr.String(),
		StderrTruncated: run.stderrTruncated,
		HistoryID:       run.historyID,
	}
	if len(run.latest) > 0 {
		start := len(run.latest) - chartPointLimit
		if start < 0 {
			start = 0
		}
		snapshot.LatestIntervals = append([]model.Interval(nil), run.latest[start:]...)
	}
	return snapshot
}

func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	m.closed = true
	runs := make([]*clientRun, 0, len(m.runs))
	for _, run := range m.runs {
		runs = append(runs, run)
	}
	servers := make([]*serverRun, 0, len(m.servers))
	for _, server := range m.servers {
		servers = append(servers, server)
	}
	m.mu.Unlock()
	m.logf("manager shutdown runs=%d servers=%d", len(runs), len(servers))

	for _, run := range runs {
		m.stopRunForShutdown(run)
	}
	for _, server := range servers {
		m.stopServerForShutdown(server)
	}
}

func (m *Manager) stopRunForShutdown(run *clientRun) {
	run.mu.Lock()
	if isTerminal(run.status) {
		run.mu.Unlock()
		return
	}
	run.stopped = true
	run.status = model.RunStatusStopping
	cancel := run.cancel
	cmd := run.cmd
	pidFile := run.pidFile
	done := run.done
	run.mu.Unlock()

	if err := m.interruptProcess(pidFile); err == nil {
		select {
		case <-done:
			return
		case <-time.After(3 * time.Second):
		}
	}
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
}

func (m *Manager) newPIDFile(id string) string {
	dir := filepath.Join(os.TempDir(), "iperf3-gui")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		m.logf("create pid directory failed: %v", err)
		return ""
	}
	path := filepath.Join(dir, id+".pid")
	_ = os.Remove(path)
	return path
}

func (m *Manager) interruptProcess(pidFile string) error {
	if pidFile == "" {
		return fmt.Errorf("pid 文件不可用")
	}
	pid, err := waitForPIDFile(pidFile, 750*time.Millisecond)
	if err != nil {
		return err
	}
	killPath := filepath.Join(filepath.Dir(m.enginePath), "kill.exe")
	if _, err := os.Stat(killPath); err != nil {
		return fmt.Errorf("kill.exe 不可用: %w", err)
	}
	cmd := exec.Command(killPath, "-INT", pid)
	cmd.Dir = filepath.Dir(killPath)
	iperf.ConfigureProcess(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kill -INT %s 失败: %w (%s)", pid, err, strings.TrimSpace(string(output)))
	}
	m.logf("interrupt pid=%s file=%s", pid, pidFile)
	return nil
}

func waitForPIDFile(path string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid := strings.TrimSpace(string(data))
			if pid != "" {
				if _, err := strconv.Atoi(pid); err == nil {
					return pid, nil
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return "", fmt.Errorf("等待 pid 文件超时: %s", path)
}

func (m *Manager) fallbackStop(done <-chan struct{}, cancel context.CancelFunc, cmd *exec.Cmd, timeout time.Duration) {
	select {
	case <-done:
		return
	case <-time.After(timeout):
	}
	m.logf("graceful stop timeout, killing process")
	if cancel != nil {
		cancel()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (m *Manager) getRun(runID string) *clientRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runs[runID]
}

func (m *Manager) emitEvent(event string, data ...interface{}) {
	m.emit(event, data...)
}

func (m *Manager) finishActiveRun(runID string) {
	m.mu.Lock()
	if m.activeRunID == runID {
		m.activeRunID = ""
	}
	m.mu.Unlock()
}

func (r *clientRun) nextSequenceLocked() int {
	r.sequence++
	return r.sequence
}

func outputMode(capabilities model.EngineCapabilities) string {
	if capabilities.JSONStream {
		return "json-stream"
	}
	return "json-final"
}

func isTerminal(status model.RunStatus) bool {
	switch status {
	case model.RunStatusStopped, model.RunStatusCompleted, model.RunStatusFailed, model.RunStatusInterrupted:
		return true
	default:
		return false
	}
}

func newID() string {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err == nil {
		return hex.EncodeToString(buffer[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func copyInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func copySummary(value *model.Summary) *model.Summary {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.Upload != nil {
		upload := *value.Upload
		cloned.Upload = &upload
	}
	if value.Download != nil {
		download := *value.Download
		cloned.Download = &download
	}
	return &cloned
}

func copyAppError(value *model.AppError) *model.AppError {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
