//go:build !windows

package utils

import (
	"os/exec"
	"syscall"
)

func AddSysProcAttr(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
