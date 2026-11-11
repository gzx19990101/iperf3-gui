package history

import (
	"path/filepath"
	"testing"
	"time"

	"iperf3-gui/internal/model"
)

func testRun(id string, direction model.Direction, status model.RunStatus) model.RunRecord {
	return model.RunRecord{
		ID:            id,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		StartedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		Status:        status,
		Protocol:      model.ProtocolTCP,
		Direction:     direction,
		Host:          "127.0.0.1",
		Port:          5201,
		Config:        model.ClientConfig{Host: "127.0.0.1", Port: 5201, Protocol: model.ProtocolTCP, Direction: direction, DurationSeconds: 10},
		OutputMode:    "json-stream",
		ParserVersion: "json-stream-1",
	}
}

func TestStoreLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	run := testRun("run-1", model.DirectionUpload, model.RunStatusRunning)
	if err := store.CreateRun(run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	intervals := []model.Interval{
		{RunID: "run-1", Sequence: 1, Direction: model.DirectionUpload, StartSeconds: 0, EndSeconds: 1, BitsPerSecond: 1000, Bytes: 100},
		{RunID: "run-1", Sequence: 2, Direction: model.DirectionUpload, StartSeconds: 1, EndSeconds: 2, BitsPerSecond: 2000, Bytes: 200},
	}
	if err := store.AppendIntervals("run-1", intervals); err != nil {
		t.Fatalf("append intervals: %v", err)
	}
	summary := &model.Summary{
		Direction: model.DirectionUpload,
		Upload:    &model.DirectionSummary{BitsPerSecond: 1500, Bytes: 300, Seconds: 2},
	}
	exitCode := 0
	if err := store.FinishRun("run-1", model.RunStatusCompleted, &exitCode, summary, "raw output", false, "stderr", false); err != nil {
		t.Fatalf("finish run: %v", err)
	}

	page, err := store.List(model.HistoryFilter{Page: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected list result: %+v", page)
	}
	if page.Items[0].Status != model.RunStatusCompleted || page.Items[0].UploadBitsPerSecond != 1500 {
		t.Fatalf("unexpected history item: %+v", page.Items[0])
	}

	result, err := store.Get("run-1", 1, 100)
	if err != nil {
		t.Fatalf("get history: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("unexpected history error: %+v", result.Error)
	}
	if result.Run.Status != model.RunStatusCompleted || result.IntervalTotal != 2 || len(result.Intervals) != 2 {
		t.Fatalf("unexpected detail: %+v", result)
	}
	if result.RawOutput != "raw output" || result.StderrSummary != "stderr" {
		t.Fatalf("unexpected raw output: %+v", result)
	}

	all, err := store.AllIntervals("run-1")
	if err != nil || len(all) != 2 {
		t.Fatalf("all intervals: %v %+v", err, all)
	}

	paged, err := store.Get("run-1", 1, 1)
	if err != nil {
		t.Fatalf("get paged history: %v", err)
	}
	if len(paged.Intervals) != 1 || paged.IntervalTotal != 2 {
		t.Fatalf("unexpected paged detail: %+v", paged)
	}
}

func TestStoreMarkStaleRunningAndFilters(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.CreateRun(testRun("upload-run", model.DirectionUpload, model.RunStatusCompleted)); err != nil {
		t.Fatalf("create upload run: %v", err)
	}
	if err := store.CreateRun(testRun("download-run", model.DirectionDownload, model.RunStatusRunning)); err != nil {
		t.Fatalf("create download run: %v", err)
	}
	affected, err := store.MarkStaleRunning()
	if err != nil {
		t.Fatalf("mark stale running: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected 1 stale run, got %d", affected)
	}
	result, err := store.Get("download-run", 1, 10)
	if err != nil {
		t.Fatalf("get stale run: %v", err)
	}
	if result.Run.Status != model.RunStatusInterrupted {
		t.Fatalf("expected interrupted status, got %s", result.Run.Status)
	}

	page, err := store.List(model.HistoryFilter{Page: 1, PageSize: 50, Direction: "download"})
	if err != nil {
		t.Fatalf("filter direction: %v", err)
	}
	if page.Total != 1 || page.Items[0].ID != "download-run" {
		t.Fatalf("unexpected direction filter result: %+v", page)
	}

	page, err = store.List(model.HistoryFilter{Page: 1, PageSize: 50, Query: "127.0.0.1", Protocol: "tcp"})
	if err != nil {
		t.Fatalf("filter host/protocol: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected 2 host/protocol matches, got %d", page.Total)
	}
}

func TestStoreDeleteAndClear(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if err := store.CreateRun(testRun("run-1", model.DirectionUpload, model.RunStatusCompleted)); err != nil {
		t.Fatalf("create run 1: %v", err)
	}
	if err := store.CreateRun(testRun("run-2", model.DirectionUpload, model.RunStatusCompleted)); err != nil {
		t.Fatalf("create run 2: %v", err)
	}
	if err := store.AppendIntervals("run-1", []model.Interval{{RunID: "run-1", Sequence: 1, Direction: model.DirectionUpload, BitsPerSecond: 1000}}); err != nil {
		t.Fatalf("append intervals: %v", err)
	}

	deleted, err := store.Delete([]string{"run-1", "missing"})
	if err != nil || deleted != 1 {
		t.Fatalf("delete history: %v deleted=%d", err, deleted)
	}
	result, err := store.Get("run-1", 1, 10)
	if err != nil {
		t.Fatalf("get deleted run: %v", err)
	}
	if result.Error == nil || result.Error.Code != model.ErrHistoryNotFound {
		t.Fatalf("expected history-not-found, got %+v", result.Error)
	}

	cleared, err := store.Clear()
	if err != nil || cleared != 1 {
		t.Fatalf("clear history: %v cleared=%d", err, cleared)
	}
	page, err := store.List(model.HistoryFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list after clear: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("expected empty history, got %d", page.Total)
	}
}
