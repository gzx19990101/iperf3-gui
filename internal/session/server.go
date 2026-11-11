package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"iperf3-gui/internal/iperf"
	"iperf3-gui/internal/model"
)

var acceptedConnectionPattern = regexp.MustCompile(`Accepted connection from (.+), port (\d+)`)

type serverRun struct {
	mu                sync.Mutex
	id                string
	config            model.ServerConfig
	key               string
	status            model.RunStatus
	startedAt         time.Time
	endedAt           time.Time
	sequence          int
	logs              []string
	connections       []model.ServerConnection
	currentTest       *model.ServerTestSnapshot
	currentConnection int
	testSequence      int
	cancel            context.CancelFunc
	cmd               *exec.Cmd
	stopped           bool
	done              chan struct{}
	pidFile           string
	exitCode          *int
	appErr            *model.AppError
	message           string
}

func (m *Manager) StartServer(cfg model.ServerConfig) model.StartServerResult {
	if appErr := iperf.ValidateServerConfig(&cfg); appErr != nil {
		return model.StartServerResult{Error: appErr}
	}

	bind := strings.TrimSpace(cfg.BindAddress)
	key := normalizedBind(bind) + "|" + strconv.Itoa(cfg.Port)

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return model.StartServerResult{Error: &model.AppError{
			Code: model.ErrStateConflict, Stage: "session", Message: "应用正在退出，无法启动服务端",
		}}
	}
	if existing := m.serverKeys[key]; existing != "" {
		m.mu.Unlock()
		return model.StartServerResult{Error: &model.AppError{
			Code: model.ErrProcessPortInUse, Stage: "spawn", Message: "该绑定地址和端口已有服务端在监听",
		}}
	}
	serverID := newID()
	now := time.Now()
	pidFile := m.newPIDFile(serverID)
	server := &serverRun{
		id:                serverID,
		config:            cfg,
		key:               key,
		status:            model.RunStatusStarting,
		startedAt:         now,
		done:              make(chan struct{}),
		pidFile:           pidFile,
		currentConnection: -1,
	}
	m.servers[serverID] = server
	m.serverKeys[key] = serverID
	m.mu.Unlock()
	m.logf("server %s start port=%d bind=%s oneOff=%v", serverID, cfg.Port, cfg.BindAddress, cfg.OneOff)

	if err := checkPortAvailable(bind, cfg.Port); err != nil {
		m.removeServer(server)
		m.logf("server %s port check failed: %v", serverID, err)
		return model.StartServerResult{Error: &model.AppError{
			Code:    model.ErrProcessPortInUse,
			Stage:   "spawn",
			Message: "无法监听指定端口，端口可能已被占用",
			Detail:  err.Error(),
		}}
	}

	m.emitServerState(server)
	go m.runServer(server)
	return model.StartServerResult{ServerID: serverID}
}

