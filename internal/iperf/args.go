package iperf

import (
	"strconv"
	"strings"

	"iperf3-gui/internal/model"
)

func BuildClientArgs(cfg model.ClientConfig, jsonStream bool) []string {
	args := []string{
		"-c", cfg.Host,
		"-p", strconv.Itoa(cfg.Port),
		"-t", strconv.Itoa(cfg.DurationSeconds),
		"-i", strconv.Itoa(cfg.IntervalSeconds),
		"-P", strconv.Itoa(cfg.ParallelStreams),
		"--connect-timeout", "5000",
	}

	if cfg.Protocol == model.ProtocolUDP {
		args = append(args,
			"-u",
			"-b", strconv.FormatInt(cfg.UDPBandwidthBitsPerSecond, 10),
			"-l", strconv.Itoa(cfg.UDPDatagramLengthBytes),
		)
	}

	switch cfg.Direction {
	case model.DirectionDownload:
		args = append(args, "-R")
	case model.DirectionBidirectional:
		args = append(args, "--bidir")
	}

	if cfg.BindAddress != "" {
		args = append(args, "-B", cfg.BindAddress)
	}
	switch cfg.IPVersion {
	case 4:
		args = append(args, "-4")
	case 6:
		args = append(args, "-6")
	}

	if jsonStream {
		args = append(args, "--json-stream", "--forceflush")
	} else {
		args = append(args, "-J")
	}
	return args
}

func BuildServerArgs(cfg model.ServerConfig, jsonStream bool) []string {
	args := []string{"-s", "-p", strconv.Itoa(cfg.Port)}
	if cfg.BindAddress != "" {
		args = append(args, "-B", cfg.BindAddress)
	}
	if cfg.OneOff {
		args = append(args, "-1")
	}
	if jsonStream {
		args = append(args, "--json-stream", "--forceflush")
	}
	return args
}

func Preview(args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, "iperf3")
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArg(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, " \t\"") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return value
}
