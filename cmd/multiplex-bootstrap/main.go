// multiplex-bootstrap runs a p2panda bootstrap node for Multiplex sessions.
// Peers that set the printed public key as their bootstrap node can find each
// other across networks without mDNS visibility.
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"codeberg.org/puregotk/puregotk/examples/p2panda-gobject-go/p2panda"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
)

func hexTo32(s string) [32]byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	var out [32]byte
	copy(out[:], b)
	return out
}

func main() {
	relayFlag := flag.String("relay", "https://euc1-1.relay.n0.iroh-canary.iroh.link", "relay URL")
	networkFlag := flag.String("network", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2", "network ID hex")

	flag.Parse()

	loop := glib.NewMainLoop(glib.MainContextDefault(), false)

	relayURI, err := glib.UriParse(*relayFlag, glib.GUriFlagsNoneValue)
	if err != nil {
		panic(err)
	}

	networkID := p2panda.NewNetworkIdFromData(hexTo32(*networkFlag))
	defer networkID.Free()

	pk := p2panda.NewPrivateKey()
	defer pk.Free()

	pubKey := pk.GetPublicKey()
	defer pubKey.Free()
	pubHex := hex.EncodeToString(unsafe.Slice((*byte)(unsafe.Pointer(pubKey.GetData())), 32))

	node := p2panda.NewNode(
		pk,
		"sqlite::memory:",
		networkID,
		relayURI,
		nil,
		p2panda.MdnsDiscoveryModeActiveValue,
	)

	var onSpawn gio.AsyncReadyCallback = func(_, resultPtr, _ uintptr) {
		if _, err := node.SpawnFinish(&gio.AsyncResultBase{Ptr: resultPtr}); err != nil {
			panic(err)
		}

		slog.Info("Bootstrap node running", "pubkey", pubHex)

		fmt.Println(pubHex)
	}
	node.SpawnAsync(nil, &onSpawn, 0)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig

		loop.Quit()
	}()

	loop.Run()
}
