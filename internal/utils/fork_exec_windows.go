//go:build windows

package utils

import (
	"os/exec"
)

func ForkExec(path string, args []string) error {
	cmd := exec.Command("cmd.exe", append([]string{"/C", "start", "/b", path}, args...)...)

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}
