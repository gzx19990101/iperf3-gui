//go:build integration

package session_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"iperf3-gui/internal/engine"
	"iperf3-gui/internal/history"
	"iperf3-gui/internal/iperf"
	"iperf3-gui/internal/model"
	"iperf3-gui/internal/session"
)

type eventMessage struct {
	name string
	data interface{}
}

func TestTCPUploadRoundTrip(t *testing.T) {
	enginePath, err := engine.EnsureBundled()
	if err != nil {
		t.Fatalf("ensure engine: %v", err)
	}
	status := iperf.Probe(context.Background(), enginePath)
	if !status.Available {
		t.Fatalf("engine unavailable: %+v", status.Error)
	}

	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	defer store.Close()

	events := make(chan eventMessage, 256)
	manager := session.New(enginePath, status.Capabilities, store, func(name string, data ...interface{}) {
		var payload interface{}
		if len(data) > 0 {
			payload = data[0]
		}
		select {
		case events <- eventMessage{name: name, data: payload}:
		default:
		}
	})
	defer manager.Shutdown(context.Background())

	port := freePort(t)
	serverResult := manager.StartServer(model.ServerConfig{Port: port, BindAddress: "127.0.0.1"})
	if serverResult.Error != nil {
		t.Fatalf("start server: %+v", serverResult.Error)
	}
	waitForServerRunning(t, events)

	clientResult := manager.StartClientTest(model.ClientConfig{
		Host:            "127.0.0.1",
		Port:            port,
		Protocol:        model.ProtocolTCP,
		Direction:       model.DirectionUpload,
		DurationSeconds: 2,
		IntervalSeconds: 1,
		ParallelStreams: 1,
	})
	if clientResult.Error != nil {
		t.Fatalf("start client: %+v", clientResult.Error)
	}

	var completed model.RunCompletedEvent
	var failed model.RunFailedEvent
	deadline := time.After(30 * time.Second)
loop:
	for {
		select {
		case event := <-events:
			switch event.name {
			case "run:completed":
				completed = event.data.(model.RunCompletedEvent)
				break loop
			case "run:failed":
				failed = event.data.(model.RunFailedEvent)
				break loop
			}
		case <-deadline:
			t.Fatal("timed out waiting for client completion")
		}
	}
	if failed.Error != nil {
		t.Fatalf("client failed: %+v", failed.Error)
	}
	if completed.Summary == nil || completed.Summary.Upload == nil || completed.Summary.Upload.BitsPerSecond <= 0 {
		t.Fatalf("unexpected client summary: %+v", completed.Summary)
	}

	result, err := store.Get(clientResult.RunID, 1, 100)
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("history error: %+v", result.Error)
	}
	if result.Run.Status != model.RunStatusCompleted || result.IntervalTotal == 0 {
		t.Fatalf("unexpected history result: %+v", result)
	}
	if result.Run.Summary == nil || result.Run.Summary.Upload == nil {
		t.Fatalf("history summary missing: %+v", result.Run.Summary)
	}

	stopResult := manager.StopServer(serverResult.ServerID)
	if stopResult.Error != nil {
		t.Logf("stop server: %+v", stopResult.Error)
	}
}

