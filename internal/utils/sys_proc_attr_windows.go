//go:build windows

package utils

import (
	"os/exec"
)

func AddSysProcAttr(command *exec.Cmd) {}
