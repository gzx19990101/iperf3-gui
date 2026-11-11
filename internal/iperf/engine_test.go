package iperf

import (
	"net"
	"strings"
	"testing"
)

func TestShouldIncludeInterface(t *testing.T) {
	tests := []struct {
		name  string
		iface net.Interface
		ip    net.IP
		want  bool
	}{
		{"loopback flag", net.Interface{Name: "Loopback", Flags: net.FlagLoopback}, net.ParseIP("192.168.1.5"), false},
		{"loopback ipv4", net.Interface{Name: "Ethernet"}, net.ParseIP("127.0.0.1"), false},
		{"loopback ipv6", net.Interface{Name: "Ethernet"}, net.ParseIP("::1"), false},
		{"bluetooth english", net.Interface{Name: "Bluetooth Network Connection"}, net.ParseIP("192.168.1.20"), false},
		{"bluetooth chinese", net.Interface{Name: "蓝牙网络连接"}, net.ParseIP("192.168.1.21"), false},
		{"ethernet", net.Interface{Name: "Ethernet"}, net.ParseIP("192.168.1.12"), true},
		{"wifi ipv6", net.Interface{Name: "Wi-Fi"}, net.ParseIP("2001:db8::1"), true},
		{"nil ip", net.Interface{Name: "Ethernet"}, nil, false},
	}
	for _, test := range tests {
		if got := shouldIncludeInterface(test.iface, test.ip); got != test.want {
			t.Fatalf("%s: expected %v, got %v", test.name, test.want, got)
		}
	}
}

func TestListInterfacesExcludesLoopbackAndBluetooth(t *testing.T) {
	for _, item := range ListInterfaces() {
		ip := net.ParseIP(item.Address)
		if ip != nil && ip.IsLoopback() {
			t.Fatalf("loopback address leaked into interface list: %+v", item)
		}
		name := strings.ToLower(item.Name)
		if strings.Contains(name, "bluetooth") || strings.Contains(item.Name, "蓝牙") {
			t.Fatalf("bluetooth interface leaked into interface list: %+v", item)
		}
	}
}
