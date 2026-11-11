package iperf

import (
	"encoding/json"
	"strings"
	"testing"

	"iperf3-gui/internal/model"
)

func baseClientConfig(protocol model.Protocol, direction model.Direction) model.ClientConfig {
	return model.ClientConfig{
		Host:            "127.0.0.1",
		Port:            5201,
		Protocol:        protocol,
		Direction:       direction,
		DurationSeconds: 2,
		IntervalSeconds: 1,
		ParallelStreams: 1,
	}
}

func decodeIntervalEvent(t *testing.T, line string) IntervalData {
	t.Helper()
	event, err := ParseStreamEvent([]byte(line))
	if err != nil {
		t.Fatalf("parse event: %v", err)
	}
	var data IntervalData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		t.Fatalf("decode interval: %v", err)
	}
	return data
}

func TestParseVersionAndCapabilities(t *testing.T) {
	version := ParseVersion("iperf 3.22 (cJSON 1.7.15)\nCYGWIN_NT-10.0")
	if version != "3.22" {
		t.Fatalf("unexpected version: %q", version)
	}
	caps := DetectCapabilities(version, "--json-stream\n--json-stream-full-output\n--bidir\n-u, --udp")
	if !caps.JSONStream || !caps.JSONStreamFullOutput || !caps.BidirTCP || !caps.UDP {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	if !caps.BidirUDP {
		t.Fatal("expected UDP bidirectional client capability")
	}
}

func TestBuildIntervalsForDirections(t *testing.T) {
	uploadLine := `{"event":"interval","data":{"sum":{"start":0,"end":1,"seconds":1,"bytes":1000,"bits_per_second":8000,"sender":true}}}`
	upload := BuildIntervals("run", 1, baseClientConfig(model.ProtocolTCP, model.DirectionUpload), decodeIntervalEvent(t, uploadLine))
	if len(upload) != 1 || upload[0].Direction != model.DirectionUpload || upload[0].BitsPerSecond != 8000 {
		t.Fatalf("unexpected upload intervals: %+v", upload)
	}

	downloadLine := `{"event":"interval","data":{"sum":{"start":0,"end":1,"seconds":1,"bytes":2000,"bits_per_second":16000,"sender":false}}}`
	download := BuildIntervals("run", 2, baseClientConfig(model.ProtocolTCP, model.DirectionDownload), decodeIntervalEvent(t, downloadLine))
	if len(download) != 1 || download[0].Direction != model.DirectionDownload || download[0].BitsPerSecond != 16000 {
		t.Fatalf("unexpected download intervals: %+v", download)
	}

	bidirLine := `{"event":"interval","data":{"sum":{"start":0,"end":1,"seconds":1,"bytes":1000,"bits_per_second":8000,"sender":true},"sum_bidir_reverse":{"start":0,"end":1,"seconds":1,"bytes":2000,"bits_per_second":16000,"sender":false}}}`
	bidir := BuildIntervals("run", 3, baseClientConfig(model.ProtocolTCP, model.DirectionBidirectional), decodeIntervalEvent(t, bidirLine))
	if len(bidir) != 2 {
		t.Fatalf("expected two bidirectional intervals, got %+v", bidir)
	}
	if bidir[0].Direction != model.DirectionUpload || bidir[0].BitsPerSecond != 8000 {
		t.Fatalf("unexpected upload leg: %+v", bidir[0])
	}
	if bidir[1].Direction != model.DirectionDownload || bidir[1].BitsPerSecond != 16000 {
		t.Fatalf("unexpected download leg: %+v", bidir[1])
	}
}

func TestBuildSummaryForDirections(t *testing.T) {
	uploadEnd := EndData{
		SumSent:     &StreamMetrics{BitsPerSecond: 4000, Retransmits: 2},
		SumReceived: &StreamMetrics{BitsPerSecond: 3600, Bytes: 900, Seconds: 2},
	}
	uploadSummary := BuildSummary(baseClientConfig(model.ProtocolTCP, model.DirectionUpload), uploadEnd)
	if uploadSummary == nil || uploadSummary.Upload == nil {
		t.Fatalf("missing upload summary: %+v", uploadSummary)
	}
	if uploadSummary.Upload.BitsPerSecond != 3600 || uploadSummary.Upload.TCPRetransmits != 2 {
		t.Fatalf("unexpected upload summary: %+v", uploadSummary.Upload)
	}

	downloadEnd := EndData{
		SumSent:     &StreamMetrics{BitsPerSecond: 4800, Retransmits: 3},
		SumReceived: &StreamMetrics{BitsPerSecond: 5000, Bytes: 1250, Seconds: 2},
	}
	downloadSummary := BuildSummary(baseClientConfig(model.ProtocolTCP, model.DirectionDownload), downloadEnd)
	if downloadSummary == nil || downloadSummary.Download == nil {
		t.Fatalf("missing download summary: %+v", downloadSummary)
	}
	if downloadSummary.Download.BitsPerSecond != 5000 || downloadSummary.Download.TCPRetransmits != 3 {
		t.Fatalf("unexpected download summary: %+v", downloadSummary.Download)
	}

	bidirEnd := EndData{
		SumReceived:             &StreamMetrics{BitsPerSecond: 3900},
		SumReceivedBidirReverse: &StreamMetrics{BitsPerSecond: 4900},
	}
	bidirSummary := BuildSummary(baseClientConfig(model.ProtocolTCP, model.DirectionBidirectional), bidirEnd)
	if bidirSummary == nil || bidirSummary.Upload == nil || bidirSummary.Download == nil {
		t.Fatalf("missing bidirectional summary: %+v", bidirSummary)
	}
	if bidirSummary.Upload.BitsPerSecond != 3900 || bidirSummary.Download.BitsPerSecond != 4900 {
		t.Fatalf("unexpected bidirectional summary: %+v", bidirSummary)
	}
}

func TestBuildSummaryUDP(t *testing.T) {
	end := EndData{
		SumReceived: &StreamMetrics{
			BitsPerSecond: 9_933_330.38,
			Bytes:         2_498_060,
			Seconds:       2,
			JitterMS:      0.0500263,
			LostPackets:   2,
			Packets:       1711,
			LostPercent:   0.1169,
		},
	}
	summary := BuildSummary(baseClientConfig(model.ProtocolUDP, model.DirectionUpload), end)
	if summary == nil || summary.Upload == nil {
		t.Fatalf("missing UDP summary: %+v", summary)
	}
	upload := summary.Upload
	if upload.BitsPerSecond != 9_933_330.38 || upload.UDPJitterMilliseconds != 0.0500263 {
		t.Fatalf("unexpected UDP throughput/jitter: %+v", upload)
	}
	if upload.UDPLostDatagrams != 2 || upload.UDPTotalDatagrams != 1711 || upload.UDPLostPercent != 0.1169 {
		t.Fatalf("unexpected UDP loss: %+v", upload)
	}
}

func TestParseServerStart(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		client    string
		protocol  model.Protocol
		direction model.Direction
		duration  int
	}{
		{
			name:      "tcp upload",
			payload:   `{"accepted_connection":{"host":"192.168.1.20","port":50000},"test_start":{"protocol":"TCP","reverse":0,"bidir":0,"duration":10}}`,
			client:    "192.168.1.20:50000",
			protocol:  model.ProtocolTCP,
			direction: model.DirectionUpload,
			duration:  10,
		},
		{
			name:      "udp download",
			payload:   `{"accepted_connection":{"host":"::1","port":50001},"test_start":{"protocol":"UDP","reverse":1,"bidir":0,"duration":5}}`,
			client:    "[::1]:50001",
			protocol:  model.ProtocolUDP,
			direction: model.DirectionDownload,
			duration:  5,
		},
		{
			name:      "tcp bidir",
			payload:   `{"accepted_connection":{"host":"10.0.0.2","port":50002},"test_start":{"protocol":"TCP","reverse":0,"bidir":1,"duration":8}}`,
			client:    "10.0.0.2:50002",
			protocol:  model.ProtocolTCP,
			direction: model.DirectionBidirectional,
			duration:  8,
		},
	}
	for _, test := range tests {
		info, err := ParseServerStart([]byte(test.payload))
		if err != nil {
			t.Fatalf("%s: parse: %v", test.name, err)
		}
		if info.Client != test.client || info.Protocol != test.protocol || info.Direction != test.direction || info.DurationSeconds != test.duration {
			t.Fatalf("%s: unexpected info: %+v", test.name, info)
		}
	}
}

