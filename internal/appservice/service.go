package appservice

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"iperf3-gui/internal/engine"
	"iperf3-gui/internal/history"
	"iperf3-gui/internal/iperf"
	"iperf3-gui/internal/logging"
	"iperf3-gui/internal/model"
	"iperf3-gui/internal/session"
)

type Service struct {
	ctx       context.Context
	store     *history.Store
	manager   *session.Manager
	logger    *logging.Logger
	engine    model.EngineStatus
	statusMu  sync.RWMutex
	storeErr  *model.AppError
	closeOnce sync.Once
}

func New(ctx context.Context, emit session.Emitter) *Service {
	service := &Service{ctx: ctx}
	service.engine = model.EngineStatus{Interfaces: iperf.ListInterfaces()}

	dataDir, dataDirErr := engine.AppDataDir()
	if dataDirErr == nil {
		if logger, logErr := logging.Open(dataDir); logErr == nil {
			service.logger = logger
		}
	}
	service.logf("application starting")

	enginePath, engineErr := engine.EnsureBundled()
	if engineErr != nil {
		service.engine.Error = &model.AppError{
			Code:    model.ErrIPerfNotFound,
			Stage:   "probe",
			Message: "随包 iPerf3 缺失或无法释放，请修复安装",
			Detail:  engineErr.Error(),
		}
		service.logf("engine bundle failed: %v", engineErr)
	} else {
		service.engine = iperf.Probe(ctx, enginePath)
		if service.engine.Available {
			service.logf(
				"engine ready path=%s version=%s jsonStream=%v bidirTcp=%v bidirUdp=%v",
				service.engine.Path,
				service.engine.Version,
				service.engine.Capabilities.JSONStream,
				service.engine.Capabilities.BidirTCP,
				service.engine.Capabilities.BidirUDP,
			)
		} else {
			service.logf("engine probe failed: %+v", service.engine.Error)
		}
	}

	if dataDirErr != nil {
		service.storeErr = &model.AppError{
			Code:    model.ErrDBOpenFailed,
			Stage:   "persist",
			Message: "无法确定应用数据目录，历史功能不可用",
			Detail:  dataDirErr.Error(),
		}
		service.logf("app data directory failed: %v", dataDirErr)
	} else {
		store, openErr := history.Open(filepath.Join(dataDir, "iperf3.db"))
		if openErr != nil {
			service.storeErr = &model.AppError{
				Code:    model.ErrDBOpenFailed,
				Stage:   "persist",
				Message: "历史数据库无法打开，测试仍可使用但不会保存历史",
				Detail:  openErr.Error(),
			}
			service.logf("history open failed: %v", openErr)
		} else {
			service.store = store
			if count, err := store.MarkStaleRunning(); err != nil {
				service.logf("mark stale runs failed: %v", err)
			} else if count > 0 {
				service.logf("marked %d stale runs as interrupted", count)
			}
		}
	}

	service.manager = session.New(enginePath, service.engine.Capabilities, service.store, emit, service.logger)
	return service
}

func (s *Service) logf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

func (s *Service) SystemStatus() model.EngineStatus {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.engine
}

func (s *Service) SetContext(ctx context.Context) {
	s.ctx = ctx
}

func (s *Service) StartClientTest(cfg model.ClientConfig) model.StartRunResult {
	if !s.engineAvailable() {
		return model.StartRunResult{Error: s.engineUnavailableError()}
	}
	return s.manager.StartClientTest(cfg)
}

func (s *Service) StopClientTest(runID string) model.StopResult {
	if s.manager == nil {
		return model.StopResult{Error: &model.AppError{
			Code: model.ErrRunNotFound, Stage: "session", Message: "会话管理器尚未初始化",
		}}
	}
	return s.manager.StopClientTest(runID)
}

func (s *Service) PreviewClientCommand(cfg model.ClientConfig) string {
	return iperf.Preview(iperf.BuildClientArgs(cfg, s.engine.Capabilities.JSONStream))
}

func (s *Service) PreviewServerCommand(cfg model.ServerConfig) string {
	return iperf.Preview(iperf.BuildServerArgs(cfg, s.engine.Capabilities.JSONStream))
}

