//go:build !windows

package utils

import (
	"os"
	"syscall"
)

func Kill(process *os.Process) error {
	return syscall.Kill(process.Pid, syscall.SIGKILL)
}