func TestBuildSummaryServerPerspective(t *testing.T) {
	uploadEnd := EndData{
		SumSent:     &StreamMetrics{BitsPerSecond: 0, Bytes: 0},
		SumReceived: &StreamMetrics{BitsPerSecond: 1_000_000_000, Bytes: 250_000_000, Seconds: 2},
	}
	upload := BuildSummary(baseClientConfig(model.ProtocolTCP, model.DirectionUpload), uploadEnd)
	if upload == nil || upload.Upload == nil || upload.Upload.BitsPerSecond != 1_000_000_000 {
		t.Fatalf("unexpected server upload summary: %+v", upload)
	}

	downloadEnd := EndData{
		SumSent:     &StreamMetrics{BitsPerSecond: 2_000_000_000, Bytes: 500_000_000, Seconds: 2, Retransmits: 3},
		SumReceived: &StreamMetrics{BitsPerSecond: 0, Bytes: 0, Seconds: 2},
	}
	download := BuildSummary(baseClientConfig(model.ProtocolTCP, model.DirectionDownload), downloadEnd)
	if download == nil || download.Download == nil {
		t.Fatalf("missing server download summary: %+v", download)
	}
	if download.Download.BitsPerSecond != 2_000_000_000 || download.Download.TCPRetransmits != 3 {
		t.Fatalf("unexpected server download summary: %+v", download.Download)
	}

	bidirEnd := EndData{
		SumReceived:             &StreamMetrics{BitsPerSecond: 1_500_000_000},
		SumSent:                 &StreamMetrics{BitsPerSecond: 0},
		SumReceivedBidirReverse: &StreamMetrics{BitsPerSecond: 0},
		SumSentBidirReverse:     &StreamMetrics{BitsPerSecond: 2_500_000_000, Retransmits: 1},
	}
	bidir := BuildSummary(baseClientConfig(model.ProtocolTCP, model.DirectionBidirectional), bidirEnd)
	if bidir == nil || bidir.Upload == nil || bidir.Download == nil {
		t.Fatalf("missing server bidirectional summary: %+v", bidir)
	}
	if bidir.Upload.BitsPerSecond != 1_500_000_000 || bidir.Download.BitsPerSecond != 2_500_000_000 {
		t.Fatalf("unexpected server bidirectional summary: %+v", bidir)
	}
	if bidir.Download.TCPRetransmits != 1 {
		t.Fatalf("unexpected server bidirectional retransmits: %+v", bidir.Download)
	}
}

