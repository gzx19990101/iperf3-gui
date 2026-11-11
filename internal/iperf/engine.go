package iperf

import (
	"context"
	"net"
	"os/exec"
	"sort"
	"strings"
	"time"

	"iperf3-gui/internal/model"
)

func Probe(ctx context.Context, path string) model.EngineStatus {
	status := model.EngineStatus{
		Path:       path,
		Interfaces: ListInterfaces(),
	}

	versionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	versionCmd := exec.CommandContext(versionCtx, path, "--version")
	configureProcess(versionCmd)
	versionOut, err := versionCmd.CombinedOutput()
	if err != nil {
		status.Error = &model.AppError{
			Code:    model.ErrIPerfProbeFailed,
			Stage:   "probe",
			Message: "无法执行随包 iPerf3，请修复安装",
			Detail:  strings.TrimSpace(string(versionOut)),
		}
		return status
	}
	version := ParseVersion(string(versionOut))
	if version == "" {
		status.Error = &model.AppError{
			Code:    model.ErrVersionUnsupported,
			Stage:   "probe",
			Message: "无法识别随包 iPerf3 版本，请修复安装",
			Detail:  strings.TrimSpace(string(versionOut)),
		}
		return status
	}

	helpCtx, cancelHelp := context.WithTimeout(ctx, 5*time.Second)
	defer cancelHelp()
	helpCmd := exec.CommandContext(helpCtx, path, "--help")
	configureProcess(helpCmd)
	helpOut, _ := helpCmd.CombinedOutput()

	status.Available = true
	status.Version = version
	status.Capabilities = DetectCapabilities(version, string(helpOut))
	return status
}

func ListInterfaces() []model.NetworkInterface {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	result := make([]model.NetworkInterface, 0, len(interfaces))
	for _, iface := range interfaces {
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ipNet, ok := address.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			if !shouldIncludeInterface(iface, ipNet.IP) {
				continue
			}
			family := "IPv6"
			if ipNet.IP.To4() != nil {
				family = "IPv4"
			}
			result = append(result, model.NetworkInterface{
				Name:    iface.Name,
				Address: ipNet.IP.String(),
				Family:  family,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].Address < result[j].Address
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func shouldIncludeInterface(iface net.Interface, ip net.IP) bool {
	if ip == nil || ip.IsLoopback() {
		return false
	}
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}
	name := strings.ToLower(iface.Name)
	if strings.Contains(name, "bluetooth") || strings.Contains(iface.Name, "蓝牙") {
		return false
	}
	return true
}