func (m *Manager) StopServer(serverID string) model.StopResult {
	server := m.getServer(serverID)
	if server == nil {
		return model.StopResult{Error: &model.AppError{
			Code: model.ErrRunNotFound, Stage: "session", Message: "服务端会话不存在",
		}}
	}

	server.mu.Lock()
	if isTerminal(server.status) {
		status := server.status
		server.mu.Unlock()
		return model.StopResult{Status: status}
	}
	server.stopped = true
	server.status = model.RunStatusStopping
	cancel := server.cancel
	cmd := server.cmd
	pidFile := server.pidFile
	done := server.done
	server.mu.Unlock()

	m.logf("server %s stop requested", server.id)
	m.emitServerState(server)
	if err := m.interruptProcess(pidFile); err != nil {
		m.logf("server %s graceful stop failed: %v", server.id, err)
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

func (m *Manager) GetServerSnapshot(serverID string) model.ServerSnapshot {
	server := m.getServer(serverID)
	if server == nil {
		return model.ServerSnapshot{ServerID: serverID, Error: &model.AppError{
			Code: model.ErrRunNotFound, Stage: "session", Message: "服务端会话不存在",
		}}
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	return serverSnapshotLocked(server)
}

func (m *Manager) stopServerForShutdown(server *serverRun) {
	server.mu.Lock()
	if isTerminal(server.status) {
		server.mu.Unlock()
		return
	}
	server.stopped = true
	server.status = model.RunStatusStopping
	cancel := server.cancel
	cmd := server.cmd
	pidFile := server.pidFile
	done := server.done
	server.mu.Unlock()

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

func (m *Manager) runServer(server *serverRun) {
	defer close(server.done)
	defer m.removeServer(server)
	if server.pidFile != "" {
		defer os.Remove(server.pidFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server.mu.Lock()
	server.cancel = cancel
	if server.stopped {
		cancel()
	}
	args := iperf.BuildServerArgs(server.config, m.capabilities.JSONStream)
	if server.pidFile != "" {
		args = append(args, "-I", server.pidFile)
	}
	server.mu.Unlock()

	cmd := exec.CommandContext(ctx, m.enginePath, args...)
	iperf.ConfigureProcess(cmd)
	cmd.WaitDelay = 5 * time.Second

	stdout := &streamWriter{onLine: func(line []byte) {
		m.handleServerStdout(server, line)
	}}
	stderr := &streamWriter{onLine: func(line []byte) {
		m.handleServerLog(server, string(line))
	}}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		server.mu.Lock()
		stopped := server.stopped
		server.mu.Unlock()
		if stopped {
			m.finishServer(server, model.RunStatusStopped, nil, nil, "服务端已停止")
			return
		}
		m.finishServer(server, model.RunStatusFailed, nil, &model.AppError{
			Code:    model.ErrProcessSpawnFailed,
			Stage:   "spawn",
			Message: "无法启动 iPerf3 服务端，请检查安装完整性",
			Detail:  err.Error(),
		}, "")
		return
	}

	server.mu.Lock()
	server.cmd = cmd
	if server.stopped {
		server.mu.Unlock()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	} else {
		server.status = model.RunStatusRunning
		server.mu.Unlock()
		m.emitServerState(server)
		m.handleServerLog(server, fmt.Sprintf("服务端已开始监听端口 %d", server.config.Port))
	}

	_ = cmd.Wait()
	stdout.Flush()
	stderr.Flush()

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	server.mu.Lock()
	stopped := server.stopped
	config := server.config
	logText := strings.Join(server.logs, "\n")
	server.mu.Unlock()

	if stopped {
		m.finishServer(server, model.RunStatusStopped, intPtr(exitCode), nil, "服务端已停止")
		return
	}
	if exitCode == 0 {
		message := "服务端已停止"
		if config.OneOff {
			message = "单次连接完成，服务端已停止"
		}
		m.finishServer(server, model.RunStatusStopped, intPtr(exitCode), nil, message)
		return
	}
	m.finishServer(server, model.RunStatusFailed, intPtr(exitCode), iperf.ClassifyProcessFailure(logText, exitCode), "")
}

func (m *Manager) handleServerStdout(server *serverRun, line []byte) {
	text := strings.TrimSpace(string(line))
	if text == "" {
		return
	}
	if !m.capabilities.JSONStream {
		m.handleServerLog(server, text)
		return
	}
	event, err := iperf.ParseStreamEvent(line)
	if err != nil {
		m.handleServerLog(server, text)
		return
	}
	switch event.Event {
	case "start":
		m.handleServerTestStart(server, event.Data)
	case "interval":
		m.handleServerTestInterval(server, event.Data)
	case "end":
		m.handleServerTestEnd(server, event.Data)
	default:
		m.handleServerLog(server, text)
	}
}

func (m *Manager) handleServerTestStart(server *serverRun, data json.RawMessage) {
	info, err := iperf.ParseServerStart(data)
	if err != nil {
		m.handleServerLog(server, "无法解析服务端测试开始事件: "+err.Error())
		return
	}
	now := time.Now()
	clock := now.Format("15:04:05")
	protocolText := strings.ToUpper(string(info.Protocol))

	server.mu.Lock()
	if server.currentTest != nil && server.currentTest.Status == model.RunStatusRunning {
		server.currentTest.Status = model.RunStatusInterrupted
		server.currentTest.EndedAt = formatTime(now)
		if server.currentConnection >= 0 && server.currentConnection < len(server.connections) {
			connection := &server.connections[server.currentConnection]
			connection.Status = "已中断"
			connection.EndedAt = clock
		}
	}
	server.currentTest = &model.ServerTestSnapshot{
		Client:          info.Client,
		Protocol:        info.Protocol,
		Direction:       info.Direction,
		Status:          model.RunStatusRunning,
		StartedAt:       formatTime(now),
		DurationSeconds: info.DurationSeconds,
		Intervals:       []model.Interval{},
	}
	server.testSequence = 0
	server.connections = append(server.connections, model.ServerConnection{
		Client:    info.Client,
		Protocol:  protocolText,
		Direction: info.Direction,
		StartedAt: clock,
		Status:    "运行中",
	})
	server.currentConnection = len(server.connections) - 1
	if len(server.connections) > 100 {
		drop := len(server.connections) - 100
		server.connections = server.connections[drop:]
		server.currentConnection -= drop
	}
	server.mu.Unlock()

	m.handleServerLog(server, fmt.Sprintf(
		"连接来自 %s：%s %s，时长 %d 秒",
		info.Client, protocolText, directionText(info.Direction), info.DurationSeconds,
	))
	m.emitServerState(server)
}

func (m *Manager) handleServerTestInterval(server *serverRun, data json.RawMessage) {
	var intervalData iperf.IntervalData
	if err := json.Unmarshal(data, &intervalData); err != nil {
		m.handleServerLog(server, "无法解析服务端区间数据: "+err.Error())
		return
	}

	server.mu.Lock()
	if server.currentTest == nil || server.currentTest.Status != model.RunStatusRunning {
		server.mu.Unlock()
		return
	}
	server.testSequence++
	cfg := model.ClientConfig{
		Protocol:  server.currentTest.Protocol,
		Direction: server.currentTest.Direction,
	}
	intervals := iperf.BuildIntervals(server.id, server.testSequence, cfg, intervalData)
	intervals = sanitizeServerIntervals(intervals, server.currentTest.Protocol, server.currentTest.Direction)
	if len(intervals) == 0 {
		server.mu.Unlock()
		return
	}
	server.currentTest.Intervals = trimLatest(append(server.currentTest.Intervals, intervals...))
	server.mu.Unlock()

	m.emitServerState(server)
}

func (m *Manager) handleServerTestEnd(server *serverRun, data json.RawMessage) {
	var endData iperf.EndData
	if err := json.Unmarshal(data, &endData); err != nil {
		m.handleServerLog(server, "无法解析服务端结束数据: "+err.Error())
		return
	}

	now := time.Now()
	clock := now.Format("15:04:05")
	server.mu.Lock()
	if server.currentTest == nil {
		server.mu.Unlock()
		return
	}
	cfg := model.ClientConfig{
		Protocol:  server.currentTest.Protocol,
		Direction: server.currentTest.Direction,
	}
	summary := iperf.BuildSummary(cfg, endData)
	sanitizeServerSummary(summary, server.currentTest.Protocol, server.currentTest.Direction)
	server.currentTest.Summary = summary
	server.currentTest.Status = model.RunStatusCompleted
	server.currentTest.EndedAt = formatTime(now)
	if server.currentConnection >= 0 && server.currentConnection < len(server.connections) {
		connection := &server.connections[server.currentConnection]
		connection.Status = "已完成"
		connection.EndedAt = clock
		if summary != nil {
			if summary.Upload != nil {
				connection.UploadBitsPerSecond = summary.Upload.BitsPerSecond
			}
			if summary.Download != nil {
				connection.DownloadBitsPerSecond = summary.Download.BitsPerSecond
			}
		}
	}
	test := copyServerTest(server.currentTest)
	server.mu.Unlock()

	m.handleServerLog(server, serverTestSummaryLog(test))
	m.emitServerState(server)
}

func (m *Manager) handleServerLog(server *serverRun, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	timestamp := time.Now().Format("15:04:05")

	server.mu.Lock()
	server.sequence++
	sequence := server.sequence
	server.logs = append(server.logs, text)
	if len(server.logs) > serverLogLimit {
		server.logs = server.logs[len(server.logs)-serverLogLimit:]
	}
	addedConnection := false
	if !m.capabilities.JSONStream {
		if match := acceptedConnectionPattern.FindStringSubmatch(text); len(match) == 3 {
			client := net.JoinHostPort(match[1], match[2])
			server.connections = append(server.connections, model.ServerConnection{
				Client:    client,
				Protocol:  "未知",
				StartedAt: timestamp,
				Status:    "已连接",
			})
			if len(server.connections) > 100 {
				server.connections = server.connections[len(server.connections)-100:]
			}
			addedConnection = true
		}
	}
	interrupted := false
	if strings.Contains(strings.ToLower(text), "the client has terminated") &&
		server.currentTest != nil && server.currentTest.Status == model.RunStatusRunning {
		now := time.Now()
		server.currentTest.Status = model.RunStatusInterrupted
		server.currentTest.EndedAt = formatTime(now)
		if server.currentConnection >= 0 && server.currentConnection < len(server.connections) {
			connection := &server.connections[server.currentConnection]
			connection.Status = "已中断"
			connection.EndedAt = now.Format("15:04:05")
		}
		interrupted = true
	}
	server.mu.Unlock()

	m.logf("server %s: %s", server.id, text)
	m.emitEvent("server:log", model.ServerLogEvent{
		ServerID:  server.id,
		Sequence:  sequence,
		Timestamp: timestamp,
		Line:      text,
	})
	if addedConnection || interrupted {
		m.emitServerState(server)
	}
}

func (m *Manager) emitServerState(server *serverRun) {
	server.mu.Lock()
	server.sequence++
	snapshot := serverSnapshotLocked(server)
	server.mu.Unlock()
	m.emitEvent("server:state", snapshot)
}

func (m *Manager) finishServer(server *serverRun, status model.RunStatus, exitCode *int, appErr *model.AppError, message string) {
	now := time.Now()
	clock := now.Format("15:04:05")
	server.mu.Lock()
	server.status = status
	server.endedAt = now
	server.exitCode = copyInt(exitCode)
	server.appErr = appErr
	server.message = message
	if server.currentTest != nil && server.currentTest.Status == model.RunStatusRunning {
		testStatus := model.RunStatusStopped
		connectionStatus := "已停止"
		if status == model.RunStatusFailed {
			testStatus = model.RunStatusFailed
			connectionStatus = "失败"
		}
		server.currentTest.Status = testStatus
		server.currentTest.EndedAt = formatTime(now)
		if server.currentConnection >= 0 && server.currentConnection < len(server.connections) {
			connection := &server.connections[server.currentConnection]
			connection.Status = connectionStatus
			connection.EndedAt = clock
		}
	}
	for index := range server.connections {
		if server.connections[index].Status == "运行中" || server.connections[index].Status == "已连接" {
			server.connections[index].Status = "已停止"
			if server.connections[index].EndedAt == "" {
				server.connections[index].EndedAt = clock
			}
		}
	}
	server.mu.Unlock()

	if exitCode != nil {
		m.logf("server %s finish status=%s exit=%d error=%+v", server.id, status, *exitCode, appErr)
	} else {
		m.logf("server %s finish status=%s exit=nil error=%+v", server.id, status, appErr)
	}
	m.emitServerState(server)
}

func serverSnapshotLocked(server *serverRun) model.ServerSnapshot {
	return model.ServerSnapshot{
		ServerID:    server.id,
		Sequence:    server.sequence,
		Status:      server.status,
		BindAddress: server.config.BindAddress,
		Port:        server.config.Port,
		StartedAt:   formatTime(server.startedAt),
		EndedAt:     formatTime(server.endedAt),
		ExitCode:    copyInt(server.exitCode),
		Connections: append([]model.ServerConnection(nil), server.connections...),
		CurrentTest: copyServerTest(server.currentTest),
		Logs:        append([]string(nil), server.logs...),
		Message:     server.message,
		Error:       copyAppError(server.appErr),
	}
}

func copyServerTest(test *model.ServerTestSnapshot) *model.ServerTestSnapshot {
	if test == nil {
		return nil
	}
	cloned := *test
	cloned.Intervals = append([]model.Interval(nil), test.Intervals...)
	cloned.Summary = copySummary(test.Summary)
	return &cloned
}

func sanitizeServerIntervals(intervals []model.Interval, protocol model.Protocol, direction model.Direction) []model.Interval {
	if protocol != model.ProtocolUDP {
		return intervals
	}
	for index := range intervals {
		if direction == model.DirectionDownload ||
			(direction == model.DirectionBidirectional && intervals[index].Direction == model.DirectionDownload) {
			intervals[index].UDPJitterMilliseconds = 0
			intervals[index].UDPLostDatagrams = 0
			intervals[index].UDPTotalDatagrams = 0
		}
	}
	return intervals
}

func sanitizeServerSummary(summary *model.Summary, protocol model.Protocol, direction model.Direction) {
	if summary == nil || protocol != model.ProtocolUDP {
		return
	}
	if (direction == model.DirectionDownload || direction == model.DirectionBidirectional) && summary.Download != nil {
		summary.Download.UDPJitterMilliseconds = 0
		summary.Download.UDPLostDatagrams = 0
		summary.Download.UDPTotalDatagrams = 0
		summary.Download.UDPLostPercent = 0
	}
}

func serverTestSummaryLog(test *model.ServerTestSnapshot) string {
	if test == nil {
		return "测试结束"
	}
	parts := []string{fmt.Sprintf("%s %s 完成", strings.ToUpper(string(test.Protocol)), directionText(test.Direction))}
	if test.Summary != nil {
		if test.Summary.Upload != nil {
			parts = append(parts, "上传 "+formatBits(test.Summary.Upload.BitsPerSecond))
		}
		if test.Summary.Download != nil {
			parts = append(parts, "下载 "+formatBits(test.Summary.Download.BitsPerSecond))
		}
	}
	return strings.Join(parts, "，")
}

func directionText(direction model.Direction) string {
	switch direction {
	case model.DirectionDownload:
		return "下载"
	case model.DirectionBidirectional:
		return "双向"
	default:
		return "上传"
	}
}

func formatBits(bitsPerSecond float64) string {
	switch {
	case bitsPerSecond >= 1e9:
		return fmt.Sprintf("%.2f Gbit/s", bitsPerSecond/1e9)
	case bitsPerSecond >= 1e6:
		return fmt.Sprintf("%.2f Mbit/s", bitsPerSecond/1e6)
	case bitsPerSecond >= 1e3:
		return fmt.Sprintf("%.2f Kbit/s", bitsPerSecond/1e3)
	default:
		return fmt.Sprintf("%.0f bit/s", bitsPerSecond)
	}
}

func (m *Manager) getServer(serverID string) *serverRun {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.servers[serverID]
}

func (m *Manager) removeServer(server *serverRun) {
	m.mu.Lock()
	if m.serverKeys[server.key] == server.id {
		delete(m.serverKeys, server.key)
	}
	delete(m.servers, server.id)
	m.mu.Unlock()
}

func normalizedBind(bind string) string {
	if bind == "" {
		return "0.0.0.0"
	}
	return bind
}

func checkPortAvailable(bind string, port int) error {
	address := net.JoinHostPort(normalizedBind(bind), strconv.Itoa(port))
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	return listener.Close()
}