func TestClientDirectionsAndUDP(t *testing.T) {
	store, manager, events := setupIntegration(t)
	defer store.Close()
	defer manager.Shutdown(context.Background())

	port := freePort(t)
	serverResult := manager.StartServer(model.ServerConfig{Port: port, BindAddress: "127.0.0.1"})
	if serverResult.Error != nil {
		t.Fatalf("start server: %+v", serverResult.Error)
	}
	waitForServerRunning(t, events)

	cases := []struct {
		name string
		cfg  model.ClientConfig
	}{
		{"tcp-upload", model.ClientConfig{Protocol: model.ProtocolTCP, Direction: model.DirectionUpload}},
		{"tcp-download", model.ClientConfig{Protocol: model.ProtocolTCP, Direction: model.DirectionDownload}},
		{"tcp-bidirectional", model.ClientConfig{Protocol: model.ProtocolTCP, Direction: model.DirectionBidirectional}},
		{"udp-upload", model.ClientConfig{Protocol: model.ProtocolUDP, Direction: model.DirectionUpload, UDPBandwidthBitsPerSecond: 10_000_000, UDPDatagramLengthBytes: 1200}},
		{"udp-download", model.ClientConfig{Protocol: model.ProtocolUDP, Direction: model.DirectionDownload, UDPBandwidthBitsPerSecond: 10_000_000, UDPDatagramLengthBytes: 1200}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			cfg := testCase.cfg
			cfg.Host = "127.0.0.1"
			cfg.Port = port
			cfg.DurationSeconds = 2
			cfg.IntervalSeconds = 1
			cfg.ParallelStreams = 1
			completed := runClientTest(t, manager, events, cfg)
			if completed.Summary == nil {
				t.Fatal("missing summary")
			}
			if completed.Summary.Direction != cfg.Direction {
				t.Fatalf("unexpected summary direction: %s", completed.Summary.Direction)
			}
			switch cfg.Direction {
			case model.DirectionUpload:
				if completed.Summary.Upload == nil || completed.Summary.Upload.BitsPerSecond <= 0 {
					t.Fatalf("unexpected upload summary: %+v", completed.Summary)
				}
			case model.DirectionDownload:
				if completed.Summary.Download == nil || completed.Summary.Download.BitsPerSecond <= 0 {
					t.Fatalf("unexpected download summary: %+v", completed.Summary)
				}
			case model.DirectionBidirectional:
				if completed.Summary.Upload == nil || completed.Summary.Download == nil {
					t.Fatalf("unexpected bidirectional summary: %+v", completed.Summary)
				}
				if completed.Summary.Upload.BitsPerSecond <= 0 || completed.Summary.Download.BitsPerSecond <= 0 {
					t.Fatalf("bidirectional legs must both have throughput: %+v", completed.Summary)
				}
			}
		})
	}

	stopResult := manager.StopServer(serverResult.ServerID)
	if stopResult.Error != nil {
		t.Logf("stop server: %+v", stopResult.Error)
	}
}

func TestServerTracksProtocolThroughputAndConnectionLifecycle(t *testing.T) {
	store, manager, events := setupIntegration(t)
	defer store.Close()
	defer manager.Shutdown(context.Background())

	port := freePort(t)
	serverResult := manager.StartServer(model.ServerConfig{Port: port, BindAddress: "127.0.0.1"})
	if serverResult.Error != nil {
		t.Fatalf("start server: %+v", serverResult.Error)
	}
	waitForServerRunning(t, events)

	udpDownload := model.ClientConfig{
		Host:                      "127.0.0.1",
		Port:                      port,
		Protocol:                  model.ProtocolUDP,
		Direction:                 model.DirectionDownload,
		DurationSeconds:           2,
		IntervalSeconds:           1,
		ParallelStreams:           1,
		UDPBandwidthBitsPerSecond: 10_000_000,
		UDPDatagramLengthBytes:    1200,
	}
	runClientTest(t, manager, events, udpDownload)
	udpSnapshot := waitForServerTest(t, manager, serverResult.ServerID, func(snapshot model.ServerSnapshot) bool {
		return snapshot.CurrentTest != nil &&
			snapshot.CurrentTest.Status == model.RunStatusCompleted &&
			snapshot.CurrentTest.Summary != nil
	})
	if udpSnapshot.CurrentTest.Protocol != model.ProtocolUDP {
		t.Fatalf("expected UDP server test, got %s", udpSnapshot.CurrentTest.Protocol)
	}
	if udpSnapshot.CurrentTest.Direction != model.DirectionDownload {
		t.Fatalf("expected download server test, got %s", udpSnapshot.CurrentTest.Direction)
	}
	if udpSnapshot.CurrentTest.Summary.Download == nil || udpSnapshot.CurrentTest.Summary.Download.BitsPerSecond <= 0 {
		t.Fatalf("unexpected UDP download summary: %+v", udpSnapshot.CurrentTest.Summary)
	}
	udpConnection := latestConnection(t, udpSnapshot)
	if udpConnection.Protocol != "UDP" || udpConnection.Direction != model.DirectionDownload {
		t.Fatalf("unexpected UDP connection metadata: %+v", udpConnection)
	}
	if udpConnection.Status != "已完成" || udpConnection.StartedAt == "" || udpConnection.EndedAt == "" {
		t.Fatalf("unexpected UDP connection lifecycle: %+v", udpConnection)
	}
	if udpConnection.DownloadBitsPerSecond <= 0 {
		t.Fatalf("unexpected UDP connection throughput: %+v", udpConnection)
	}

	tcpUpload := model.ClientConfig{
		Host:            "127.0.0.1",
		Port:            port,
		Protocol:        model.ProtocolTCP,
		Direction:       model.DirectionUpload,
		DurationSeconds: 2,
		IntervalSeconds: 1,
		ParallelStreams: 1,
	}
	runClientTest(t, manager, events, tcpUpload)
	tcpSnapshot := waitForServerTest(t, manager, serverResult.ServerID, func(snapshot model.ServerSnapshot) bool {
		return snapshot.CurrentTest != nil &&
			snapshot.CurrentTest.Protocol == model.ProtocolTCP &&
			snapshot.CurrentTest.Status == model.RunStatusCompleted &&
			snapshot.CurrentTest.Summary != nil
	})
	if tcpSnapshot.CurrentTest.Direction != model.DirectionUpload {
		t.Fatalf("expected upload server test, got %s", tcpSnapshot.CurrentTest.Direction)
	}
	if tcpSnapshot.CurrentTest.Summary.Upload == nil || tcpSnapshot.CurrentTest.Summary.Upload.BitsPerSecond <= 0 {
		t.Fatalf("unexpected TCP upload summary: %+v", tcpSnapshot.CurrentTest.Summary)
	}
	tcpConnection := latestConnection(t, tcpSnapshot)
	if tcpConnection.Protocol != "TCP" || tcpConnection.Direction != model.DirectionUpload {
		t.Fatalf("unexpected TCP connection metadata: %+v", tcpConnection)
	}
	if tcpConnection.Status != "已完成" || tcpConnection.EndedAt == "" {
		t.Fatalf("unexpected TCP connection lifecycle: %+v", tcpConnection)
	}
	if len(tcpSnapshot.Connections) < 2 {
		t.Fatalf("expected at least two connections, got %d", len(tcpSnapshot.Connections))
	}

	stopResult := manager.StopServer(serverResult.ServerID)
	if stopResult.Error != nil {
		t.Logf("stop server: %+v", stopResult.Error)
	}
}

