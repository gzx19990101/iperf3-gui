package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"iperf3-gui/internal/iperf"
	"iperf3-gui/internal/model"
)

const finalJSONLimit = 4 * 1024 * 1024

func (m *Manager) runClient(run *clientRun) {
	defer close(run.done)
	defer m.finishActiveRun(run.id)
	if run.pidFile != "" {
		defer os.Remove(run.pidFile)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	run.mu.Lock()
	run.cancel = cancel
	if run.stopped {
		cancel()
	}
	args := iperf.BuildClientArgs(run.config, m.capabilities.JSONStream)
	if run.pidFile != "" {
		args = append(args, "-I", run.pidFile)
	}
	config := run.config
	run.mu.Unlock()

	cmd := exec.CommandContext(ctx, m.enginePath, args...)
	iperf.ConfigureProcess(cmd)
	cmd.WaitDelay = 5 * time.Second

	var stdoutStream *streamWriter
	var finalBuffer *boundedBufferWriter
	if m.capabilities.JSONStream {
		stdoutStream = &streamWriter{onLine: func(line []byte) {
			m.handleClientLine(run, line)
		}}
		cmd.Stdout = stdoutStream
	} else {
		finalBuffer = newBoundedBufferWriter(finalJSONLimit)
		cmd.Stdout = finalBuffer
	}
	stderrBuffer := newBoundedBufferWriter(stderrLimit)
	cmd.Stderr = stderrBuffer

	if err := cmd.Start(); err != nil {
		run.mu.Lock()
		stopped := run.stopped
		run.mu.Unlock()
		if stopped {
			m.finishClientRun(run, model.RunStatusStopped, nil, nil, nil, "测试已停止", "", false)
			return
		}
		m.failRun(run, &model.AppError{
			Code:    model.ErrProcessSpawnFailed,
			Stage:   "spawn",
			Message: "无法启动 iPerf3 进程，请检查安装完整性",
			Detail:  err.Error(),
		}, "", false)
		return
	}

	run.mu.Lock()
	run.cmd = cmd
	if run.stopped {
		run.mu.Unlock()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	} else {
		run.status = model.RunStatusRunning
		sequence := run.nextSequenceLocked()
		startedAt := formatTime(run.startedAt)
		run.mu.Unlock()
		m.emitEvent("run:state", model.RunStateEvent{
			RunID:     run.id,
			Sequence:  sequence,
			Status:    model.RunStatusRunning,
			StartedAt: startedAt,
		})
	}

	waitErr := cmd.Wait()
	if stdoutStream != nil {
		stdoutStream.Flush()
	}

	stderrText, stderrTruncated := stderrBuffer.String()
	run.mu.Lock()
	run.stderr.Reset()
	run.stderr.WriteString(stderrText)
	run.stderrTruncated = run.stderrTruncated || stderrTruncated
	run.mu.Unlock()

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	if finalBuffer != nil {
		m.handleFinalJSON(run, finalBuffer, config)
	}

	run.mu.Lock()
	stopped := run.stopped
	summary := copySummary(run.summary)
	parseErrors := run.parseErrors
	run.mu.Unlock()

	if stopped {
		m.finishClientRun(run, model.RunStatusStopped, intPtr(exitCode), summary, nil, "测试已停止", stderrText, stderrTruncated)
		return
	}
	if exitCode == 0 && summary != nil {
		m.finishClientRun(run, model.RunStatusCompleted, intPtr(exitCode), summary, nil, "", stderrText, stderrTruncated)
		return
	}
	if exitCode == 0 {
		detail := "iPerf3 未返回可解析的结束汇总"
		if parseErrors > 0 {
			detail = fmt.Sprintf("有 %d 行输出无法解析", parseErrors)
		}
		m.finishClientRun(run, model.RunStatusFailed, intPtr(exitCode), nil, &model.AppError{
			Code:    model.ErrParseFailed,
			Stage:   "parse",
			Message: "无法解析 iPerf3 输出，请展开原始输出排查",
			Detail:  detail,
		}, "", stderrText, stderrTruncated)
		return
	}

	appErr := iperf.ClassifyProcessFailure(stderrText, exitCode)
	if waitErr != nil && stderrText == "" {
		appErr.Detail = waitErr.Error()
	}
	m.finishClientRun(run, model.RunStatusFailed, intPtr(exitCode), summary, appErr, "", stderrText, stderrTruncated)
}

func (m *Manager) handleClientLine(run *clientRun, line []byte) {
	run.mu.Lock()
	run.rawTruncated = appendBounded(&run.raw, string(line)+"\n", rawOutputLimit) || run.rawTruncated
	config := run.config
	run.mu.Unlock()

	event, err := iperf.ParseStreamEvent(line)
	if err != nil {
		run.mu.Lock()
		run.parseErrors++
		run.mu.Unlock()
		return
	}
	switch event.Event {
	case "interval":
		var data iperf.IntervalData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			run.mu.Lock()
			run.parseErrors++
			run.mu.Unlock()
			return
		}
		run.mu.Lock()
		sequence := run.nextSequenceLocked()
		intervals := iperf.BuildIntervals(run.id, sequence, config, data)
		run.latest = trimLatest(append(run.latest, intervals...))
		historyID := run.historyID
		run.mu.Unlock()
		if len(intervals) == 0 {
			return
		}
		if m.store != nil && historyID != "" {
			_ = m.store.AppendIntervals(run.id, intervals)
		}
		m.emitEvent("run:interval", model.RunIntervalEvent{
			RunID:     run.id,
			Sequence:  sequence,
			Intervals: intervals,
		})
	case "end":
		var data iperf.EndData
		if err := json.Unmarshal(event.Data, &data); err != nil {
			run.mu.Lock()
			run.parseErrors++
			run.mu.Unlock()
			return
		}
		if summary := iperf.BuildSummary(config, data); summary != nil {
			run.mu.Lock()
			run.summary = summary
			run.mu.Unlock()
		}
	}
}

