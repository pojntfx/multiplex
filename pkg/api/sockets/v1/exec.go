package v1

import (
	"encoding/json"
	"net"
)

func RunMPVCommand(ipcFile string, command func(encoder *json.Encoder, decoder *json.Decoder) error) error {
	sock, err := net.Dial("unix", ipcFile)
	if err != nil {
		return err
	}
	defer sock.Close()

	encoder := json.NewEncoder(sock)
	decoder := json.NewDecoder(sock)

	return command(encoder, decoder)
}
