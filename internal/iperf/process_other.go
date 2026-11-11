//go:build !windows

package iperf

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}

func ConfigureProcess(cmd *exec.Cmd) {}
