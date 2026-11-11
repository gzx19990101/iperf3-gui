package iperf

import (
	"strings"
	"testing"

	"iperf3-gui/internal/model"
)

func validClientConfig() model.ClientConfig {
	return model.ClientConfig{
		Host:            "127.0.0.1",
		Port:            5201,
		Protocol:        model.ProtocolTCP,
		Direction:       model.DirectionUpload,
		DurationSeconds: 10,
		IntervalSeconds: 1,
		ParallelStreams: 1,
	}
}

func TestValidateClientConfigAcceptsValidInputs(t *testing.T) {
	tests := []model.ClientConfig{
		validClientConfig(),
		func() model.ClientConfig {
			cfg := validClientConfig()
			cfg.Direction = model.DirectionDownload
			return cfg
		}(),
		func() model.ClientConfig {
			cfg := validClientConfig()
			cfg.Direction = model.DirectionBidirectional
			return cfg
		}(),
		func() model.ClientConfig {
			cfg := validClientConfig()
			cfg.DurationSeconds = 0
			return cfg
		}(),
		func() model.ClientConfig {
			cfg := validClientConfig()
			cfg.Protocol = model.ProtocolUDP
			cfg.UDPBandwidthBitsPerSecond = 100_000_000
			cfg.UDPDatagramLengthBytes = 1200
			return cfg
		}(),
		func() model.ClientConfig {
			cfg := validClientConfig()
			cfg.Host = "2001:db8::1"
			cfg.BindAddress = "::1"
			cfg.IPVersion = 6
			return cfg
		}(),
	}
	for index, cfg := range tests {
		if appErr := ValidateClientConfig(&cfg); appErr != nil {
			t.Fatalf("case %d: unexpected error: %+v", index, appErr)
		}
	}
}

func TestValidateClientConfigRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.ClientConfig)
		code   string
	}{
		{"empty host", func(c *model.ClientConfig) { c.Host = "  " }, model.ErrInvalidHost},
		{"host starts with dash", func(c *model.ClientConfig) { c.Host = "-oops" }, model.ErrInvalidHost},
		{"host control char", func(c *model.ClientConfig) { c.Host = "a\x00b" }, model.ErrInvalidHost},
		{"port zero", func(c *model.ClientConfig) { c.Port = 0 }, model.ErrInvalidPort},
		{"port too large", func(c *model.ClientConfig) { c.Port = 70000 }, model.ErrInvalidPort},
		{"invalid protocol", func(c *model.ClientConfig) { c.Protocol = "sctp" }, model.ErrInvalidValue},
		{"invalid direction", func(c *model.ClientConfig) { c.Direction = "sideways" }, model.ErrInvalidValue},
		{"duration negative", func(c *model.ClientConfig) { c.DurationSeconds = -1 }, model.ErrInvalidValue},
		{"duration too large", func(c *model.ClientConfig) { c.DurationSeconds = 3601 }, model.ErrInvalidValue},
		{"interval zero", func(c *model.ClientConfig) { c.IntervalSeconds = 0 }, model.ErrInvalidValue},
		{"parallel zero", func(c *model.ClientConfig) { c.ParallelStreams = 0 }, model.ErrInvalidValue},
		{"parallel too large", func(c *model.ClientConfig) { c.ParallelStreams = 129 }, model.ErrInvalidValue},
		{"invalid ip version", func(c *model.ClientConfig) { c.IPVersion = 5 }, model.ErrInvalidValue},
		{"bind dash", func(c *model.ClientConfig) { c.BindAddress = "-x" }, model.ErrInvalidBind},
		{"bind family conflict", func(c *model.ClientConfig) { c.BindAddress = "::1"; c.IPVersion = 4 }, model.ErrConfigConflict},
		{"udp no bandwidth", func(c *model.ClientConfig) { c.Protocol = model.ProtocolUDP }, model.ErrInvalidValue},
		{"udp bad length", func(c *model.ClientConfig) {
			c.Protocol = model.ProtocolUDP
			c.UDPBandwidthBitsPerSecond = 1000
			c.UDPDatagramLengthBytes = 70000
		}, model.ErrInvalidValue},
	}
	for _, test := range tests {
		cfg := validClientConfig()
		test.mutate(&cfg)
		appErr := ValidateClientConfig(&cfg)
		if appErr == nil {
			t.Fatalf("%s: expected error", test.name)
		}
		if appErr.Code != test.code {
			t.Fatalf("%s: expected code %s, got %s (%s)", test.name, test.code, appErr.Code, appErr.Message)
		}
	}
}

func TestValidateServerConfig(t *testing.T) {
	cfg := model.ServerConfig{Port: 5201}
	if appErr := ValidateServerConfig(&cfg); appErr != nil {
		t.Fatalf("unexpected error: %+v", appErr)
	}
	cfg.Port = 0
	if appErr := ValidateServerConfig(&cfg); appErr == nil || appErr.Code != model.ErrInvalidPort {
		t.Fatalf("expected invalid port, got %+v", appErr)
	}
	cfg.Port = 5201
	cfg.BindAddress = "-bad"
	if appErr := ValidateServerConfig(&cfg); appErr == nil || appErr.Code != model.ErrInvalidBind {
		t.Fatalf("expected invalid bind, got %+v", appErr)
	}
}

func TestDirectionSupported(t *testing.T) {
	caps := model.EngineCapabilities{BidirTCP: true, BidirUDP: false}
	if !DirectionSupported(caps, model.ProtocolTCP, model.DirectionBidirectional) {
		t.Fatal("expected TCP bidirectional to be supported")
	}
	if DirectionSupported(caps, model.ProtocolUDP, model.DirectionBidirectional) {
		t.Fatal("expected UDP bidirectional to be unsupported when capability is false")
	}
	caps.BidirUDP = true
	if !DirectionSupported(caps, model.ProtocolUDP, model.DirectionBidirectional) {
		t.Fatal("expected UDP bidirectional to be supported when capability is true")
	}
	if !DirectionSupported(caps, model.ProtocolUDP, model.DirectionUpload) {
		t.Fatal("upload should always be supported")
	}
	appErr := UnsupportedDirectionError(model.ProtocolUDP)
	if appErr.Code != model.ErrUnsupportedDirection || !strings.Contains(appErr.Message, "UDP") {
		t.Fatalf("unexpected error: %+v", appErr)
	}
}
