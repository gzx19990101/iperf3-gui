package iperf

import (
	"fmt"
	"net"
	"strings"
	"unicode"

	"iperf3-gui/internal/model"
)

func ValidateClientConfig(cfg *model.ClientConfig) *model.AppError {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.BindAddress = strings.TrimSpace(cfg.BindAddress)

	if cfg.Host == "" || strings.HasPrefix(cfg.Host, "-") || hasControl(cfg.Host) {
		return configError(model.ErrInvalidHost, "目标地址不能为空、不能以 - 开头，且不能包含控制字符")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return configError(model.ErrInvalidPort, "端口必须是 1-65535 的整数")
	}
	if cfg.Protocol != model.ProtocolTCP && cfg.Protocol != model.ProtocolUDP {
		return configError(model.ErrInvalidValue, "协议必须是 TCP 或 UDP")
	}
	switch cfg.Direction {
	case model.DirectionUpload, model.DirectionDownload, model.DirectionBidirectional:
	default:
		return configError(model.ErrInvalidValue, "方向必须是上传、下载或双向")
	}
	if cfg.DurationSeconds < 0 || cfg.DurationSeconds > 3600 {
		return configError(model.ErrInvalidValue, "时长必须在 0-3600 秒之间，0 表示不限时")
	}
	if cfg.IntervalSeconds < 1 || cfg.IntervalSeconds > 3600 {
		return configError(model.ErrInvalidValue, "报告间隔必须在 1-3600 秒之间")
	}
	if cfg.ParallelStreams < 1 || cfg.ParallelStreams > 128 {
		return configError(model.ErrInvalidValue, "并行流必须在 1-128 之间")
	}
	if cfg.IPVersion != 0 && cfg.IPVersion != 4 && cfg.IPVersion != 6 {
		return configError(model.ErrInvalidValue, "IP 版本只能选择自动、IPv4 或 IPv6")
	}
	if cfg.BindAddress != "" {
		if strings.HasPrefix(cfg.BindAddress, "-") || hasControl(cfg.BindAddress) {
			return configError(model.ErrInvalidBind, "绑定地址不能以 - 开头，且不能包含控制字符")
		}
		if ip := net.ParseIP(cfg.BindAddress); ip != nil {
			is4 := ip.To4() != nil
			if cfg.IPVersion == 4 && !is4 {
				return configError(model.ErrConfigConflict, "绑定地址是 IPv6，但 IP 版本选择了 IPv4")
			}
			if cfg.IPVersion == 6 && is4 {
				return configError(model.ErrConfigConflict, "绑定地址是 IPv4，但 IP 版本选择了 IPv6")
			}
		}
	}
	if cfg.Protocol == model.ProtocolUDP {
		if cfg.UDPBandwidthBitsPerSecond <= 0 {
			return configError(model.ErrInvalidValue, "UDP 目标带宽必须大于 0")
		}
		if cfg.UDPDatagramLengthBytes < 1 || cfg.UDPDatagramLengthBytes > 65507 {
			return configError(model.ErrInvalidValue, "UDP 数据报长度必须在 1-65507 字节之间")
		}
	}
	return nil
}

func ValidateServerConfig(cfg *model.ServerConfig) *model.AppError {
	cfg.BindAddress = strings.TrimSpace(cfg.BindAddress)
	if cfg.Port < 1 || cfg.Port > 65535 {
		return configError(model.ErrInvalidPort, "监听端口必须是 1-65535 的整数")
	}
	if cfg.BindAddress != "" {
		if strings.HasPrefix(cfg.BindAddress, "-") || hasControl(cfg.BindAddress) {
			return configError(model.ErrInvalidBind, "绑定地址不能以 - 开头，且不能包含控制字符")
		}
	}
	return nil
}

func configError(code, message string) *model.AppError {
	return &model.AppError{Code: code, Stage: "config", Message: message}
}

func hasControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func DirectionSupported(caps model.EngineCapabilities, protocol model.Protocol, direction model.Direction) bool {
	if direction != model.DirectionBidirectional {
		return true
	}
	if protocol == model.ProtocolUDP {
		return caps.BidirUDP
	}
	return caps.BidirTCP
}

func UnsupportedDirectionError(protocol model.Protocol) *model.AppError {
	return &model.AppError{
		Code:    model.ErrUnsupportedDirection,
		Stage:   "config",
		Message: fmt.Sprintf("当前随包 iPerf3 不支持 %s 双向测试，请改用上传或下载模式", strings.ToUpper(string(protocol))),
	}
}
