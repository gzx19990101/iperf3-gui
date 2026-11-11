package iperf

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"iperf3-gui/internal/model"
)

const ParserVersion = "json-stream-1"

type StreamEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type StreamMetrics struct {
	Socket        int     `json:"socket"`
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	Seconds       float64 `json:"seconds"`
	Bytes         int64   `json:"bytes"`
	BitsPerSecond float64 `json:"bits_per_second"`
	Packets       int64   `json:"packets"`
	Retransmits   int64   `json:"retransmits"`
	Omitted       bool    `json:"omitted"`
	Sender        bool    `json:"sender"`
	JitterMS      float64 `json:"jitter_ms"`
	LostPackets   int64   `json:"lost_packets"`
	LostPercent   float64 `json:"lost_percent"`
}

type IntervalData struct {
	Streams         []StreamMetrics `json:"streams"`
	Sum             *StreamMetrics  `json:"sum"`
	SumBidirReverse *StreamMetrics  `json:"sum_bidir_reverse,omitempty"`
}

type EndData struct {
	Sum                     *StreamMetrics `json:"sum,omitempty"`
	SumSent                 *StreamMetrics `json:"sum_sent,omitempty"`
	SumReceived             *StreamMetrics `json:"sum_received,omitempty"`
	SumSentBidirReverse     *StreamMetrics `json:"sum_sent_bidir_reverse,omitempty"`
	SumReceivedBidirReverse *StreamMetrics `json:"sum_received_bidir_reverse,omitempty"`
}

func ParseStreamEvent(line []byte) (*StreamEvent, error) {
	var event StreamEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return nil, err
	}
	return &event, nil
}

type ServerStartData struct {
	AcceptedConnection struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	} `json:"accepted_connection"`
	TestStart struct {
		Protocol string `json:"protocol"`
		Reverse  int    `json:"reverse"`
		Bidir    int    `json:"bidir"`
		Duration int    `json:"duration"`
	} `json:"test_start"`
}

type ServerStartInfo struct {
	Client          string
	Protocol        model.Protocol
	Direction       model.Direction
	DurationSeconds int
}

func ParseServerStart(data json.RawMessage) (*ServerStartInfo, error) {
	var start ServerStartData
	if err := json.Unmarshal(data, &start); err != nil {
		return nil, err
	}
	protocol := model.ProtocolTCP
	if strings.EqualFold(start.TestStart.Protocol, "UDP") {
		protocol = model.ProtocolUDP
	}
	direction := model.DirectionUpload
	if start.TestStart.Bidir != 0 {
		direction = model.DirectionBidirectional
	} else if start.TestStart.Reverse != 0 {
		direction = model.DirectionDownload
	}
	client := "未知客户端"
	if start.AcceptedConnection.Host != "" {
		client = net.JoinHostPort(start.AcceptedConnection.Host, strconv.Itoa(start.AcceptedConnection.Port))
	}
	return &ServerStartInfo{
		Client:          client,
		Protocol:        protocol,
		Direction:       direction,
		DurationSeconds: start.TestStart.Duration,
	}, nil
}

func BuildIntervals(runID string, sequence int, cfg model.ClientConfig, data IntervalData) []model.Interval {
	switch cfg.Direction {
	case model.DirectionUpload:
		sum := data.Sum
		if sum == nil {
			sum = aggregateStreams(data.Streams, true)
		}
		return intervalFrom(runID, sequence, model.DirectionUpload, sum)
	case model.DirectionDownload:
		sum := data.Sum
		if sum == nil {
			sum = aggregateStreams(data.Streams, false)
		}
		return intervalFrom(runID, sequence, model.DirectionDownload, sum)
	case model.DirectionBidirectional:
		upload := data.Sum
		if upload == nil {
			upload = aggregateStreams(data.Streams, true)
		}
		download := data.SumBidirReverse
		if download == nil {
			download = aggregateStreams(data.Streams, false)
		}
		return append(
			intervalFrom(runID, sequence, model.DirectionUpload, upload),
			intervalFrom(runID, sequence, model.DirectionDownload, download)...,
		)
	default:
		return nil
	}
}

func BuildSummary(cfg model.ClientConfig, end EndData) *model.Summary {
	summary := &model.Summary{Direction: cfg.Direction}
	switch cfg.Direction {
	case model.DirectionUpload:
		summary.Upload = directionSummary(pickNonZero(end.SumReceived, end.SumSent, end.Sum), end.SumSent)
	case model.DirectionDownload:
		summary.Download = directionSummary(pickNonZero(end.SumReceived, end.SumSent, end.Sum), end.SumSent)
	case model.DirectionBidirectional:
		summary.Upload = directionSummary(pickNonZero(end.SumReceived, end.SumSent, end.Sum), end.SumSent)
		summary.Download = directionSummary(pickNonZero(end.SumReceivedBidirReverse, end.SumSentBidirReverse), end.SumSentBidirReverse)
	}
	if summary.Upload == nil && summary.Download == nil {
		return nil
	}
	return summary
}

func ParseFinalJSON(data []byte, cfg model.ClientConfig) ([]model.Interval, *model.Summary, error) {
	var final struct {
		Intervals []IntervalData `json:"intervals"`
		End       EndData        `json:"end"`
	}
	if err := json.Unmarshal(data, &final); err != nil {
		return nil, nil, err
	}
	var intervals []model.Interval
	for i, item := range final.Intervals {
		intervals = append(intervals, BuildIntervals("", i+1, cfg, item)...)
	}
	return intervals, BuildSummary(cfg, final.End), nil
}

