package components

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/anacrolix/torrent"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/internal/resources"
	"github.com/pojntfx/multiplex/internal/utils"
	mpv "github.com/pojntfx/multiplex/pkg/api/sockets/v1"
	api "github.com/pojntfx/multiplex/pkg/api/webrtc/v1"
	mpvClient "github.com/pojntfx/multiplex/pkg/client"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog/log"
	"github.com/teivah/broadcast"
	"github.com/teris-io/shortid"
)

const (
	readmePlaceholder = "No README found."

	playIcon  = "media-playback-start-symbolic"
	pauseIcon = "media-playback-pause-symbolic"

	keycodeEscape = 66
)

var (
	errKilled = errors.New("signal: killed")
)

type media struct {
	name string
	size int
}

type mediaWithPriorityAndID struct {
	media
	priority int
	id       int
}

type audioTrack struct {
	lang string
	id   int
}

func getDisplayPathWithoutRoot(p string) string {
	parts := strings.Split(p, "/") // Incoming paths are always UNIX

	if len(parts) < 2 {
		return p
	}

	return filepath.Join(parts[1:]...) // Outgoing paths are OS-specific (display only)
}

func formatDuration(duration time.Duration) string {
	hours := math.Floor(duration.Hours())
	minutes := math.Floor(duration.Minutes()) - (hours * 60)
	seconds := math.Floor(duration.Seconds()) - (minutes * 60) - (hours * 3600)

	return fmt.Sprintf("%02d:%02d:%02d", int(hours), int(minutes), int(seconds))
}

func getStreamURL(base string, magnet, path string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	streamSuffix, err := url.Parse("/stream")
	if err != nil {
		return "", err
	}

	stream := baseURL.ResolveReference(streamSuffix)

	q := stream.Query()
	q.Set("magnet", magnet)
	q.Set("path", path)
	stream.RawQuery = q.Encode()

	return stream.String(), nil
}

