package client

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
)

var (
	ErrNoWorkingMPVExecutableFound = errors.New("could not find working a working mpv executable")
)

func DiscoverMPVExecutable() (string, error) {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		if err := exec.Command("flatpak-spawn", "--host", "mpv", "--version").Run(); err == nil {
			return "flatpak-spawn --host mpv", nil
		}

		if err := exec.Command("flatpak-spawn", "--host", "flatpak", "run", "io.mpv.Mpv", "--version").Run(); err == nil {
			return "flatpak-spawn --host flatpak run io.mpv.Mpv", nil
		}

		return "", ErrNoWorkingMPVExecutableFound
	}

	if err := exec.Command("mpv", "--version").Run(); err == nil {
		return "mpv", nil
	}

	if err := exec.Command("flatpak", "run", "io.mpv.Mpv", "--version").Run(); err == nil {
		return "flatpak run io.mpv.Mpv", nil
	}

	return "", ErrNoWorkingMPVExecutableFound
}

func ExecuteMPVRequest(ipcFile string, command func(encoder *json.Encoder, decoder *json.Decoder) error) error {
	sock, err := net.Dial("unix", ipcFile)
	if err != nil {
		return err
	}
	defer sock.Close()

	encoder := json.NewEncoder(sock)
	decoder := json.NewDecoder(sock)

	return command(encoder, decoder)
}
