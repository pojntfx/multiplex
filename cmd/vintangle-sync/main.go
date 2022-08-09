package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/mitchellh/mapstructure"
	v1 "github.com/pojntfx/vintangle/pkg/api/webrtc/v1"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog"
	"github.com/teivah/broadcast"
)

var (
	errMissingCommunity = errors.New("missing community")
	errMissingPassword  = errors.New("missing password")
	errMissingKey       = errors.New("missing key")

	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

func main() {
	verboseFlag := flag.Int("verbose", 5, "Verbosity level (0 is disabled, default is info, 7 is trace)")
	raddrFlag := flag.String("raddr", "wss://weron.herokuapp.com/", "Remote address")
	timeoutFlag := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	communityFlag := flag.String("community", "", "ID of community to join")
	passwordFlag := flag.String("password", "", "Password for community")
	keyFlag := flag.String("key", "", "Encryption key for community")
	iceFlag := flag.String("ice", "stun:stun.l.google.com:19302", "Comma-separated list of STUN servers (in format stun:host:port) and TURN servers to use (in format username:credential@turn:host:port) (i.e. username:credential@turn:global.turn.twilio.com:3478?transport=tcp)")
	forceRelayFlag := flag.Bool("force-relay", false, "Force usage of TURN servers")

	flag.Parse()

	switch *verboseFlag {
	case 0:
		zerolog.SetGlobalLevel(zerolog.Disabled)
	case 1:
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	case 2:
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case 3:
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case 4:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case 5:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case 6:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if strings.TrimSpace(*communityFlag) == "" {
		panic(errMissingCommunity)
	}

	if strings.TrimSpace(*passwordFlag) == "" {
		panic(errMissingPassword)
	}

	if strings.TrimSpace(*keyFlag) == "" {
		panic(errMissingKey)
	}

	log.Println("Connecting to signaler with address", *raddrFlag)

	u, err := url.Parse(*raddrFlag)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", *communityFlag)
	q.Set("password", *passwordFlag)
	u.RawQuery = q.Encode()

	adapter := wrtcconn.NewAdapter(
		u.String(),
		*keyFlag,
		strings.Split(*iceFlag, ","),
		[]string{"vintangle/sync"},
		&wrtcconn.AdapterConfig{
			Timeout:    *timeoutFlag,
			ForceRelay: *forceRelayFlag,
			OnSignalerReconnect: func() {
				log.Println("Reconnecting to signaler with address", *raddrFlag)
			},
		},
		ctx,
	)

	ids, err := adapter.Open()
	if err != nil {
		panic(err)
	}
	defer adapter.Close()

	pauses := broadcast.NewRelay[bool]()
	defer pauses.Close()

	pause := true
	go func() {
		b := make([]byte, 1)
		for {
			os.Stdin.Read(b)

			if string(b) == "\n" {
				pause = !pause

				if pause {
					fmt.Println("Pausing")
				} else {
					fmt.Println("Unpausing")
				}

				pauses.Broadcast(pause)
			}
		}
	}()

	errs := make(chan error)
	for {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != context.Canceled {
				panic(err)
			}

			return
		case err := <-errs:
			panic(err)
		case rid := <-ids:
			log.Println("Connected to signaler with address", *raddrFlag, "and ID", rid)
		case peer := <-adapter.Accept():
			go func() {
				defer func() {
					log.Println("Disconnected from peer with ID", peer.PeerID, "and channel", peer.ChannelID)
				}()

				log.Println("Connected to peer with ID", peer.PeerID, "and channel", peer.ChannelID)

				encoder := json.NewEncoder(peer.Conn)
				decoder := json.NewDecoder(peer.Conn)

				if err := encoder.Encode(v1.NewPause(pause)); err != nil {
					log.Println("Could not encode pause, stopping:", err)

					return
				}

				go func() {
					l := pauses.Listener(0)
					defer l.Close()

					for {
						select {
						case <-ctx.Done():
							return
						case pause := <-l.Ch():
							if err := encoder.Encode(v1.NewPause(pause)); err != nil {
								log.Println("Could not encode pause, stopping:", err)

								return
							}
						}
					}
				}()

				for {
					var j interface{}
					if err := decoder.Decode(&j); err != nil {
						log.Println("Could not decode structure, stopping:", err)

						return
					}

					var message v1.Message
					if err := mapstructure.Decode(j, &message); err != nil {
						log.Println("Could not decode message, skipping:", err)

						continue
					}

					switch message.Type {
					case v1.TypePause:
						var p v1.Pause
						if err := mapstructure.Decode(j, &p); err != nil {
							log.Println("Could not decode pause, skipping:", err)

							continue
						}

						pause = p.Pause

						if pause {
							fmt.Println("Pausing")
						} else {
							fmt.Println("Unpausing")
						}
					}
				}
			}()
		}
	}
}