func OpenControlsWindow(
	ctx context.Context,
	app *adw.Application,

	torrentTitle string,
	subtitles []mediaWithPriorityAndID,
	selectedTorrentMedia,
	torrentReadme string,

	manager *client.Manager,
	apiAddr, apiUsername,
	apiPassword,

	magnetLink,
	streamURL string,

	settings *gio.Settings,
	gateway *server.Gateway,
	cancel func(),
	tmpDir string,

	ready chan struct{},
	cancelDownload func(),

	adapter *wrtcconn.Adapter,
	ids chan string,
	adapterCtx context.Context,
	cancelAdapterCtx func(),

	community,
	password,
	key string,

	bufferedMessages []interface{},
	bufferedPeer *wrtcconn.Peer,
	bufferedDecoder *json.Decoder,
) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemePreferDark)

	builder := gtk.NewBuilderFromResource(resources.GResourceControlsPath)

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	overlay := builder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)
	buttonHeaderbarTitle := builder.GetObject("button-headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarSubtitle := builder.GetObject("button-headerbar-subtitle").Cast().(*gtk.Label)
	playButton := builder.GetObject("play-button").Cast().(*gtk.Button)
	stopButton := builder.GetObject("stop-button").Cast().(*gtk.Button)
	volumeScale := builder.GetObject("volume-scale").Cast().(*gtk.Scale)
	volumeButton := builder.GetObject("volume-button").Cast().(*gtk.MenuButton)
	volumeMuteButton := builder.GetObject("audiovolume-button-mute-button").Cast().(*gtk.Button)
	subtitleButton := builder.GetObject("subtitle-button").Cast().(*gtk.Button)
	audiotracksButton := builder.GetObject("audiotracks-button").Cast().(*gtk.Button)
	fullscreenButton := builder.GetObject("fullscreen-button").Cast().(*gtk.ToggleButton)
	mediaInfoButton := builder.GetObject("media-info-button").Cast().(*gtk.Button)
	headerbarSpinner := builder.GetObject("headerbar-spinner").Cast().(*gtk.Spinner)
	menuButton := builder.GetObject("menu-button").Cast().(*gtk.MenuButton)
	elapsedTrackLabel := builder.GetObject("elapsed-track-label").Cast().(*gtk.Label)
	remainingTrackLabel := builder.GetObject("remaining-track-label").Cast().(*gtk.Label)
	seeker := builder.GetObject("seeker").Cast().(*gtk.Scale)
	watchingWithTitleLabel := builder.GetObject("watching-with-title-label").Cast().(*gtk.Label)
	streamCodeInput := builder.GetObject("stream-code-input").Cast().(*gtk.Entry)
	copyStreamCodeButton := builder.GetObject("copy-stream-code-button").Cast().(*gtk.Button)

	descriptionBuilder := gtk.NewBuilderFromResource(resources.GResourceDescriptionPath)
	descriptionWindow := descriptionBuilder.GetObject("description-window").Cast().(*adw.Window)
	descriptionText := descriptionBuilder.GetObject("description-text").Cast().(*gtk.TextView)
	descriptionHeaderbarTitle := descriptionBuilder.GetObject("headerbar-title").Cast().(*gtk.Label)
	descriptionHeaderbarSubtitle := descriptionBuilder.GetObject("headerbar-subtitle").Cast().(*gtk.Label)
	descriptionProgressBar := descriptionBuilder.GetObject("preparing-progress-bar").Cast().(*gtk.ProgressBar)

	subtitlesBuilder := gtk.NewBuilderFromResource(resources.GResourceSubtitlesPath)
	subtitlesDialog := subtitlesBuilder.GetObject("subtitles-dialog").Cast().(*gtk.Dialog)
	subtitlesCancelButton := subtitlesBuilder.GetObject("button-cancel").Cast().(*gtk.Button)
	subtitlesSpinner := subtitlesBuilder.GetObject("headerbar-spinner").Cast().(*gtk.Spinner)
	subtitlesOKButton := subtitlesBuilder.GetObject("button-ok").Cast().(*gtk.Button)
	subtitlesSelectionGroup := subtitlesBuilder.GetObject("subtitle-tracks").Cast().(*adw.PreferencesGroup)
	addSubtitlesFromFileButton := subtitlesBuilder.GetObject("add-from-file-button").Cast().(*gtk.Button)
	subtitlesOverlay := subtitlesBuilder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)

	audiotracksBuilder := gtk.NewBuilderFromResource(resources.GResourceAudiotracksPath)
	audiotracksDialog := audiotracksBuilder.GetObject("audiotracks-dialog").Cast().(*gtk.Dialog)
	audiotracksCancelButton := audiotracksBuilder.GetObject("button-cancel").Cast().(*gtk.Button)
	audiotracksOKButton := audiotracksBuilder.GetObject("button-ok").Cast().(*gtk.Button)
	audiotracksSelectionGroup := audiotracksBuilder.GetObject("audiotracks").Cast().(*adw.PreferencesGroup)

	preparingBuilder := gtk.NewBuilderFromResource(resources.GResourcePreparingPath)
	preparingWindow := preparingBuilder.GetObject("preparing-window").Cast().(*adw.Window)
	preparingProgressBar := preparingBuilder.GetObject("preparing-progress-bar").Cast().(*gtk.ProgressBar)
	preparingCancelButton := preparingBuilder.GetObject("cancel-preparing-button").Cast().(*gtk.Button)

	buttonHeaderbarTitle.SetLabel(torrentTitle)
	descriptionHeaderbarTitle.SetLabel(torrentTitle)
	buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))
	descriptionHeaderbarSubtitle.SetVisible(true)
	descriptionHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))

	descriptionProgressBar.SetVisible(true)

	if community == "" || password == "" || key == "" {
		sid, err := shortid.New(1, shortid.DefaultABC, uint64(time.Now().UnixNano()))
		if err != nil {
			return err
		}

		community, err = sid.Generate()
		if err != nil {
			return err
		}

		password, err = sid.Generate()
		if err != nil {
			return err
		}

		key, err = sid.Generate()
		if err != nil {
			return err
		}
	}

	if adapter == nil {
		adapterCtx, cancelAdapterCtx = context.WithCancel(context.Background())
	}

	u, err := url.Parse(settings.String(resources.GSchemaWeronURLKey))
	if err != nil {
		cancelAdapterCtx()

		return err
	}

	q := u.Query()
	q.Set("community", community)
	q.Set("password", password)
	u.RawQuery = q.Encode()

	pauses := broadcast.NewRelay[bool]()
	positions := broadcast.NewRelay[float64]()
	buffering := broadcast.NewRelay[bool]()

	if adapter == nil {
		adapter = wrtcconn.NewAdapter(
			u.String(),
			key,
			strings.Split(settings.String(resources.GSchemaWeronICEKey), ","),
			[]string{"multiplex/sync"},
			&wrtcconn.AdapterConfig{
				Timeout:    time.Duration(time.Second * time.Duration(settings.Int64(resources.GSchemaWeronTimeoutKey))),
				ForceRelay: settings.Boolean(resources.GSchemaWeronForceRelayKey),
				OnSignalerReconnect: func() {
					log.Info().
						Str("raddr", settings.String(resources.GSchemaWeronURLKey)).
						Msg("Reconnecting to signaler")
				},
			},
			adapterCtx,
		)

		ids, err = adapter.Open()
		if err != nil {
			cancelAdapterCtx()

			return err
		}
	}

	streamCodeInput.SetText(fmt.Sprintf("%v:%v:%v", community, password, key))

	c := int32(0)
	connectedPeers := &c
	syncWatchingWithLabel := func(connected bool) {
		if connected {
			atomic.AddInt32(connectedPeers, 1)

			toast := adw.NewToast("Someone joined the session.")

			overlay.AddToast(toast)
		} else {
			atomic.AddInt32(connectedPeers, -1)

			toast := adw.NewToast("Someone left the session.")

			overlay.AddToast(toast)
		}

		if *connectedPeers <= 0 {
			watchingWithTitleLabel.SetText("You're currently watching alone.")

			return
		}

		if *connectedPeers == 1 {
			watchingWithTitleLabel.SetText(fmt.Sprintf("You're currently watching with %v other person.", *connectedPeers))

			return
		}

		watchingWithTitleLabel.SetText(fmt.Sprintf("You're currently watching with %v other people.", *connectedPeers))
	}

	copyStreamCodeButton.ConnectClicked(func() {
		window.Clipboard().SetText(streamCodeInput.Text())
	})

	stopButton.ConnectClicked(func() {
		window.Close()

		if err := OpenAssistantWindow(ctx, app, manager, apiAddr, apiUsername, apiPassword, settings, gateway, cancel, tmpDir); err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}
	})

	mediaInfoButton.ConnectClicked(func() {
		descriptionWindow.SetVisible(true)
	})

	ctrl := gtk.NewEventControllerKey()
	descriptionWindow.AddController(ctrl)
	descriptionWindow.SetTransientFor(&window.Window)

	descriptionWindow.ConnectCloseRequest(func() (ok bool) {
		descriptionWindow.Close()
		descriptionWindow.SetVisible(false)

		return ok
	})

	ctrl.ConnectKeyReleased(func(keyval, keycode uint, state gdk.ModifierType) {
		if keycode == keycodeEscape {
			descriptionWindow.Close()
			descriptionWindow.SetVisible(false)
		}
	})

	descriptionText.SetWrapMode(gtk.WrapWord)
	if !utf8.Valid([]byte(torrentReadme)) || strings.TrimSpace(torrentReadme) == "" {
		descriptionText.Buffer().SetText(readmePlaceholder)
	} else {
		descriptionText.Buffer().SetText(torrentReadme)
	}

	preparingWindow.SetTransientFor(&window.Window)

	progressBarTicker := time.NewTicker(time.Millisecond * 500)
	go func() {
		for range progressBarTicker.C {
			metrics, err := manager.GetMetrics()
			if err != nil {
				OpenErrorDialog(ctx, window, err)

				return
			}

			length := float64(0)
			completed := float64(0)
			peers := 0

		l:
			for _, t := range metrics {
				selectedTorrent, err := torrent.TorrentSpecFromMagnetUri(magnetLink)
				if err != nil {
					OpenErrorDialog(ctx, window, err)

					return
				}

				if selectedTorrent.InfoHash.HexString() == t.InfoHash {
					peers = t.Peers

					for _, f := range t.Files {
						if f.Path == selectedTorrentMedia {
							length = float64(f.Length)
							completed = float64(f.Completed)

							break l
						}
					}
				}
			}

		n:
			for _, progressBar := range []*gtk.ProgressBar{preparingProgressBar, descriptionProgressBar} {
				if length > 0 {
					progressBar.SetFraction(completed / length)
					progressBar.SetText(fmt.Sprintf("%v MB/%v MB (%v peers)", int(completed/1000/1000), int(length/1000/1000), peers))

					continue n
				}

				progressBar.SetText("Searching for peers")
			}
		}
	}()

	preparingWindow.ConnectCloseRequest(func() (ok bool) {
		preparingWindow.Close()
		preparingWindow.SetVisible(false)

		return ok
	})

	preparingCancelButton.ConnectClicked(func() {
		adapter.Close()
		cancelAdapterCtx()

		pauses.Close()
		positions.Close()
		buffering.Close()

		progressBarTicker.Stop()

		cancelDownload()

		window.Destroy()

		preparingWindow.Close()

		if err := OpenAssistantWindow(ctx, app, manager, apiAddr, apiUsername, apiPassword, settings, gateway, cancel, tmpDir); err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}
	})

	usernameAndPassword := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v:%v", apiUsername, apiPassword)))

	ipcDir, err := os.MkdirTemp(os.TempDir(), "mpv-ipc")
	if err != nil {
		return err
	}

	ipcFile := filepath.Join(ipcDir, "mpv.sock")

	shell := []string{"sh", "-c"}
	if runtime.GOOS == "windows" {
		shell = []string{"cmd.exe", "/c", "start"}
	}
	commandLine := append(shell, fmt.Sprintf("%v '--no-sub-visibility' '--keep-open=always' '--no-osc' '--no-input-default-bindings' '--pause' '--input-ipc-server=%v' '--http-header-fields=Authorization: Basic %v' '%v'", settings.String(resources.GSchemaMPVKey), ipcFile, usernameAndPassword, streamURL))

	command := exec.Command(
		commandLine[0],
		commandLine[1:]...,
	)
	utils.AddSysProcAttr(command)

	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	AddMainMenu(
		ctx,
		app,
		window,
		settings,
		menuButton,
		overlay,
		gateway,
		func() string {
			return magnetLink
		},
		func() {
			cancel()

			if command.Process != nil {
				if err := command.Process.Kill(); err != nil {
					OpenErrorDialog(ctx, window, err)

					return
				}
			}
		},
	)

	app.AddWindow(&window.Window)

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		log.Debug().Msg("Gracefully shutting down")

		window.Close()
	}()

	window.ConnectShow(func() {
		preparingWindow.SetVisible(true)

		go func() {
			<-ready

			if err := command.Start(); err != nil {
				OpenErrorDialog(ctx, window, err)

				return
			}
		}()

		window.ConnectCloseRequest(func() (ok bool) {
			adapter.Close()
			cancelAdapterCtx()

			pauses.Close()
			positions.Close()
			buffering.Close()

			progressBarTicker.Stop()

			if command.Process != nil {
				if err := utils.Kill(command.Process); err != nil {
					OpenErrorDialog(ctx, window, err)

					return false
				}
			}

			if err := os.RemoveAll(ipcDir); err != nil {
				OpenErrorDialog(ctx, window, err)

				return false
			}

			return true
		})

		go func() {
			<-ready

			for {
				sock, err := net.Dial("unix", ipcFile)
				if err == nil {
					_ = sock.Close()

					break
				}

				time.Sleep(time.Millisecond * 100)

				log.Error().
					Str("path", ipcFile).
					Err(err).
					Msg("Could not dial IPC socket, retrying in 100ms")
			}

			startPlayback := func() {
				playButton.SetIconName(pauseIcon)

				if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Info().Msg("Starting playback")

					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "pause", false}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(ctx, window, err)

					return
				}
			}

			pausePlayback := func() {
				playButton.SetIconName(playIcon)

				if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Info().Msg("Pausing playback")

					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "pause", true}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(ctx, window, err)

					return
				}
			}

			var trackListResponse mpv.ResponseTrackList
			if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
				log.Debug().Msg("Getting tracklist")

				if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "track-list"}}); err != nil {
					return err
				}

				return decoder.Decode(&trackListResponse)
			}); err != nil {
				OpenErrorDialog(ctx, window, err)

				return
			}

			audiotracks := []audioTrack{}
			for _, track := range trackListResponse.Data {
				if track.Type == mpv.TypeAudio {
					audiotracks = append(audiotracks, audioTrack{
						lang: track.Lang,
						id:   track.ID,
					})
				}
			}

			subtracks := []mediaWithPriorityAndID{}
			for _, track := range trackListResponse.Data {
				if track.Type == mpv.TypeSub {
					subtracks = append(subtracks, mediaWithPriorityAndID{
						media: media{
							name: track.Title,
							size: 0,
						},
						id:       track.ID,
						priority: 0,
					})
				}
			}

			seekerIsSeeking := false
			seekerIsUnderPointer := false
			total := time.Duration(0)
			seekToPosition := func(position float64) {
				seekerIsSeeking = true

				seeker.SetValue(position)

				elapsed := time.Duration(int64(position))

				if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpv.Request{[]interface{}{"seek", int64(elapsed.Seconds()), "absolute"}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(ctx, window, err)

					return
				}

				log.Info().
					Dur("duration", elapsed).
					Msg("Seeking")

				remaining := total - elapsed

				elapsedTrackLabel.SetLabel(formatDuration(elapsed))
				remainingTrackLabel.SetLabel("-" + formatDuration(remaining))

				var updateScalePosition func(done bool)
				updateScalePosition = func(done bool) {
					if seekerIsUnderPointer {
						if done {
							seekerIsSeeking = false

							return
						}

						updateScalePosition(true)
					} else {
						seekerIsSeeking = false
					}
				}

				time.AfterFunc(
					time.Millisecond*200,
					func() {
						updateScalePosition(false)
					},
				)
			}

			handlePeer := func(peer *wrtcconn.Peer, decoder *json.Decoder) {
				defer func() {
					log.Info().
						Str("peerID", peer.PeerID).
						Str("channel", peer.ChannelID).
						Msg("Disconnected from peer")

					headerbarSpinner.SetSpinning(false)

					syncWatchingWithLabel(false)
				}()

				log.Info().
					Str("peerID", peer.PeerID).
					Str("channel", peer.ChannelID).
					Msg("Connected to peer")

				syncWatchingWithLabel(true)

				encoder := json.NewEncoder(peer.Conn)
				if decoder == nil {
					decoder = json.NewDecoder(peer.Conn)
				}

				go func() {
					pl := pauses.Listener(0)
					defer pl.Close()

					ol := positions.Listener(0)
					defer ol.Close()

					bl := buffering.Listener(0)
					defer bl.Close()

					for {
						select {
						case <-ctx.Done():
							return
						case pause, ok := <-pl.Ch():
							if !ok {
								continue
							}

							if err := encoder.Encode(api.NewPause(pause)); err != nil {
								log.Debug().
									Err(err).
									Msg("Could not encode pause, stopping")

								return
							}
						case position, ok := <-ol.Ch():
							if !ok {
								continue
							}

							if err := encoder.Encode(api.NewPosition(position)); err != nil {
								log.Debug().
									Err(err).
									Msg("Could not encode pause, stopping")

								return
							}
						case buffering, ok := <-bl.Ch():
							if !ok {
								continue
							}

							if err := encoder.Encode(api.NewBuffering(buffering)); err != nil {
								log.Debug().
									Err(err).
									Msg("Could not encode buffering, stopping")

								return
							}
						}
					}
				}()

				if err := encoder.Encode(api.NewPause(true)); err != nil {
					log.Debug().
						Err(err).
						Msg("Could not encode pause, stopping")

					return
				}

				s := []api.Subtitle{}
				for _, subtitle := range subtitles {
					s = append(s, api.Subtitle{
						Name: subtitle.name,
						Size: subtitle.size,
					})
				}

				if err := encoder.Encode(api.NewMagnetLink(magnetLink, selectedTorrentMedia, torrentTitle, torrentReadme, s)); err != nil {
					log.Debug().
						Err(err).
						Msg("Could not encode magnet link, stopping")

					return
				}

				var elapsedResponse mpv.ResponseFloat64
				if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "time-pos"}}); err != nil {
						return err
					}

					return decoder.Decode(&elapsedResponse)
				}); err != nil {
					log.Error().
						Err(err).
						Msg("Could not parse JSON from socket")

					return
				}

				if elapsedResponse.Data != 0 {
					elapsed, err := time.ParseDuration(fmt.Sprintf("%vs", int64(elapsedResponse.Data)))
					if err != nil {
						OpenErrorDialog(ctx, window, err)

						return
					}

					positions.Broadcast(float64(elapsed.Nanoseconds()))
				}

				for {
					var j interface{}
					if len(bufferedMessages) > 0 {
						j = bufferedMessages[len(bufferedMessages)-1]
						bufferedMessages = bufferedMessages[:len(bufferedMessages)-1]
					} else {
						if err := decoder.Decode(&j); err != nil {
							log.Debug().
								Err(err).
								Msg("Could not decode structure, skipping")

							return
						}
					}

					var message api.Message
					if err := mapstructure.Decode(j, &message); err != nil {
						log.Debug().
							Err(err).
							Msg("Could not decode message, skipping")

						continue
					}

					log.Info().Interface("message", message).Msg("Decoded message")

					switch message.Type {
					case api.TypePause:
						var p api.Pause
						if err := mapstructure.Decode(j, &p); err != nil {
							log.Debug().
								Err(err).
								Msg("Could not decode pause, skipping")

							continue
						}

						if p.Pause {
							if pausePlayback != nil {
								pausePlayback()
							}
						} else {
							if startPlayback != nil {
								startPlayback()
							}
						}
					case api.TypePosition:
						var p api.Position
						if err := mapstructure.Decode(j, &p); err != nil {
							log.Debug().
								Err(err).
								Msg("Could not decode position, skipping")

							continue
						}

						if seekToPosition != nil {
							seekToPosition(p.Position)
						}
					case api.TypeMagnet:
						var m api.Magnet
						if err := mapstructure.Decode(j, &m); err != nil {
							log.Debug().
								Err(err).
								Msg("Could not decode magnet, skipping")

							continue
						}

						log.Info().
							Str("magnet", m.Magnet).
							Str("path", m.Path).
							Msg("Got magnet link")
					case api.TypeBuffering:
						var b api.Buffering
						if err := mapstructure.Decode(j, &b); err != nil {
							log.Debug().
								Err(err).
								Msg("Could not decode buffering, skipping")

							continue
						}

						if b.Buffering {
							headerbarSpinner.SetSpinning(true)

							if pausePlayback != nil {
								pausePlayback()
							}

							playButton.SetIconName(pauseIcon)
						} else {
							headerbarSpinner.SetSpinning(false)

							if startPlayback != nil {
								startPlayback()
							}
						}
					}
				}
			}

			if bufferedPeer != nil {
				go handlePeer(bufferedPeer, bufferedDecoder)
			}

			go func() {
				for {
					select {
					case <-ctx.Done():
						if err := ctx.Err(); err != context.Canceled {
							OpenErrorDialog(ctx, window, err)

							return
						}

						return
					case rid := <-ids:
						log.Info().
							Str("raddr", settings.String(resources.GSchemaWeronURLKey)).
							Str("id", rid).
							Msg("Reconnecting to signaler")
					case peer := <-adapter.Accept():
						go handlePeer(peer, nil)
					}
				}
			}()

			if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
				if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "volume", 100}}); err != nil {
					return err
				}

				var successResponse mpv.ResponseSuccess
				return decoder.Decode(&successResponse)
			}); err != nil {
				OpenErrorDialog(ctx, window, err)

				return
			}

			subtitleActivators := []*gtk.CheckButton{}

			for i, file := range append(
				append([]mediaWithPriorityAndID{
					{media: media{
						name: "None",
						size: 0,
					},
						priority: -1,
					},
				},
					subtracks...,
				), subtitles...) {
				row := adw.NewActionRow()

				activator := gtk.NewCheckButton()

				if len(subtitleActivators) > 0 {
					activator.SetGroup(subtitleActivators[i-1])
					activator.SetActive(false)
				} else {
					activator.SetActive(true)
				}
				subtitleActivators = append(subtitleActivators, activator)

				m := file.name
				p := file.priority
				sid := file.id
				j := i
				activator.ConnectActivate(func() {
					defer func() {
						if len(subtitleActivators) <= 1 {
							activator.SetActive(true)
						}
					}()

					if j == 0 {
						log.Info().
							Msg("Disabling subtitles")

						if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
							if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", "no"}}); err != nil {
								return err
							}

							var successResponse mpv.ResponseSuccess
							return decoder.Decode(&successResponse)
						}); err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}

						if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
							if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sub-visibility", "no"}}); err != nil {
								return err
							}

							var successResponse mpv.ResponseSuccess
							return decoder.Decode(&successResponse)
						}); err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}

						return
					}

					if p == 0 {
						if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
							log.Debug().
								Msg("Setting subtitle ID")

							if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", sid}}); err != nil {
								return err
							}

							var successResponse mpv.ResponseSuccess
							return decoder.Decode(&successResponse)
						}); err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}

						if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
							if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sub-visibility", "yes"}}); err != nil {
								return err
							}

							var successResponse mpv.ResponseSuccess
							return decoder.Decode(&successResponse)
						}); err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}

						return
					}

					go func() {
						defer func() {
							subtitlesSpinner.SetSpinning(false)
							subtitlesSpinner.SetVisible(false)
							subtitlesOKButton.SetSensitive(true)
						}()

						subtitlesOKButton.SetSensitive(false)
						subtitlesSpinner.SetVisible(true)
						subtitlesSpinner.SetSpinning(true)

						streamURL, err := getStreamURL(apiAddr, magnetLink, m)
						if err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}

						log.Info().
							Str("streamURL", streamURL).
							Msg("Downloading subtitles")

						hc := &http.Client{}

						req, err := http.NewRequest(http.MethodGet, streamURL, http.NoBody)
						if err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}
						req.SetBasicAuth(apiUsername, apiPassword)

						res, err := hc.Do(req)
						if err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}
						if res.Body != nil {
							defer res.Body.Close()
						}
						if res.StatusCode != http.StatusOK {
							OpenErrorDialog(ctx, window, errors.New(res.Status))

							return
						}

						log.Info().
							Str("streamURL", streamURL).
							Msg("Finished downloading subtitles")

						SetSubtitles(ctx, window, m, res.Body, tmpDir, ipcFile, subtitleActivators[0], subtitlesOverlay)
					}()
				})

				if i == 0 {
					row.SetTitle(file.name)
					row.SetSubtitle("Disable subtitles")

					activator.SetActive(true)
				} else if file.priority == 0 {
					row.SetTitle(getDisplayPathWithoutRoot(file.name))
					row.SetSubtitle("Integrated subtitle")
				} else if file.priority == 1 {
					row.SetTitle(getDisplayPathWithoutRoot(file.name))
					row.SetSubtitle("Subtitle from torrent")
				} else {
					row.SetTitle(getDisplayPathWithoutRoot(file.name))
					row.SetSubtitle("Extra file from torrent")
				}

				row.SetActivatable(true)

				row.AddPrefix(activator)
				row.SetActivatableWidget(activator)

				subtitlesSelectionGroup.Add(row)
			}

			audiotrackActivators := []*gtk.CheckButton{}

			for i, audiotrack := range append(
				[]audioTrack{
					{
						lang: "None",
						id:   -1,
					},
				},
				audiotracks...,
			) {
				row := adw.NewActionRow()

				activator := gtk.NewCheckButton()

				if len(audiotrackActivators) > 0 {
					activator.SetGroup(audiotrackActivators[i-1])
					activator.SetActive(false)
				} else {
					activator.SetActive(true)
				}
				audiotrackActivators = append(audiotrackActivators, activator)

				a := audiotrack
				j := i
				activator.ConnectActivate(func() {
					defer func() {
						if len(audiotrackActivators) <= 1 {
							activator.SetActive(true)
						}
					}()

					if len(audiotrackActivators) <= 1 {
						// Don't disable audio if the "None" track is the only one

						return
					}

					if j == 0 {
						log.Info().
							Msg("Disabling audio track")

						if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
							if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "aid", "no"}}); err != nil {
								return err
							}

							var successResponse mpv.ResponseSuccess
							return decoder.Decode(&successResponse)
						}); err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}

						return
					}

					if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						log.Debug().
							Int("aid", a.id).
							Msg("Setting audio ID")

						if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "aid", a.id}}); err != nil {
							return err
						}

						var successResponse mpv.ResponseSuccess
						return decoder.Decode(&successResponse)
					}); err != nil {
						OpenErrorDialog(ctx, window, err)

						return
					}
				})

				if j == 0 {
					row.SetSubtitle("Disable audio")
				} else {
					row.SetSubtitle(fmt.Sprintf("Track %v", a.id))
				}

				if i == 1 {
					activator.SetActive(true)
				}

				if strings.TrimSpace(a.lang) == "" {
					row.SetTitle("Untitled Track")
				} else {
					row.SetTitle(a.lang)
				}

				row.SetActivatable(true)

				row.AddPrefix(activator)
				row.SetActivatableWidget(activator)

				audiotracksSelectionGroup.Add(row)
			}

			ctrl := gtk.NewEventControllerMotion()
			ctrl.ConnectEnter(func(x, y float64) {
				seekerIsUnderPointer = true
			})
			ctrl.ConnectLeave(func() {
				seekerIsUnderPointer = false
			})
			seeker.AddController(ctrl)

			seeker.ConnectChangeValue(func(scroll gtk.ScrollType, value float64) (ok bool) {
				seekToPosition(value)

				positions.Broadcast(value)

				return true
			})

			preparingClosed := false
			done := make(chan struct{})
			previouslyBuffered := false
			go func() {
				t := time.NewTicker(time.Millisecond * 200)

				updateSeeker := func() {
					var durationResponse mpv.ResponseFloat64
					if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "duration"}}); err != nil {
							return err
						}

						return decoder.Decode(&durationResponse)
					}); err != nil {
						log.Error().
							Err(err).
							Msg("Could not parse JSON from socket")

						return
					}

					total, err = time.ParseDuration(fmt.Sprintf("%vs", int64(durationResponse.Data)))
					if err != nil {
						OpenErrorDialog(ctx, window, err)

						return
					}

					if total != 0 && !preparingClosed {
						preparingWindow.SetVisible(false)

						preparingClosed = true
					}

					var elapsedResponse mpv.ResponseFloat64
					if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "time-pos"}}); err != nil {
							return err
						}

						return decoder.Decode(&elapsedResponse)
					}); err != nil {
						log.Error().
							Err(err).
							Msg("Could not parse JSON from socket")

						return
					}

					elapsed, err := time.ParseDuration(fmt.Sprintf("%vs", int64(elapsedResponse.Data)))
					if err != nil {
						OpenErrorDialog(ctx, window, err)

						return
					}

					var pausedResponse mpv.ResponseBool
					if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "core-idle"}}); err != nil {
							return err
						}

						return decoder.Decode(&pausedResponse)
					}); err != nil {
						log.Error().
							Err(err).
							Msg("Could not parse JSON from socket")

						return
					}

					// If MPV is paused, but the GUI is showing the playing state, assume we're buffering
					if pausedResponse.Data == (playButton.IconName() == pauseIcon) {
						if !previouslyBuffered {
							previouslyBuffered = true

							headerbarSpinner.SetSpinning(true)
							buffering.Broadcast(true)
							pauses.Broadcast(true)
							positions.Broadcast(float64(elapsed.Nanoseconds()))
						}
					} else {
						if previouslyBuffered {
							previouslyBuffered = false

							headerbarSpinner.SetSpinning(false)
							buffering.Broadcast(false)
							pauses.Broadcast(false)
							positions.Broadcast(float64(elapsed.Nanoseconds()))
						}
					}

					if !seekerIsSeeking {
						seeker.
							SetRange(0, float64(total.Nanoseconds()))
						seeker.
							SetValue(float64(elapsed.Nanoseconds()))

						remaining := total - elapsed

						log.Trace().
							Float64("total", total.Seconds()).
							Float64("elapsed", elapsed.Seconds()).
							Float64("remaining", remaining.Seconds()).
							Msg("Updating scale")

						elapsedTrackLabel.SetLabel(formatDuration(elapsed))
						remainingTrackLabel.SetLabel("-" + formatDuration(remaining))
					}
				}

				for {
					select {
					case <-t.C:
						updateSeeker()
					case <-done:
						return
					}
				}
			}()

			volumeMuteButton.ConnectClicked(func() {
				if volumeScale.Value() <= 0 {
					volumeScale.SetValue(1)
				} else {
					volumeScale.SetValue(0)
				}
			})

			volumeScale.ConnectValueChanged(func() {
				value := volumeScale.Value()

				if value <= 0 {
					volumeButton.SetIconName("audio-volume-muted-symbolic")
					volumeMuteButton.SetIconName("audio-volume-muted-symbolic")
				} else if value <= 0.3 {
					volumeButton.SetIconName("audio-volume-low-symbolic")
					volumeMuteButton.SetIconName("audio-volume-high-symbolic")
				} else if value <= 0.6 {
					volumeButton.SetIconName("audio-volume-medium-symbolic")
					volumeMuteButton.SetIconName("audio-volume-high-symbolic")
				} else {
					volumeButton.SetIconName("audio-volume-high-symbolic")
					volumeMuteButton.SetIconName("audio-volume-high-symbolic")
				}

				if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {

					log.Info().
						Float64("value", value).
						Msg("Setting volume")

					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "volume", value * 100}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(ctx, window, err)
				}
			})

			subtitleButton.ConnectClicked(func() {
				subtitlesDialog.SetVisible(true)
			})

			audiotracksButton.ConnectClicked(func() {
				audiotracksDialog.SetVisible(true)
			})

			for _, d := range []*gtk.Dialog{subtitlesDialog, audiotracksDialog} {
				dialog := d

				escCtrl := gtk.NewEventControllerKey()
				dialog.AddController(escCtrl)
				dialog.SetTransientFor(&window.Window)

				dialog.ConnectCloseRequest(func() (ok bool) {
					dialog.Close()
					dialog.SetVisible(false)

					return ok
				})

				escCtrl.ConnectKeyReleased(func(keyval, keycode uint, state gdk.ModifierType) {
					if keycode == keycodeEscape {
						dialog.Close()
						dialog.SetVisible(false)
					}
				})
			}

			subtitlesCancelButton.ConnectClicked(func() {
				log.Info().
					Msg("Disabling subtitles")

				if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", "no"}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(ctx, window, err)

					return
				}

				subtitlesDialog.Close()
			})

			subtitlesOKButton.ConnectClicked(func() {
				subtitlesDialog.Close()
			})

			audiotracksCancelButton.ConnectClicked(func() {
				audiotracksDialog.Close()
			})

			audiotracksOKButton.ConnectClicked(func() {
				audiotracksDialog.Close()
			})

			addSubtitlesFromFileButton.ConnectClicked(func() {
				filePicker := gtk.NewFileChooserNative(
					"Select storage location",
					&window.Window,
					gtk.FileChooserActionOpen,
					"",
					"")
				filePicker.SetModal(true)
				filePicker.ConnectResponse(func(responseId int) {
					if responseId == int(gtk.ResponseAccept) {
						log.Info().
							Str("path", filePicker.File().Path()).
							Msg("Setting subtitles")

						m := filePicker.File().Path()
						subtitlesFile, err := os.Open(m)
						if err != nil {
							OpenErrorDialog(ctx, window, err)

							return
						}
						defer subtitlesFile.Close()

						SetSubtitles(ctx, window, m, subtitlesFile, tmpDir, ipcFile, subtitleActivators[0], subtitlesOverlay)

						row := adw.NewActionRow()

						activator := gtk.NewCheckButton()

						activator.SetGroup(subtitleActivators[len(subtitleActivators)-1])
						subtitleActivators = append(subtitleActivators, activator)

						activator.SetActive(true)
						activator.ConnectActivate(func() {
							m := filePicker.File().Path()
							subtitlesFile, err := os.Open(m)
							if err != nil {
								OpenErrorDialog(ctx, window, err)

								return
							}
							defer subtitlesFile.Close()

							SetSubtitles(ctx, window, m, subtitlesFile, tmpDir, ipcFile, subtitleActivators[0], subtitlesOverlay)
						})

						row.SetTitle(filePicker.File().Basename())
						row.SetSubtitle("Manually added")

						row.SetActivatable(true)

						row.AddPrefix(activator)
						row.SetActivatableWidget(activator)

						subtitlesSelectionGroup.Add(row)
					}

					filePicker.Destroy()
				})

				filePicker.Show()
			})

			fullscreenButton.ConnectClicked(func() {
				if fullscreenButton.Active() {
					if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						log.Info().Msg("Enabling fullscreen")

						if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "fullscreen", true}}); err != nil {
							return err
						}

						var successResponse mpv.ResponseSuccess
						return decoder.Decode(&successResponse)
					}); err != nil {
						OpenErrorDialog(ctx, window, err)

						return
					}

					return
				}

				if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Info().Msg("Disabling fullscreen")

					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "fullscreen", false}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(ctx, window, err)

					return
				}
			})

			playButton.ConnectClicked(func() {
				if !headerbarSpinner.Spinning() {
					if playButton.IconName() == playIcon {
						pauses.Broadcast(false)

						startPlayback()

						return
					}

					pauses.Broadcast(true)

					pausePlayback()
				}
			})

			go func() {
				if err := command.Wait(); err != nil && err.Error() != errKilled.Error() {
					OpenErrorDialog(ctx, window, err)

					return
				}

				done <- struct{}{}

				window.Destroy()
			}()

			playButton.GrabFocus()
		}()
	})

	window.SetVisible(true)

	return nil
}