func (s *Service) GetRunSnapshot(runID string) model.RunSnapshot {
	if s.manager == nil {
		return model.RunSnapshot{RunID: runID, Error: &model.AppError{
			Code: model.ErrRunNotFound, Stage: "session", Message: "会话管理器尚未初始化",
		}}
	}
	return s.manager.GetRunSnapshot(runID)
}

func (s *Service) StartServer(cfg model.ServerConfig) model.StartServerResult {
	if !s.engineAvailable() {
		return model.StartServerResult{Error: s.engineUnavailableError()}
	}
	return s.manager.StartServer(cfg)
}

func (s *Service) StopServer(serverID string) model.StopResult {
	if s.manager == nil {
		return model.StopResult{Error: &model.AppError{
			Code: model.ErrRunNotFound, Stage: "session", Message: "会话管理器尚未初始化",
		}}
	}
	return s.manager.StopServer(serverID)
}

func (s *Service) ListHistory(filter model.HistoryFilter) model.HistoryPage {
	if s.store == nil {
		return model.HistoryPage{Error: s.storeUnavailableError()}
	}
	page, err := s.store.List(filter)
	if err != nil {
		return model.HistoryPage{Error: &model.AppError{
			Code: model.ErrDBWriteFailed, Stage: "persist", Message: "查询历史失败", Detail: err.Error(),
		}}
	}
	return page
}

func (s *Service) GetHistoryResult(id string, page, pageSize int) model.HistoryResult {
	if s.store == nil {
		return model.HistoryResult{Error: s.storeUnavailableError()}
	}
	result, err := s.store.Get(id, page, pageSize)
	if err != nil {
		return model.HistoryResult{Error: &model.AppError{
			Code: model.ErrDBWriteFailed, Stage: "persist", Message: "读取历史详情失败", Detail: err.Error(),
		}}
	}
	return result
}