func latestConnection(t *testing.T, snapshot model.ServerSnapshot) model.ServerConnection {
	t.Helper()
	if len(snapshot.Connections) == 0 {
		t.Fatal("missing server connection record")
	}
	return snapshot.Connections[len(snapshot.Connections)-1]
}

func waitForServerTest(t *testing.T, manager *session.Manager, serverID string, predicate func(model.ServerSnapshot) bool) model.ServerSnapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := manager.GetServerSnapshot(serverID)
		if snapshot.Error == nil && predicate(snapshot) {
			return snapshot
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for server test; last snapshot: %+v", manager.GetServerSnapshot(serverID))
	return model.ServerSnapshot{}
}

func TestStoppingClientKeepsServerRunning(t *testing.T) {
	store, manager, events := setupIntegration(t)
	defer store.Close()
	defer manager.Shutdown(context.Background())

	port := freePort(t)
	serverResult := manager.StartServer(model.ServerConfig{Port: port, BindAddress: "127.0.0.1"})
	if serverResult.Error != nil {
		t.Fatalf("start server: %+v", serverResult.Error)
	}
	waitForServerRunning(t, events)

	clientResult := manager.StartClientTest(model.ClientConfig{
		Host:            "127.0.0.1",
		Port:            port,
		Protocol:        model.ProtocolTCP,
		Direction:       model.DirectionUpload,
		DurationSeconds: 0,
		IntervalSeconds: 1,
		ParallelStreams: 1,
	})
	if clientResult.Error != nil {
		t.Fatalf("start client: %+v", clientResult.Error)
	}
	waitForRunStatus(t, manager, clientResult.RunID, model.RunStatusRunning)
	time.Sleep(700 * time.Millisecond)
	stopResult := manager.StopClientTest(clientResult.RunID)
	if stopResult.Error != nil {
		t.Fatalf("stop client: %+v", stopResult.Error)
	}
	tcpStopped := waitForRunStatus(t, manager, clientResult.RunID, model.RunStatusStopped)
	if tcpStopped.Summary == nil {
		t.Fatalf("missing TCP summary after graceful stop: %+v", tcpStopped)
	}
	time.Sleep(700 * time.Millisecond)

	serverSnapshot := manager.GetServerSnapshot(serverResult.ServerID)
	if serverSnapshot.Error != nil {
		t.Fatalf("server snapshot: %+v", serverSnapshot.Error)
	}
	if serverSnapshot.Status != model.RunStatusRunning {
		t.Fatalf("server stopped after TCP client stop: %+v", serverSnapshot)
	}

	udpClient := manager.StartClientTest(model.ClientConfig{
		Host:                      "127.0.0.1",
		Port:                      port,
		Protocol:                  model.ProtocolUDP,
		Direction:                 model.DirectionUpload,
		DurationSeconds:           0,
		IntervalSeconds:           1,
		ParallelStreams:           1,
		UDPBandwidthBitsPerSecond: 10_000_000,
		UDPDatagramLengthBytes:    1200,
	})
	if udpClient.Error != nil {
		t.Fatalf("start UDP client: %+v", udpClient.Error)
	}
	waitForRunStatus(t, manager, udpClient.RunID, model.RunStatusRunning)
	time.Sleep(700 * time.Millisecond)
	udpStopResult := manager.StopClientTest(udpClient.RunID)
	if udpStopResult.Error != nil {
		t.Fatalf("stop UDP client: %+v", udpStopResult.Error)
	}
	udpStopped := waitForRunStatus(t, manager, udpClient.RunID, model.RunStatusStopped)
	if udpStopped.Summary == nil {
		t.Fatalf("missing UDP summary after graceful stop: %+v", udpStopped)
	}
	time.Sleep(700 * time.Millisecond)
	udpServerSnapshot := manager.GetServerSnapshot(serverResult.ServerID)
	if udpServerSnapshot.Error != nil {
		t.Fatalf("UDP server snapshot: %+v", udpServerSnapshot.Error)
	}
	if udpServerSnapshot.Status != model.RunStatusRunning {
		t.Fatalf("server stopped after UDP client stop: %+v", udpServerSnapshot)
	}
}

