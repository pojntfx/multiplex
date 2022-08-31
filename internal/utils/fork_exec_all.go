//go:build !windows

package utils

import (
	"os"
	"syscall"
)

func ForkExec(path string, args []string) error {
	if _, err := syscall.ForkExec(
		path,
		args,
		&syscall.ProcAttr{
			Env:   os.Environ(),
			Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
		},
	); err != nil {
		return err
	}

	return nil
}