func (s *Service) ExportResult(id, format string) model.ExportResultInfo {
	if s.store == nil {
		return model.ExportResultInfo{Error: s.storeUnavailableError()}
	}
	result, err := s.store.Get(id, 1, 1)
	if err != nil {
		return model.ExportResultInfo{Error: &model.AppError{
			Code: model.ErrDBWriteFailed, Stage: "persist", Message: "读取历史详情失败", Detail: err.Error(),
		}}
	}
	if result.Error != nil {
		return model.ExportResultInfo{Error: result.Error}
	}
	intervals, err := s.store.AllIntervals(id)
	if err != nil {
		return model.ExportResultInfo{Error: &model.AppError{
			Code: model.ErrDBWriteFailed, Stage: "persist", Message: "读取区间数据失败", Detail: err.Error(),
		}}
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format != "json" && format != "csv" {
		return model.ExportResultInfo{Error: &model.AppError{
			Code: model.ErrInvalidValue, Stage: "export", Message: "导出格式必须是 JSON 或 CSV",
		}}
	}

	filename := fmt.Sprintf("iperf3-%s-%s.%s", time.Now().Format("20060102-150405"), sanitizeFilename(result.Run.Host), format)
	path, dialogErr := runtime.SaveFileDialog(s.ctx, runtime.SaveDialogOptions{
		Title:           "导出测试结果",
		DefaultFilename: filename,
		Filters: []runtime.FileFilter{{
			DisplayName: strings.ToUpper(format) + " 文件",
			Pattern:     "*." + format,
		}},
	})
	if dialogErr != nil {
		return model.ExportResultInfo{Error: &model.AppError{
			Code: model.ErrFileDialogFailed, Stage: "export", Message: "无法打开保存对话框", Detail: dialogErr.Error(),
		}}
	}
	if strings.TrimSpace(path) == "" {
		return model.ExportResultInfo{Canceled: true}
	}

	var data []byte
	if format == "json" {
		payload := struct {
			ExportedAt string           `json:"exportedAt"`
			Run        model.RunRecord  `json:"run"`
			Intervals  []model.Interval `json:"intervals"`
		}{
			ExportedAt: time.Now().UTC().Format(time.RFC3339Nano),
			Run:        result.Run,
			Intervals:  intervals,
		}
		data, err = json.MarshalIndent(payload, "", "  ")
	} else {
		data, err = exportCSV(result.Run, intervals)
	}
	if err != nil {
		return model.ExportResultInfo{Error: &model.AppError{
			Code: model.ErrExportWriteFailed, Stage: "export", Message: "生成导出文件失败", Detail: err.Error(),
		}}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return model.ExportResultInfo{Error: &model.AppError{
			Code: model.ErrExportWriteFailed, Stage: "export", Message: "写入导出文件失败", Detail: err.Error(),
		}}
	}
	s.logf("exported run %s to %s", id, path)
	return model.ExportResultInfo{Path: path}
}

func (s *Service) DeleteHistory(ids []string) model.DeleteResult {
	if s.store == nil {
		return model.DeleteResult{Error: s.storeUnavailableError()}
	}
	deleted, err := s.store.Delete(ids)
	if err != nil {
		return model.DeleteResult{Error: &model.AppError{
			Code: model.ErrDBDeleteFailed, Stage: "persist", Message: "删除历史记录失败", Detail: err.Error(),
		}}
	}
	s.logf("deleted %d history records", deleted)
	return model.DeleteResult{Deleted: deleted}
}

func (s *Service) ClearHistory() model.DeleteResult {
	if s.store == nil {
		return model.DeleteResult{Error: s.storeUnavailableError()}
	}
	deleted, err := s.store.Clear()
	if err != nil {
		return model.DeleteResult{Error: &model.AppError{
			Code: model.ErrDBDeleteFailed, Stage: "persist", Message: "清空历史记录失败", Detail: err.Error(),
		}}
	}
	s.logf("cleared %d history records", deleted)
	return model.DeleteResult{Deleted: deleted}
}

func (s *Service) Close(ctx context.Context) {
	s.closeOnce.Do(func() {
		if s.manager != nil {
			s.manager.Shutdown(ctx)
		}
		if s.store != nil {
			_ = s.store.Close()
		}
		s.logf("application stopped")
		if s.logger != nil {
			_ = s.logger.Close()
		}
	})
}

func (s *Service) engineAvailable() bool {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.engine.Available && s.engine.Path != ""
}

func (s *Service) engineUnavailableError() *model.AppError {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	if s.engine.Error != nil {
		return copyError(s.engine.Error)
	}
	return &model.AppError{Code: model.ErrIPerfNotFound, Stage: "probe", Message: "随包 iPerf3 不可用"}
}

func (s *Service) storeUnavailableError() *model.AppError {
	if s.storeErr != nil {
		return copyError(s.storeErr)
	}
	return &model.AppError{Code: model.ErrDBOpenFailed, Stage: "persist", Message: "历史功能不可用"}
}

func exportCSV(run model.RunRecord, intervals []model.Interval) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buffer)
	header := []string{
		"run_id", "created_at", "protocol", "direction", "host", "port", "status",
		"direction_detail", "bits_per_second", "bytes", "seconds",
		"tcp_retransmits", "udp_jitter_ms", "udp_lost", "udp_total",
	}
	if err := writer.Write(header); err != nil {
		return nil, err
	}
	if run.Summary != nil {
		if run.Summary.Upload != nil {
			writeCSVSummary(writer, run, model.DirectionUpload, run.Summary.Upload)
		}
		if run.Summary.Download != nil {
			writeCSVSummary(writer, run, model.DirectionDownload, run.Summary.Download)
		}
	}
	_ = intervals
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func writeCSVSummary(writer *csv.Writer, run model.RunRecord, direction model.Direction, summary *model.DirectionSummary) {
	_ = writer.Write([]string{
		run.ID,
		run.CreatedAt,
		string(run.Protocol),
		string(run.Direction),
		run.Host,
		strconv.Itoa(run.Port),
		string(run.Status),
		string(direction),
		strconv.FormatFloat(summary.BitsPerSecond, 'f', 3, 64),
		strconv.FormatInt(summary.Bytes, 10),
		strconv.FormatFloat(summary.Seconds, 'f', 6, 64),
		strconv.FormatInt(summary.TCPRetransmits, 10),
		strconv.FormatFloat(summary.UDPJitterMilliseconds, 'f', 6, 64),
		strconv.FormatInt(summary.UDPLostDatagrams, 10),
		strconv.FormatInt(summary.UDPTotalDatagrams, 10),
	})
}

func sanitizeFilename(value string) string {
	replacer := strings.NewReplacer("\\", "_", "/", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	value = strings.TrimSpace(replacer.Replace(value))
	if value == "" {
		return "result"
	}
	return value
}

func copyError(value *model.AppError) *model.AppError {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
