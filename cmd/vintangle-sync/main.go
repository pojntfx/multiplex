package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	v1 "github.com/pojntfx/vintangle/pkg/api/webrtc/v1"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog"
	"github.com/teivah/broadcast"
)

var (
	errMissingStreamCode = errors.New("missing stream code")
	errInvalidStreamCode = errors.New("invalid stream code")
)

func main() {
	verboseFlag := flag.Int("verbose", 5, "Verbosity level (0 is disabled, default is info, 7 is trace)")
	raddrFlag := flag.String("raddr", "wss://weron.herokuapp.com/", "Remote address")
	timeoutFlag := flag.Duration("timeout", time.Second*10, "Time to wait for connections")
	streamCodeFlag := flag.String("stream-code", "", "Stream code to join by (in format community:password:key)")
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

	if strings.TrimSpace(*streamCodeFlag) == "" {
		panic(errMissingStreamCode)
	}

	streamCodeParts := strings.Split(*streamCodeFlag, ":")
	if len(streamCodeParts) < 3 {
		panic(errInvalidStreamCode)
	}

	log.Println("Connecting to signaler with address", *raddrFlag)

	u, err := url.Parse(*raddrFlag)
	if err != nil {
		panic(err)
	}

	q := u.Query()
	q.Set("community", streamCodeParts[0])
	q.Set("password", streamCodeParts[1])
	u.RawQuery = q.Encode()

	adapter := wrtcconn.NewAdapter(
		u.String(),
		streamCodeParts[2],
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

	buffering := true
	go func() {
		b := make([]byte, 1)
		for {
			os.Stdin.Read(b)

			if string(b) == "\n" {
				buffering = !buffering

				if buffering {
					fmt.Println("Pausing")
				} else {
					fmt.Println("Unpausing")
				}

				pauses.Broadcast(buffering)
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

				if err := encoder.Encode(v1.NewPause(buffering)); err != nil {
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

						buffering = p.Pause

						if buffering {
							fmt.Println("Pausing")
						} else {
							fmt.Println("Unpausing")
						}
					case v1.TypePosition:
						var p v1.Position
						if err := mapstructure.Decode(j, &p); err != nil {
							log.Println("Could not decode position, skipping:", err)

							continue
						}

						log.Println("Position:", p.Position)
					case v1.TypeMagnet:
						var m v1.Magnet
						if err := mapstructure.Decode(j, &m); err != nil {
							log.Println("Could not decode magnet, skipping:", err)

							continue
						}

						log.Println("Got magnet link:", m)
					case v1.TypeBuffering:
						var p v1.Buffering
						if err := mapstructure.Decode(j, &p); err != nil {
							log.Println("Could not decode buffering, skipping:", err)

							continue
						}

						buffering = p.Buffering

						if buffering {
							fmt.Println("Showing buffering indicator")
						} else {
							fmt.Println("Removing buffering indicator")
						}
					}
				}
			}()
		}
	}
}