func waitForRunStatus(t *testing.T, manager *session.Manager, runID string, status model.RunStatus) model.RunSnapshot {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := manager.GetRunSnapshot(runID)
		if snapshot.Error == nil && snapshot.Status == status {
			return snapshot
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for run %s status %s; last: %+v", runID, status, manager.GetRunSnapshot(runID))
	return model.RunSnapshot{}
}

func setupIntegration(t *testing.T) (*history.Store, *session.Manager, chan eventMessage) {
	t.Helper()
	enginePath, err := engine.EnsureBundled()
	if err != nil {
		t.Fatalf("ensure engine: %v", err)
	}
	status := iperf.Probe(context.Background(), enginePath)
	if !status.Available {
		t.Fatalf("engine unavailable: %+v", status.Error)
	}
	store, err := history.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	events := make(chan eventMessage, 1024)
	manager := session.New(enginePath, status.Capabilities, store, func(name string, data ...interface{}) {
		var payload interface{}
		if len(data) > 0 {
			payload = data[0]
		}
		select {
		case events <- eventMessage{name: name, data: payload}:
		default:
		}
	})
	return store, manager, events
}

func runClientTest(t *testing.T, manager *session.Manager, events <-chan eventMessage, cfg model.ClientConfig) model.RunCompletedEvent {
	t.Helper()
	result := manager.StartClientTest(cfg)
	if result.Error != nil {
		t.Fatalf("start client: %+v", result.Error)
	}
	deadline := time.After(30 * time.Second)
	for {
		select {
		case event := <-events:
			switch event.name {
			case "run:completed":
				completed := event.data.(model.RunCompletedEvent)
				if completed.RunID != result.RunID {
					continue
				}
				return completed
			case "run:failed":
				failed := event.data.(model.RunFailedEvent)
				if failed.RunID != result.RunID {
					continue
				}
				t.Fatalf("client failed: %+v", failed.Error)
			}
		case <-deadline:
			t.Fatal("timed out waiting for client completion")
		}
	}
}

func waitForServerRunning(t *testing.T, events <-chan eventMessage) {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for {
		select {
		case event := <-events:
			if event.name != "server:state" {
				continue
			}
			snapshot, ok := event.data.(model.ServerSnapshot)
			if ok && snapshot.Status == model.RunStatusRunning {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for server to listen")
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return port
}
