package iperf

import (
	"slices"
	"strings"
	"testing"

	"iperf3-gui/internal/model"
)

func containsPair(args []string, first, second string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == first && args[index+1] == second {
			return true
		}
	}
	return false
}

func TestBuildClientArgsDirectionAndProtocol(t *testing.T) {
	base := model.ClientConfig{
		Host:            "example.com",
		Port:            5201,
		Protocol:        model.ProtocolTCP,
		Direction:       model.DirectionUpload,
		DurationSeconds: 10,
		IntervalSeconds: 1,
		ParallelStreams: 4,
	}
	upload := BuildClientArgs(base, true)
	if !containsPair(upload, "-c", "example.com") || !containsPair(upload, "-P", "4") {
		t.Fatalf("base args missing values: %v", upload)
	}
	if slices.Contains(upload, "-R") || slices.Contains(upload, "--bidir") {
		t.Fatalf("upload should not contain direction flags: %v", upload)
	}
	if !slices.Contains(upload, "--json-stream") || !slices.Contains(upload, "--forceflush") {
		t.Fatalf("stream flags missing: %v", upload)
	}

	downloadConfig := base
	downloadConfig.Direction = model.DirectionDownload
	download := BuildClientArgs(downloadConfig, true)
	if !slices.Contains(download, "-R") {
		t.Fatalf("download should contain -R: %v", download)
	}

	bidirConfig := base
	bidirConfig.Direction = model.DirectionBidirectional
	bidir := BuildClientArgs(bidirConfig, true)
	if !slices.Contains(bidir, "--bidir") {
		t.Fatalf("bidirectional should contain --bidir: %v", bidir)
	}

	udpConfig := base
	udpConfig.Protocol = model.ProtocolUDP
	udpConfig.UDPBandwidthBitsPerSecond = 100_000_000
	udpConfig.UDPDatagramLengthBytes = 1200
	udp := BuildClientArgs(udpConfig, true)
	if !slices.Contains(udp, "-u") || !containsPair(udp, "-b", "100000000") || !containsPair(udp, "-l", "1200") {
		t.Fatalf("udp args missing values: %v", udp)
	}

	udpBidirConfig := udpConfig
	udpBidirConfig.Direction = model.DirectionBidirectional
	udpBidir := BuildClientArgs(udpBidirConfig, true)
	if !slices.Contains(udpBidir, "-u") || !slices.Contains(udpBidir, "--bidir") {
		t.Fatalf("udp bidirectional args missing values: %v", udpBidir)
	}

	indefiniteConfig := base
	indefiniteConfig.DurationSeconds = 0
	indefinite := BuildClientArgs(indefiniteConfig, true)
	if !containsPair(indefinite, "-t", "0") {
		t.Fatalf("indefinite duration should produce -t 0: %v", indefinite)
	}

	finalJSON := BuildClientArgs(base, false)
	if !slices.Contains(finalJSON, "-J") || slices.Contains(finalJSON, "--json-stream") {
		t.Fatalf("final JSON mode wrong: %v", finalJSON)
	}
}

func TestBuildClientArgsBindAndIPVersion(t *testing.T) {
	cfg := model.ClientConfig{
		Host:            "2001:db8::1",
		Port:            5201,
		Protocol:        model.ProtocolTCP,
		Direction:       model.DirectionUpload,
		DurationSeconds: 5,
		IntervalSeconds: 1,
		ParallelStreams: 1,
		BindAddress:     "::1",
		IPVersion:       6,
	}
	args := BuildClientArgs(cfg, true)
	if !containsPair(args, "-B", "::1") || !slices.Contains(args, "-6") {
		t.Fatalf("bind/ip args missing: %v", args)
	}
}

func TestBuildServerArgs(t *testing.T) {
	cfg := model.ServerConfig{Port: 5201, BindAddress: "127.0.0.1", OneOff: true}
	args := BuildServerArgs(cfg, true)
	for _, expected := range []string{"-s", "-p", "5201", "-B", "127.0.0.1", "-1", "--json-stream", "--forceflush"} {
		if !slices.Contains(args, expected) {
			t.Fatalf("server args missing %q: %v", expected, args)
		}
	}
	withoutStream := BuildServerArgs(cfg, false)
	if slices.Contains(withoutStream, "--json-stream") {
		t.Fatalf("non-stream server args should not contain --json-stream: %v", withoutStream)
	}
}

func TestPreview(t *testing.T) {
	preview := Preview([]string{"-c", "example.com", "-T", "hello world"})
	if !strings.HasPrefix(preview, "iperf3 ") {
		t.Fatalf("unexpected preview: %s", preview)
	}
	if !strings.Contains(preview, `"hello world"`) {
		t.Fatalf("preview should quote spaces: %s", preview)
	}
}
