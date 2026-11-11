package engine

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed assets/*
var bundled embed.FS

const BundleVersion = "3.22"

var requiredFiles = []string{
	"iperf3.exe",
	"kill.exe",
	"cygwin1.dll",
	"cygcrypto-3.dll",
	"cygz.dll",
}

func EnsureBundled() (string, error) {
	base, err := appDataDir()
	if err != nil {
		return "", err
	}
	engineDir := filepath.Join(base, "engine", BundleVersion)
	exePath := filepath.Join(engineDir, "iperf3.exe")

	if bundledComplete(engineDir) {
		return exePath, nil
	}
	if err := os.MkdirAll(engineDir, 0o755); err != nil {
		return "", fmt.Errorf("创建引擎目录失败: %w", err)
	}
	entries, err := fs.ReadDir(bundled, "assets")
	if err != nil {
		return "", fmt.Errorf("读取内置引擎失败: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := bundled.ReadFile("assets/" + entry.Name())
		if err != nil {
			return "", fmt.Errorf("读取内置文件 %s 失败: %w", entry.Name(), err)
		}
		target := filepath.Join(engineDir, entry.Name())
		if err := os.WriteFile(target, data, 0o755); err != nil {
			return "", fmt.Errorf("释放引擎文件 %s 失败: %w", entry.Name(), err)
		}
	}
	if !bundledComplete(engineDir) {
		return "", fmt.Errorf("内置引擎释放不完整: %s", engineDir)
	}
	return exePath, nil
}

func AppDataDir() (string, error) {
	return appDataDir()
}

func appDataDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("无法确定应用数据目录: %w", err)
		}
	}
	return filepath.Join(base, "Iperf3GUI"), nil
}

func bundledComplete(dir string) bool {
	for _, name := range requiredFiles {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.IsDir() || info.Size() == 0 {
			return false
		}
	}
	return true
}