func TestParseFinalJSON(t *testing.T) {
	payload := `{"intervals":[{"sum":{"start":0,"end":1,"seconds":1,"bytes":1000,"bits_per_second":8000,"sender":true}}],"end":{"sum_received":{"start":0,"end":1,"seconds":1,"bytes":990,"bits_per_second":7900,"sender":true}}}`
	intervals, summary, err := ParseFinalJSON([]byte(payload), baseClientConfig(model.ProtocolTCP, model.DirectionUpload))
	if err != nil {
		t.Fatalf("parse final JSON: %v", err)
	}
	if len(intervals) != 1 || intervals[0].Direction != model.DirectionUpload {
		t.Fatalf("unexpected final intervals: %+v", intervals)
	}
	if summary == nil || summary.Upload == nil || summary.Upload.BitsPerSecond != 7900 {
		t.Fatalf("unexpected final summary: %+v", summary)
	}
}

func TestClassifyProcessFailure(t *testing.T) {
	tests := []struct {
		stderr string
		code   string
	}{
		{"iperf3: error - unable to start listener for connections: Address already in use", model.ErrProcessPortInUse},
		{"iperf3: error - unable to connect to server: Name or service not known", model.ErrProcessDNSFailed},
		{"iperf3: error - unable to connect to server: Connection timed out", model.ErrProcessConnectTimeout},
		{"iperf3: error - unable to connect to server: Connection refused", model.ErrProcessConnectFailed},
		{"something unexpected", model.ErrProcessUnexpectedExit},
	}
	for _, test := range tests {
		appErr := ClassifyProcessFailure(test.stderr, 1)
		if appErr.Code != test.code {
			t.Fatalf("stderr %q: expected %s, got %s", test.stderr, test.code, appErr.Code)
		}
	}
}

func TestSummarizeOutput(t *testing.T) {
	text, truncated := SummarizeOutput(strings.Repeat("x", 100), 50)
	if !truncated || len(text) != 50 {
		t.Fatalf("unexpected truncation result: len=%d truncated=%v", len(text), truncated)
	}
	text, truncated = SummarizeOutput("short", 50)
	if truncated || text != "short" {
		t.Fatalf("unexpected non-truncated result: %q %v", text, truncated)
	}
}