func (m *Manager) handleFinalJSON(run *clientRun, buffer *boundedBufferWriter, config model.ClientConfig) {
	raw, truncated := buffer.String()
	run.mu.Lock()
	run.rawTruncated = appendBounded(&run.raw, raw, rawOutputLimit) || run.rawTruncated
	historyID := run.historyID
	baseSequence := run.sequence
	run.mu.Unlock()

	intervals, summary, err := iperf.ParseFinalJSON([]byte(raw), config)
	if err != nil {
		run.mu.Lock()
		run.parseErrors++
		run.mu.Unlock()
		return
	}
	for index := range intervals {
		intervals[index].RunID = run.id
		intervals[index].Sequence = baseSequence + index + 1
	}
	run.mu.Lock()
	run.latest = trimLatest(append(run.latest, intervals...))
	run.sequence = baseSequence + len(intervals)
	run.summary = summary
	if truncated {
		run.message = "原始 JSON 输出过大，仅解析了前 4 MiB"
	}
	run.mu.Unlock()
	if m.store != nil && historyID != "" && len(intervals) > 0 {
		_ = m.store.AppendIntervals(run.id, intervals)
	}
}

func (m *Manager) failRun(run *clientRun, appErr *model.AppError, stderrText string, stderrTruncated bool) {
	m.finishClientRun(run, model.RunStatusFailed, nil, nil, appErr, "", stderrText, stderrTruncated)
}

func (m *Manager) finishClientRun(run *clientRun, status model.RunStatus, exitCode *int, summary *model.Summary, appErr *model.AppError, message, stderrText string, stderrTruncated bool) {
	m.finishActiveRun(run.id)
	run.mu.Lock()
	run.status = status
	run.endedAt = time.Now()
	run.exitCode = copyInt(exitCode)
	run.summary = summary
	run.appErr = appErr
	if message != "" {
		run.message = message
	}
	sequence := run.nextSequenceLocked()
	rawOutput := run.raw.String()
	rawTruncated := run.rawTruncated
	historyID := run.historyID
	if run.stderr.Len() == 0 && stderrText != "" {
		run.stderr.WriteString(stderrText)
		run.stderrTruncated = run.stderrTruncated || stderrTruncated
	}
	stderrSummary := run.stderr.String()
	stderrWasTruncated := run.stderrTruncated
	run.mu.Unlock()

	if m.store != nil && historyID != "" {
		if err := m.store.FinishRun(run.id, status, exitCode, summary, rawOutput, rawTruncated, stderrSummary, stderrWasTruncated); err != nil {
			m.logf("run %s history finish failed: %v", run.id, err)
		}
	}

	exitValue := "nil"
	if exitCode != nil {
		exitValue = fmt.Sprintf("%d", *exitCode)
	}
	m.logf("run %s finish status=%s exit=%s error=%+v", run.id, status, exitValue, appErr)

	switch status {
	case model.RunStatusCompleted:
		m.emitEvent("run:completed", model.RunCompletedEvent{
			RunID:     run.id,
			Sequence:  sequence,
			Summary:   summary,
			ExitCode:  exitCode,
			HistoryID: historyID,
		})
	case model.RunStatusFailed:
		m.emitEvent("run:failed", model.RunFailedEvent{
			RunID:         run.id,
			Sequence:      sequence,
			Stage:         errorStage(appErr),
			Error:         appErr,
			StderrSummary: stderrSummary,
		})
	default:
		m.emitEvent("run:state", model.RunStateEvent{
			RunID:     run.id,
			Sequence:  sequence,
			Status:    status,
			StartedAt: formatTime(run.startedAt),
			Message:   message,
		})
	}
}

func errorStage(appErr *model.AppError) string {
	if appErr == nil {
		return "run"
	}
	return appErr.Stage
}

func intPtr(value int) *int {
	return &value
}

func trimLatest(items []model.Interval) []model.Interval {
	if len(items) == 0 {
		return items
	}
	hasUpload := false
	hasDownload := false
	for _, item := range items {
		if item.Direction == model.DirectionUpload {
			hasUpload = true
		} else {
			hasDownload = true
		}
	}
	uploadCount := 0
	downloadCount := 0
	start := 0
	for index := len(items) - 1; index >= 0; index-- {
		if items[index].Direction == model.DirectionUpload {
			uploadCount++
		} else {
			downloadCount++
		}
		start = index
		if hasUpload && hasDownload && uploadCount >= chartPointLimit && downloadCount >= chartPointLimit {
			break
		}
		if !hasUpload && downloadCount >= chartPointLimit {
			break
		}
		if !hasDownload && uploadCount >= chartPointLimit {
			break
		}
	}
	return items[start:]
}