func ParseVersion(output string) string {
	re := regexp.MustCompile(`\biperf\s+([0-9]+(?:\.[0-9]+)+)`)
	if match := re.FindStringSubmatch(output); len(match) > 1 {
		return match[1]
	}
	return ""
}

func DetectCapabilities(version, helpOutput string) model.EngineCapabilities {
	caps := model.EngineCapabilities{
		JSONStream:           strings.Contains(helpOutput, "--json-stream"),
		JSONStreamFullOutput: strings.Contains(helpOutput, "--json-stream-full-output"),
		BidirTCP:             strings.Contains(helpOutput, "--bidir"),
		UDP:                  strings.Contains(helpOutput, "--udp"),
	}
	// The Cygwin client supports UDP bidirectional, while the Cygwin server
	// has a known multi-stream bug (esnet/iperf#1992). Keep the capability for
	// client tests and warn about the local server limitation in the UI.
	caps.BidirUDP = strings.Contains(helpOutput, "--bidir")
	return caps
}

func ClassifyProcessFailure(stderr string, exitCode int) *model.AppError {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "address already in use"),
		strings.Contains(lower, "unable to start listener"):
		return &model.AppError{Code: model.ErrProcessPortInUse, Stage: "spawn", Message: "监听端口已被占用，请更换端口或结束占用进程", Detail: summarize(stderr)}
	case strings.Contains(lower, "name or service not known"),
		strings.Contains(lower, "nodename nor servname"),
		strings.Contains(lower, "getaddrinfo"),
		strings.Contains(lower, "no such host"):
		return &model.AppError{Code: model.ErrProcessDNSFailed, Stage: "connect", Message: "无法解析目标地址，请检查域名或 DNS 配置", Detail: summarize(stderr)}
	case strings.Contains(lower, "connection timed out"),
		strings.Contains(lower, "timeout"):
		return &model.AppError{Code: model.ErrProcessConnectTimeout, Stage: "connect", Message: "连接目标超时，请检查地址、端口和网络连通性", Detail: summarize(stderr)}
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "unable to connect to server"),
		strings.Contains(lower, "no route to host"),
		strings.Contains(lower, "network is unreachable"):
		return &model.AppError{Code: model.ErrProcessConnectFailed, Stage: "connect", Message: "无法连接到目标服务端，请确认服务端已启动且端口正确", Detail: summarize(stderr)}
	default:
		return &model.AppError{
			Code:    model.ErrProcessUnexpectedExit,
			Stage:   "run",
			Message: fmt.Sprintf("iPerf3 进程异常退出（退出码 %d）", exitCode),
			Detail:  summarize(stderr),
		}
	}
}

func SummarizeOutput(output string, limit int) (string, bool) {
	if len(output) <= limit {
		return output, false
	}
	return output[:limit], true
}

func summarize(stderr string) string {
	text, _ := SummarizeOutput(strings.TrimSpace(stderr), 4096)
	return text
}

func intervalFrom(runID string, sequence int, direction model.Direction, metrics *StreamMetrics) []model.Interval {
	if metrics == nil {
		return nil
	}
	bps := metrics.BitsPerSecond
	if bps == 0 && metrics.Seconds > 0 {
		bps = float64(metrics.Bytes) * 8 / metrics.Seconds
	}
	return []model.Interval{{
		RunID:                 runID,
		Sequence:              sequence,
		Direction:             direction,
		StartSeconds:          metrics.Start,
		EndSeconds:            metrics.End,
		BitsPerSecond:         bps,
		Bytes:                 metrics.Bytes,
		TCPRetransmits:        metrics.Retransmits,
		UDPJitterMilliseconds: metrics.JitterMS,
		UDPLostDatagrams:      metrics.LostPackets,
		UDPTotalDatagrams:     metrics.Packets,
	}}
}

func directionSummary(primary, retransmitSource *StreamMetrics) *model.DirectionSummary {
	if primary == nil {
		return nil
	}
	bps := primary.BitsPerSecond
	if bps == 0 && primary.Seconds > 0 {
		bps = float64(primary.Bytes) * 8 / primary.Seconds
	}
	item := &model.DirectionSummary{
		BitsPerSecond:         bps,
		Bytes:                 primary.Bytes,
		Seconds:               primary.Seconds,
		UDPJitterMilliseconds: primary.JitterMS,
		UDPLostDatagrams:      primary.LostPackets,
		UDPTotalDatagrams:     primary.Packets,
		UDPLostPercent:        primary.LostPercent,
	}
	if retransmitSource != nil {
		item.TCPRetransmits = retransmitSource.Retransmits
	}
	if item.TCPRetransmits == 0 {
		item.TCPRetransmits = primary.Retransmits
	}
	return item
}

func pick(values ...*StreamMetrics) *StreamMetrics {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func pickNonZero(values ...*StreamMetrics) *StreamMetrics {
	for _, value := range values {
		if value != nil && (value.BitsPerSecond > 0 || value.Bytes > 0) {
			return value
		}
	}
	return pick(values...)
}

func aggregateStreams(streams []StreamMetrics, sender bool) *StreamMetrics {
	var result StreamMetrics
	found := false
	for _, stream := range streams {
		if stream.Sender != sender {
			continue
		}
		if !found {
			result.Start = stream.Start
			found = true
		}
		result.End = stream.End
		result.Seconds += stream.Seconds
		result.Bytes += stream.Bytes
		result.BitsPerSecond += stream.BitsPerSecond
		result.Packets += stream.Packets
		result.Retransmits += stream.Retransmits
		result.JitterMS += stream.JitterMS
		result.LostPackets += stream.LostPackets
		result.LostPercent = stream.LostPercent
	}
	if !found {
		return nil
	}
	result.Sender = sender
	return &result
}
