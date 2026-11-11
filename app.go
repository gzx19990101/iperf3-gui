package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"iperf3-gui/internal/appservice"
	"iperf3-gui/internal/model"
	"iperf3-gui/internal/session"
)

type App struct {
	ctx     context.Context
	service *appservice.Service
}

func NewApp() *App {
	app := &App{}
	emit := session.Emitter(func(event string, data ...interface{}) {
		if app.ctx != nil {
			runtime.EventsEmit(app.ctx, event, data...)
		}
	})
	app.service = appservice.New(context.Background(), emit)
	return app
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.service != nil {
		a.service.SetContext(ctx)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.service != nil {
		a.service.Close(ctx)
	}
}

func (a *App) GetSystemStatus() model.EngineStatus {
	if a.service == nil {
		return model.EngineStatus{Error: &model.AppError{
			Code: model.ErrIPerfProbeFailed, Stage: "probe", Message: "应用服务尚未初始化",
		}}
	}
	return a.service.SystemStatus()
}

func (a *App) StartClientTest(config model.ClientConfig) model.StartRunResult {
	if a.service == nil {
		return model.StartRunResult{Error: serviceUnavailableError()}
	}
	return a.service.StartClientTest(config)
}

func (a *App) StopClientTest(runID string) model.StopResult {
	if a.service == nil {
		return model.StopResult{Error: serviceUnavailableError()}
	}
	return a.service.StopClientTest(runID)
}

func (a *App) PreviewClientCommand(config model.ClientConfig) string {
	if a.service == nil {
		return "iperf3"
	}
	return a.service.PreviewClientCommand(config)
}

func (a *App) PreviewServerCommand(config model.ServerConfig) string {
	if a.service == nil {
		return "iperf3"
	}
	return a.service.PreviewServerCommand(config)
}

func (a *App) GetRunSnapshot(runID string) model.RunSnapshot {
	if a.service == nil {
		return model.RunSnapshot{RunID: runID, Error: serviceUnavailableError()}
	}
	return a.service.GetRunSnapshot(runID)
}

func (a *App) StartServer(config model.ServerConfig) model.StartServerResult {
	if a.service == nil {
		return model.StartServerResult{Error: serviceUnavailableError()}
	}
	return a.service.StartServer(config)
}

func (a *App) StopServer(serverID string) model.StopResult {
	if a.service == nil {
		return model.StopResult{Error: serviceUnavailableError()}
	}
	return a.service.StopServer(serverID)
}

func (a *App) ListHistory(filter model.HistoryFilter) model.HistoryPage {
	if a.service == nil {
		return model.HistoryPage{Error: serviceUnavailableError()}
	}
	return a.service.ListHistory(filter)
}

func (a *App) GetHistoryResult(id string, page, pageSize int) model.HistoryResult {
	if a.service == nil {
		return model.HistoryResult{Error: serviceUnavailableError()}
	}
	return a.service.GetHistoryResult(id, page, pageSize)
}

func (a *App) ExportResult(id, format string) model.ExportResultInfo {
	if a.service == nil {
		return model.ExportResultInfo{Error: serviceUnavailableError()}
	}
	return a.service.ExportResult(id, format)
}

func (a *App) DeleteHistory(ids []string) model.DeleteResult {
	if a.service == nil {
		return model.DeleteResult{Error: serviceUnavailableError()}
	}
	return a.service.DeleteHistory(ids)
}

func (a *App) ClearHistory() model.DeleteResult {
	if a.service == nil {
		return model.DeleteResult{Error: serviceUnavailableError()}
	}
	return a.service.ClearHistory()
}

func serviceUnavailableError() *model.AppError {
	return &model.AppError{
		Code: model.ErrStateConflict, Stage: "session", Message: "应用服务尚未初始化",
	}
}
