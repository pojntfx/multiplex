//go:build windows

package utils

import (
	"os"
)

func Kill(process *os.Process) error {
	return process.Kill()
}
