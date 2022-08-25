package main

//go:generate glib-compile-schemas .

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/anacrolix/torrent"
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/mitchellh/mapstructure"
	"github.com/phayes/freeport"
	v1 "github.com/pojntfx/htorrent/pkg/api/http/v1"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	api "github.com/pojntfx/vintangle/pkg/api/webrtc/v1"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/teivah/broadcast"
	"github.com/teris-io/shortid"

	_ "embed"
)

type media struct {
	name string
	size int
}

type mediaWithPriority struct {
	media
	priority int
}

type audioTrack struct {
	lang string
	id   int
}

type mpvCommand struct {
	Command []interface{} `json:"command"`
}

type mpvFloat64Response struct {
	Data float64 `json:"data"`
}

type mpvTrackListResponse struct {
	Data []mpvTrackDescription `json:"data"`
}

type mpvTrackDescription struct {
	ID               int    `json:"id"`
	Type             string `json:"type"`
	ExternalFilename string `json:"external-filename"`
	Lang             string `json:"lang"`
}

type mpvSuccessResponse struct {
	Data []any `json:"data"`
}

type mpvBoolResponse struct {
	Data bool `json:"data"`
}

var (
	//go:embed assistant.ui
	assistantUI string

	//go:embed controls.ui
	controlsUI string

	//go:embed description.ui
	descriptionUI string

	//go:embed warning.ui
	warningUI string

	//go:embed error.ui
	errorUI string

	//go:embed menu.ui
	menuUI string

	//go:embed about.ui
	aboutUI string

	//go:embed preferences.ui
	preferencesUI string

	//go:embed subtitles.ui
	subtitlesUI string

	//go:embed audiotracks.ui
	audiotracksUI string

	//go:embed preparing.ui
	preparingUI string

	//go:embed style.css
	styleCSS string

	//go:embed gschemas.compiled
	geschemas []byte

	letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	errKilled            = errors.New("signal: killed")
	errNoWorkingMPVFound = errors.New("could not find working a working mpv")
)

const (
	appID   = "com.pojtinger.felicitas.vintangle"
	stateID = appID + ".state"

	welcomePageName = "welcome-page"
	mediaPageName   = "media-page"
	readyPageName   = "ready-page"

	playIcon  = "media-playback-start-symbolic"
	pauseIcon = "media-playback-pause-symbolic"

	readmePlaceholder = "No README found."

	verboseFlag = "verbose"
	storageFlag = "storage"
	mpvFlag     = "mpv"

	gatewayRemoteFlag   = "gatewayremote"
	gatewayURLFlag      = "gatewayurl"
	gatewayUsernameFlag = "gatewayusername"
	gatewayPasswordFlag = "gatewaypassword"

	weronURLFlag        = "weronurl"
	weronTimeoutFlag    = "werontimeout"
	weronICEFlag        = "weronice"
	weronForceRelayFlag = "weronforcerelay"

	keycodeEscape = 66

	schemaDirEnvVar = "GSETTINGS_SCHEMA_DIR"

	preferencesActionName      = "preferences"
	applyPreferencesActionName = "applypreferences"
	openDownloadsActionName    = "opendownloads"
	copyMagnetLinkActionName   = "copymagnetlink"

	mpvFlathubURL = "https://flathub.org/apps/details/io.mpv.Mpv"
	mpvWebsiteURL = "https://mpv.io/installation/"

	issuesURL = "https://github.com/pojntfx/vintangle/issues"

	mpvTypeSub   = "sub"
	mpvTypeAudio = "audio"
)

// See https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go/22892986#22892986
func randSeq(n int) string {
	b := make([]rune, n)

	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
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

func formatDuration(duration time.Duration) string {
	hours := math.Floor(duration.Hours())
	minutes := math.Floor(duration.Minutes()) - (hours * 60)
	seconds := math.Floor(duration.Seconds()) - (minutes * 60) - (hours * 3600)

	return fmt.Sprintf("%02d:%02d:%02d", int(hours), int(minutes), int(seconds))
}

func getDisplayPathWithoutRoot(p string) string {
	parts := strings.Split(p, "/") // Incoming paths are always UNIX

	if len(parts) < 2 {
		return p
	}

	return filepath.Join(parts[1:]...) // Outgoing paths are OS-specific (display only)
}

func findWorkingMPV() (string, error) {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		if err := exec.Command("flatpak-spawn", "--host", "mpv", "--version").Run(); err == nil {
			return "flatpak-spawn --host mpv", nil
		}

		if err := exec.Command("flatpak-spawn", "--host", "flatpak", "run", "io.mpv.Mpv", "--version").Run(); err == nil {
			return "flatpak-spawn --host flatpak run io.mpv.Mpv", nil
		}

		return "", errNoWorkingMPVFound
	}

	if err := exec.Command("mpv", "--version").Run(); err == nil {
		return "mpv", nil
	}

	if err := exec.Command("flatpak", "run", "io.mpv.Mpv", "--version").Run(); err == nil {
		return "flatpak run io.mpv.Mpv", nil
	}

	return "", errNoWorkingMPVFound
}

func runMPVCommand(ipcFile string, command func(encoder *json.Encoder, decoder *json.Decoder) error) error {
	sock, err := net.Dial("unix", ipcFile)
	if err != nil {
		return err
	}
	defer sock.Close()

	encoder := json.NewEncoder(sock)
	decoder := json.NewDecoder(sock)

	return command(encoder, decoder)
}

func setSubtitles(
	ctx context.Context,
	window *adw.ApplicationWindow,

	filePath string,
	file io.Reader,
	tmpDir string,
	ipcFile string,

	noneActivator *gtk.CheckButton,
	subtitlesOverlay *adw.ToastOverlay,
) {
	subtitlesDir, err := os.MkdirTemp(tmpDir, "subtitles")
	if err != nil {
		openErrorDialog(ctx, window, err)

		return
	}

	subtitlesFile := filepath.Join(subtitlesDir, path.Base(filePath))
	f, err := os.Create(subtitlesFile)
	if err != nil {
		openErrorDialog(ctx, window, err)

		return
	}

	if _, err := io.Copy(f, file); err != nil {
		openErrorDialog(ctx, window, err)

		return
	}

	if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().
			Str("path", subtitlesFile).
			Msg("Adding subtitles path")

		if err := encoder.Encode(mpvCommand{[]interface{}{"change-list", "sub-file-paths", "set", subtitlesDir}}); err != nil {
			return err
		}

		var successResponse mpvSuccessResponse
		return decoder.Decode(&successResponse)
	}); err != nil {
		openErrorDialog(ctx, window, err)

		return
	}

	if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().Msg("Reloading subtitles")

		if err := encoder.Encode(mpvCommand{[]interface{}{"rescan-external-files"}}); err != nil {
			return err
		}

		var successResponse mpvSuccessResponse
		return decoder.Decode(&successResponse)
	}); err != nil {
		openErrorDialog(ctx, window, err)

		return
	}

	var trackListResponse mpvTrackListResponse
	if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().Msg("Getting tracklist")

		if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "track-list"}}); err != nil {
			return err
		}

		return decoder.Decode(&trackListResponse)
	}); err != nil {
		openErrorDialog(ctx, window, err)

		return
	}

	sid := -1
	for _, track := range trackListResponse.Data {
		if track.Type == mpvTypeSub && track.ExternalFilename == subtitlesFile {
			sid = track.ID

			break
		}
	}

	if sid == -1 {
		log.Info().
			Msg("Disabling subtitles")

		time.AfterFunc(time.Millisecond*100, func() {
			noneActivator.SetActive(true)
		})

		if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
			if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "sid", "no"}}); err != nil {
				return err
			}

			var successResponse mpvSuccessResponse
			return decoder.Decode(&successResponse)
		}); err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		toast := adw.NewToast("This file does not contain subtitles.")

		subtitlesOverlay.AddToast(toast)

		return
	}

	if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().
			Str("path", subtitlesFile).
			Int("sid", sid).
			Msg("Setting subtitle ID")

		if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "sid", sid}}); err != nil {
			return err
		}

		var successResponse mpvSuccessResponse
		return decoder.Decode(&successResponse)
	}); err != nil {
		openErrorDialog(ctx, window, err)

		return
	}

	if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "sub-visibility", "yes"}}); err != nil {
			return err
		}

		var successResponse mpvSuccessResponse
		return decoder.Decode(&successResponse)
	}); err != nil {
		openErrorDialog(ctx, window, err)

		return
	}
}

func openAssistantWindow(ctx context.Context, app *adw.Application, manager *client.Manager, apiAddr, apiUsername, apiPassword string, settings *gio.Settings, gateway *server.Gateway, cancel func(), tmpDir string) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemeDefault)

	builder := gtk.NewBuilderFromString(assistantUI, len(assistantUI))

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	overlay := builder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)
	buttonHeaderbarTitle := builder.GetObject("button-headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarSubtitle := builder.GetObject("button-headerbar-subtitle").Cast().(*gtk.Label)
	previousButton := builder.GetObject("previous-button").Cast().(*gtk.Button)
	nextButton := builder.GetObject("next-button").Cast().(*gtk.Button)
	menuButton := builder.GetObject("menu-button").Cast().(*gtk.MenuButton)
	headerbarSpinner := builder.GetObject("headerbar-spinner").Cast().(*gtk.Spinner)
	stack := builder.GetObject("stack").Cast().(*gtk.Stack)
	magnetLinkEntry := builder.GetObject("magnet-link-entry").Cast().(*gtk.Entry)
	mediaSelectionGroup := builder.GetObject("media-selection-group").Cast().(*adw.PreferencesGroup)
	rightsConfirmationButton := builder.GetObject("rights-confirmation-button").Cast().(*gtk.CheckButton)
	downloadAndPlayButton := builder.GetObject("download-and-play-button").Cast().(*adw.SplitButton)
	streamWithoutDownloadingButton := builder.GetObject("stream-without-downloading-button").Cast().(*gtk.Button)
	streamPopover := builder.GetObject("stream-popover").Cast().(*gtk.Popover)
	mediaInfoDisplay := builder.GetObject("media-info-display").Cast().(*gtk.Box)
	mediaInfoButton := builder.GetObject("media-info-button").Cast().(*gtk.Button)

	descriptionBuilder := gtk.NewBuilderFromString(descriptionUI, len(descriptionUI))
	descriptionWindow := descriptionBuilder.GetObject("description-window").Cast().(*adw.Window)
	descriptionText := descriptionBuilder.GetObject("description-text").Cast().(*gtk.TextView)
	descriptionHeaderbarTitle := descriptionBuilder.GetObject("headerbar-title").Cast().(*gtk.Label)
	descriptionHeaderbarSubtitle := descriptionBuilder.GetObject("headerbar-subtitle").Cast().(*gtk.Label)

	warningBuilder := gtk.NewBuilderFromString(warningUI, len(warningUI))
	warningDialog := warningBuilder.GetObject("warning-dialog").Cast().(*gtk.MessageDialog)
	mpvFlathubDownloadButton := warningBuilder.GetObject("mpv-download-flathub-button").Cast().(*gtk.Button)
	mpvWebsiteDownloadButton := warningBuilder.GetObject("mpv-download-website-button").Cast().(*gtk.Button)
	mpvManualConfigurationButton := warningBuilder.GetObject("mpv-manual-configuration-button").Cast().(*gtk.Button)

	magnetLink := ""
	torrentTitle := ""
	torrentMedia := []media{}
	torrentReadme := ""
	isNewSession := true

	selectedTorrentMedia := ""
	activators := []*gtk.CheckButton{}
	mediaRows := []*adw.ActionRow{}

	subtitles := []mediaWithPriority{}

	community := ""
	password := ""
	key := ""

	bufferedMessages := []interface{}{}
	var bufferedPeer *wrtcconn.Peer
	var bufferedDecoder *json.Decoder

	var adapter *wrtcconn.Adapter
	var ids chan string
	var adapterCtx context.Context
	var cancelAdapterCtx func()

	stack.SetVisibleChildName(welcomePageName)

	magnetLinkEntry.ConnectChanged(func() {
		selectedTorrentMedia = ""
		for _, activator := range activators {
			activator.SetActive(false)
		}

		if magnetLinkEntry.Text() == "" {
			nextButton.SetSensitive(false)

			return
		}

		nextButton.SetSensitive(true)
	})

	onNext := func() {
		switch stack.VisibleChildName() {
		case welcomePageName:
			go func() {
				magnetLinkOrStreamCode := magnetLinkEntry.Text()
				u, err := url.Parse(magnetLinkOrStreamCode)
				if err == nil && u != nil && u.Scheme == "magnet" {
					isNewSession = true

					if selectedTorrentMedia == "" {
						nextButton.SetSensitive(false)
					}

					headerbarSpinner.SetSpinning(true)
					magnetLinkEntry.SetSensitive(false)

					log.Info().
						Str("magnetLink", magnetLinkOrStreamCode).
						Msg("Getting info for magnet link")

					info, err := manager.GetInfo(magnetLinkOrStreamCode)
					if err != nil {
						log.Warn().
							Str("magnetLink", magnetLinkOrStreamCode).
							Err(err).
							Msg("Could not get info for magnet link")

						toast := adw.NewToast("Could not get info for this magnet link.")

						overlay.AddToast(toast)

						headerbarSpinner.SetSpinning(false)
						magnetLinkEntry.SetSensitive(true)

						magnetLinkEntry.GrabFocus()

						return
					}

					torrentTitle = info.Name
					torrentReadme = strings.Map(
						func(r rune) rune {
							if r == '\n' || unicode.IsGraphic(r) && unicode.IsPrint(r) {
								return r
							}

							return -1
						},
						info.Description,
					)
					torrentMedia = []media{}
					for _, file := range info.Files {
						torrentMedia = append(torrentMedia, media{
							name: file.Path,
							size: int(file.Length),
						})
					}

					for _, row := range mediaRows {
						mediaSelectionGroup.Remove(row)
					}
					mediaRows = []*adw.ActionRow{}

					activators = []*gtk.CheckButton{}
					for i, file := range torrentMedia {
						row := adw.NewActionRow()

						activator := gtk.NewCheckButton()

						if len(activators) > 0 {
							activator.SetGroup(activators[i-1])
						}
						activators = append(activators, activator)

						m := file.name
						activator.SetActive(false)
						activator.ConnectActivate(func() {
							if m != selectedTorrentMedia {
								selectedTorrentMedia = m

								rightsConfirmationButton.SetActive(false)
							}

							nextButton.SetSensitive(true)
						})

						row.SetTitle(getDisplayPathWithoutRoot(file.name))
						row.SetSubtitle(fmt.Sprintf("%v MB", file.size/1000/1000))
						row.SetActivatable(true)

						row.AddPrefix(activator)
						row.SetActivatableWidget(activator)

						mediaRows = append(mediaRows, row)
						mediaSelectionGroup.Add(row)
					}

					headerbarSpinner.SetSpinning(false)
					magnetLinkEntry.SetSensitive(true)
					previousButton.SetVisible(true)

					buttonHeaderbarTitle.SetLabel(torrentTitle)
					descriptionHeaderbarTitle.SetLabel(torrentTitle)

					mediaInfoDisplay.SetVisible(false)
					mediaInfoButton.SetVisible(true)

					descriptionText.SetWrapMode(gtk.WrapWord)
					if !utf8.Valid([]byte(torrentReadme)) || strings.TrimSpace(torrentReadme) == "" {
						descriptionText.Buffer().SetText(readmePlaceholder)
					} else {
						descriptionText.Buffer().SetText(torrentReadme)
					}

					stack.SetVisibleChildName(mediaPageName)

					magnetLink = magnetLinkEntry.Text()

					return
				}

				go func() {
					log.Info().
						Str("streamCode", magnetLinkOrStreamCode).
						Msg("Joining session for stream code")

					isNewSession = false

					streamCodeParts := strings.Split(magnetLinkOrStreamCode, ":")
					if len(streamCodeParts) < 3 {
						toast := adw.NewToast("This stream code is invalid.")

						overlay.AddToast(toast)

						return
					}
					community, password, key = streamCodeParts[0], streamCodeParts[1], streamCodeParts[2]

					wu, err := url.Parse(settings.String(weronURLFlag))
					if err != nil {
						openErrorDialog(ctx, window, err)

						return
					}

					headerbarSpinner.SetSpinning(true)
					magnetLinkEntry.SetSensitive(false)

					q := wu.Query()
					q.Set("community", streamCodeParts[0])
					q.Set("password", streamCodeParts[1])
					wu.RawQuery = q.Encode()

					adapterCtx, cancelAdapterCtx = context.WithCancel(context.Background())

					adapter = wrtcconn.NewAdapter(
						wu.String(),
						streamCodeParts[2],
						strings.Split(settings.String(weronICEFlag), ","),
						[]string{"vintangle/sync"},
						&wrtcconn.AdapterConfig{
							Timeout:    time.Duration(time.Second * time.Duration(settings.Int64(weronTimeoutFlag))),
							ForceRelay: settings.Boolean(weronForceRelayFlag),
							OnSignalerReconnect: func() {
								log.Info().
									Str("raddr", settings.String(weronURLFlag)).
									Msg("Reconnecting to signaler")
							},
						},
						adapterCtx,
					)

					ids, err = adapter.Open()
					if err != nil {
						cancelAdapterCtx()

						openErrorDialog(ctx, window, err)

						return
					}

					var receivedMagnetLink api.Magnet
				l:
					for {
						select {
						case <-ctx.Done():
							if err := ctx.Err(); err != context.Canceled {
								openErrorDialog(ctx, window, err)

								adapter.Close()
								cancelAdapterCtx()

								return
							}

							adapter.Close()
							cancelAdapterCtx()

							return
						case rid := <-ids:
							log.Info().
								Str("raddr", settings.String(weronURLFlag)).
								Str("id", rid).
								Msg("Reconnecting to signaler")
						case peer := <-adapter.Accept():
							log.Info().
								Str("peerID", peer.PeerID).
								Str("channel", peer.ChannelID).
								Msg("Connected to peer")

							bufferedPeer = peer
							bufferedDecoder = json.NewDecoder(peer.Conn)

							for {
								var j interface{}
								if err := bufferedDecoder.Decode(&j); err != nil {
									log.Debug().
										Err(err).
										Msg("Could not decode structure, skipping")

									adapter.Close()
									cancelAdapterCtx()

									return
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

									receivedMagnetLink = m

									break l
								default:
									bufferedMessages = append(bufferedMessages, j)
								}
							}
						}
					}

					magnetLink = receivedMagnetLink.Magnet
					torrentTitle = receivedMagnetLink.Title
					torrentReadme = receivedMagnetLink.Description
					selectedTorrentMedia = receivedMagnetLink.Path

					torrentMedia = []media{}
					for _, subtitle := range receivedMagnetLink.Subtitles {
						torrentMedia = append(torrentMedia, media{
							name: subtitle.Name,
							size: subtitle.Size,
						})
					}

					headerbarSpinner.SetSpinning(false)
					magnetLinkEntry.SetSensitive(true)
					previousButton.SetVisible(true)

					buttonHeaderbarTitle.SetLabel(torrentTitle)
					descriptionHeaderbarTitle.SetLabel(torrentTitle)

					mediaInfoDisplay.SetVisible(false)
					mediaInfoButton.SetVisible(true)

					descriptionText.SetWrapMode(gtk.WrapWord)
					if !utf8.Valid([]byte(torrentReadme)) || strings.TrimSpace(torrentReadme) == "" {
						descriptionText.Buffer().SetText(readmePlaceholder)
					} else {
						descriptionText.Buffer().SetText(torrentReadme)
					}

					nextButton.SetVisible(false)

					buttonHeaderbarSubtitle.SetVisible(true)
					descriptionHeaderbarSubtitle.SetVisible(true)
					buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))
					descriptionHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))

					stack.SetVisibleChildName(readyPageName)
				}()
			}()
		case mediaPageName:
			nextButton.SetVisible(false)

			buttonHeaderbarSubtitle.SetVisible(true)
			descriptionHeaderbarSubtitle.SetVisible(true)
			buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))
			descriptionHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))

			stack.SetVisibleChildName(readyPageName)
		}
	}

	onPrevious := func() {
		switch stack.VisibleChildName() {
		case mediaPageName:
			previousButton.SetVisible(false)
			nextButton.SetSensitive(true)

			mediaInfoDisplay.SetVisible(true)
			mediaInfoButton.SetVisible(false)

			stack.SetVisibleChildName(welcomePageName)
		case readyPageName:
			nextButton.SetVisible(true)

			buttonHeaderbarSubtitle.SetVisible(false)
			descriptionHeaderbarSubtitle.SetVisible(false)

			if !isNewSession {
				if adapter != nil {
					adapter.Close()
				}

				if cancelAdapterCtx != nil {
					cancelAdapterCtx()
				}

				adapter = nil
				ids = nil
				adapterCtx = nil
				cancelAdapterCtx = nil

				community = ""
				password = ""
				key = ""

				previousButton.SetVisible(false)
				nextButton.SetSensitive(true)

				mediaInfoDisplay.SetVisible(true)
				mediaInfoButton.SetVisible(false)

				stack.SetVisibleChildName(welcomePageName)

				return
			}

			stack.SetVisibleChildName(mediaPageName)
		}
	}

	magnetLinkEntry.ConnectActivate(onNext)
	nextButton.ConnectClicked(onNext)
	previousButton.ConnectClicked(onPrevious)

	preferencesWindow, mpvCommandInput := addMainMenu(ctx, app, window, settings, menuButton, overlay, gateway, nil, cancel)

	mediaInfoButton.ConnectClicked(func() {
		descriptionWindow.Show()
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

	rightsConfirmationButton.ConnectToggled(func() {
		if rightsConfirmationButton.Active() {
			downloadAndPlayButton.AddCSSClass("suggested-action")
			downloadAndPlayButton.SetSensitive(true)

			return
		}

		downloadAndPlayButton.RemoveCSSClass("suggested-action")
		downloadAndPlayButton.SetSensitive(false)
	})

	refreshSubtitles := func() {
		subtitles = []mediaWithPriority{}
		for _, media := range torrentMedia {
			if media.name != selectedTorrentMedia {
				if strings.HasSuffix(media.name, ".srt") || strings.HasSuffix(media.name, ".vtt") || strings.HasSuffix(media.name, ".ass") {
					subtitles = append(subtitles, mediaWithPriority{
						media:    media,
						priority: 0,
					})
				} else {
					subtitles = append(subtitles, mediaWithPriority{
						media:    media,
						priority: 1,
					})
				}
			}
		}
	}

	downloadAndPlayButton.ConnectClicked(func() {
		window.Close()
		refreshSubtitles()

		streamURL, err := getStreamURL(apiAddr, magnetLink, selectedTorrentMedia)
		if err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		selectedTorrent, err := torrent.TorrentSpecFromMagnetUri(magnetLink)
		if err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		dstFile := filepath.Join(settings.String(storageFlag), "Manual Downloads", selectedTorrent.InfoHash.HexString(), selectedTorrentMedia)

		if err := os.MkdirAll(filepath.Dir(dstFile), os.ModePerm); err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		ctxDownload, cancel := context.WithCancel(context.Background())
		ready := make(chan struct{})
		if err := openControlsWindow(ctx, app, torrentTitle, subtitles, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, magnetLink, dstFile, settings, gateway, cancel, tmpDir, ready, cancel, adapter, ids, adapterCtx, cancelAdapterCtx, community, password, key, bufferedMessages, bufferedPeer, bufferedDecoder); err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		go func() {
			log.Info().
				Str("streamURL", streamURL).
				Msg("Downloading media")

			hc := &http.Client{}

			req, err := http.NewRequest(http.MethodGet, streamURL, http.NoBody)
			if err != nil {
				if err == context.Canceled {
					return
				}

				openErrorDialog(ctx, window, err)

				return
			}
			req.SetBasicAuth(apiUsername, apiPassword)

			res, err := hc.Do(req.WithContext(ctxDownload))
			if err != nil {
				if err == context.Canceled {
					return
				}

				openErrorDialog(ctx, window, err)

				return
			}
			if res.Body != nil {
				defer res.Body.Close()
			}
			if res.StatusCode != http.StatusOK {
				if err == context.Canceled {
					return
				}

				openErrorDialog(ctx, window, errors.New(res.Status))

				return
			}

			f, err := os.Create(dstFile)
			if err != nil {
				if err == context.Canceled {
					return
				}

				openErrorDialog(ctx, window, err)

				return
			}
			defer f.Close()

			if _, err := io.Copy(f, res.Body); err != nil {
				if err == context.Canceled {
					return
				}

				openErrorDialog(ctx, window, err)

				return
			}

			close(ready)
		}()
	})

	streamWithoutDownloadingButton.ConnectClicked(func() {
		streamPopover.Hide()

		window.Close()
		refreshSubtitles()

		streamURL, err := getStreamURL(apiAddr, magnetLink, selectedTorrentMedia)
		if err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		ready := make(chan struct{})
		if err := openControlsWindow(ctx, app, torrentTitle, subtitles, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, magnetLink, streamURL, settings, gateway, cancel, tmpDir, ready, func() {}, adapter, ids, adapterCtx, cancelAdapterCtx, community, password, key, bufferedMessages, bufferedPeer, bufferedDecoder); err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		close(ready)
	})

	if runtime.GOOS == "linux" {
		mpvFlathubDownloadButton.SetVisible(true)
		warningDialog.SetDefaultWidget(mpvFlathubDownloadButton)
	} else {
		warningDialog.SetDefaultWidget(mpvWebsiteDownloadButton)
	}

	mpvFlathubDownloadButton.ConnectClicked(func() {
		gtk.ShowURIFull(ctx, &window.Window, mpvFlathubURL, gdk.CURRENT_TIME, func(res gio.AsyncResulter) {
			warningDialog.Close()

			os.Exit(0)
		})
	})

	mpvWebsiteDownloadButton.ConnectClicked(func() {
		gtk.ShowURIFull(ctx, &window.Window, mpvWebsiteURL, gdk.CURRENT_TIME, func(res gio.AsyncResulter) {
			warningDialog.Close()

			os.Exit(0)
		})
	})

	mpvManualConfigurationButton.ConnectClicked(func() {
		warningDialog.Close()

		preferencesWindow.Show()
		mpvCommandInput.GrabFocus()
	})

	warningDialog.SetTransientFor(&window.Window)
	warningDialog.ConnectCloseRequest(func() (ok bool) {
		warningDialog.Close()
		warningDialog.SetVisible(false)

		return ok
	})

	app.AddWindow(&window.Window)

	window.ConnectShow(func() {
		if oldMPVCommand := settings.String(mpvFlag); strings.TrimSpace(oldMPVCommand) == "" {
			newMPVCommand, err := findWorkingMPV()
			if err != nil {
				warningDialog.Show()

				return
			}

			settings.SetString(mpvFlag, newMPVCommand)
			settings.Apply()
		}

		magnetLinkEntry.GrabFocus()
	})

	window.Show()

	return nil
}

func openControlsWindow(ctx context.Context, app *adw.Application, torrentTitle string, subtitles []mediaWithPriority, selectedTorrentMedia, torrentReadme string, manager *client.Manager, apiAddr, apiUsername, apiPassword, magnetLink, streamURL string, settings *gio.Settings, gateway *server.Gateway, cancel func(), tmpDir string, ready chan struct{}, cancelDownload func(), adapter *wrtcconn.Adapter, ids chan string, adapterCtx context.Context, cancelAdapterCtx func(), community, password, key string, bufferedMessages []interface{}, bufferedPeer *wrtcconn.Peer, bufferedDecoder *json.Decoder) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemePreferDark)

	builder := gtk.NewBuilderFromString(controlsUI, len(controlsUI))

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	overlay := builder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)
	buttonHeaderbarTitle := builder.GetObject("button-headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarSubtitle := builder.GetObject("button-headerbar-subtitle").Cast().(*gtk.Label)
	playButton := builder.GetObject("play-button").Cast().(*gtk.Button)
	stopButton := builder.GetObject("stop-button").Cast().(*gtk.Button)
	volumeButton := builder.GetObject("volume-button").Cast().(*gtk.VolumeButton)
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

	descriptionBuilder := gtk.NewBuilderFromString(descriptionUI, len(descriptionUI))
	descriptionWindow := descriptionBuilder.GetObject("description-window").Cast().(*adw.Window)
	descriptionText := descriptionBuilder.GetObject("description-text").Cast().(*gtk.TextView)
	descriptionHeaderbarTitle := descriptionBuilder.GetObject("headerbar-title").Cast().(*gtk.Label)
	descriptionHeaderbarSubtitle := descriptionBuilder.GetObject("headerbar-subtitle").Cast().(*gtk.Label)
	descriptionProgressBar := descriptionBuilder.GetObject("preparing-progress-bar").Cast().(*gtk.ProgressBar)

	subtitlesBuilder := gtk.NewBuilderFromString(subtitlesUI, len(subtitlesUI))
	subtitlesDialog := subtitlesBuilder.GetObject("subtitles-dialog").Cast().(*gtk.Dialog)
	subtitlesCancelButton := subtitlesBuilder.GetObject("button-cancel").Cast().(*gtk.Button)
	subtitlesOKButton := subtitlesBuilder.GetObject("button-ok").Cast().(*gtk.Button)
	subtitlesSelectionGroup := subtitlesBuilder.GetObject("subtitle-tracks").Cast().(*adw.PreferencesGroup)
	addSubtitlesFromFileButton := subtitlesBuilder.GetObject("add-from-file-button").Cast().(*gtk.Button)
	subtitlesOverlay := subtitlesBuilder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)

	audiotracksBuilder := gtk.NewBuilderFromString(audiotracksUI, len(audiotracksUI))
	audiotracksDialog := audiotracksBuilder.GetObject("audiotracks-dialog").Cast().(*gtk.Dialog)
	audiotracksCancelButton := audiotracksBuilder.GetObject("button-cancel").Cast().(*gtk.Button)
	audiotracksOKButton := audiotracksBuilder.GetObject("button-ok").Cast().(*gtk.Button)
	audiotracksSelectionGroup := audiotracksBuilder.GetObject("audiotracks").Cast().(*adw.PreferencesGroup)

	preparingBuilder := gtk.NewBuilderFromString(preparingUI, len(preparingUI))
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

	u, err := url.Parse(settings.String(weronURLFlag))
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
			strings.Split(settings.String(weronICEFlag), ","),
			[]string{"vintangle/sync"},
			&wrtcconn.AdapterConfig{
				Timeout:    time.Duration(time.Second * time.Duration(settings.Int64(weronTimeoutFlag))),
				ForceRelay: settings.Boolean(weronForceRelayFlag),
				OnSignalerReconnect: func() {
					log.Info().
						Str("raddr", settings.String(weronURLFlag)).
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

		if err := openAssistantWindow(ctx, app, manager, apiAddr, apiUsername, apiPassword, settings, gateway, cancel, tmpDir); err != nil {
			openErrorDialog(ctx, window, err)

			return
		}
	})

	mediaInfoButton.ConnectClicked(func() {
		descriptionWindow.Show()
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
				openErrorDialog(ctx, window, err)

				return
			}

			length := float64(0)
			completed := float64(0)
			peers := 0

		l:
			for _, t := range metrics {
				selectedTorrent, err := torrent.TorrentSpecFromMagnetUri(magnetLink)
				if err != nil {
					openErrorDialog(ctx, window, err)

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

		if err := openAssistantWindow(ctx, app, manager, apiAddr, apiUsername, apiPassword, settings, gateway, cancel, tmpDir); err != nil {
			openErrorDialog(ctx, window, err)

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
		shell = []string{"cmd", "/c"}
	}
	commandLine := append(shell, fmt.Sprintf("%v '--no-sub-visibility' '--keep-open=always' '--no-osc' '--no-input-default-bindings' '--pause' '--input-ipc-server=%v' '--http-header-fields=Authorization: Basic %v' '%v'", settings.String(mpvFlag), ipcFile, usernameAndPassword, streamURL))

	command := exec.Command(
		commandLine[0],
		commandLine[1:]...,
	)
	if runtime.GOOS != "windows" {
		command.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}
	}

	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	addMainMenu(
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
					openErrorDialog(ctx, window, err)

					return
				}
			}
		},
	)

	app.AddWindow(&window.Window)

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		log.Debug().Msg("Gracefully shutting down")

		window.Close()
	}()

	window.ConnectShow(func() {
		preparingWindow.Show()

		go func() {
			<-ready

			if err := command.Start(); err != nil {
				openErrorDialog(ctx, window, err)

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
				if runtime.GOOS == "windows" {
					if err := command.Process.Kill(); err != nil {
						openErrorDialog(ctx, window, err)

						return false
					}
				} else {
					if err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL); err != nil {
						openErrorDialog(ctx, window, err)

						return false
					}
				}
			}

			if err := os.RemoveAll(ipcDir); err != nil {
				openErrorDialog(ctx, window, err)

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

				if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Info().Msg("Starting playback")

					if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "pause", false}}); err != nil {
						return err
					}

					var successResponse mpvSuccessResponse
					return decoder.Decode(&successResponse)
				}); err != nil {
					openErrorDialog(ctx, window, err)

					return
				}
			}

			pausePlayback := func() {
				playButton.SetIconName(playIcon)

				if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Info().Msg("Pausing playback")

					if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "pause", true}}); err != nil {
						return err
					}

					var successResponse mpvSuccessResponse
					return decoder.Decode(&successResponse)
				}); err != nil {
					openErrorDialog(ctx, window, err)

					return
				}
			}

			var trackListResponse mpvTrackListResponse
			if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
				log.Debug().Msg("Getting tracklist")

				if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "track-list"}}); err != nil {
					return err
				}

				return decoder.Decode(&trackListResponse)
			}); err != nil {
				openErrorDialog(ctx, window, err)

				return
			}

			audiotracks := []audioTrack{}
			for _, track := range trackListResponse.Data {
				if track.Type == mpvTypeAudio {
					audiotracks = append(audiotracks, audioTrack{
						lang: track.Lang,
						id:   track.ID,
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

				if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpvCommand{[]interface{}{"seek", int64(elapsed.Seconds()), "absolute"}}); err != nil {
						return err
					}

					var successResponse mpvSuccessResponse
					return decoder.Decode(&successResponse)
				}); err != nil {
					openErrorDialog(ctx, window, err)

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

				var elapsedResponse mpvFloat64Response
				if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "time-pos"}}); err != nil {
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
						openErrorDialog(ctx, window, err)

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
							openErrorDialog(ctx, window, err)

							return
						}

						return
					case rid := <-ids:
						log.Info().
							Str("raddr", settings.String(weronURLFlag)).
							Str("id", rid).
							Msg("Reconnecting to signaler")
					case peer := <-adapter.Accept():
						go handlePeer(peer, nil)
					}
				}
			}()

			if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
				if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "volume", 100}}); err != nil {
					return err
				}

				var successResponse mpvSuccessResponse
				return decoder.Decode(&successResponse)
			}); err != nil {
				openErrorDialog(ctx, window, err)

				return
			}

			subtitleActivators := []*gtk.CheckButton{}

			for i, file := range append(
				[]mediaWithPriority{
					{media: media{
						name: "None",
						size: 0,
					},
						priority: -1,
					},
				},
				subtitles...) {
				row := adw.NewActionRow()

				activator := gtk.NewCheckButton()

				if len(subtitleActivators) > 0 {
					activator.SetGroup(subtitleActivators[i-1])
				}
				subtitleActivators = append(subtitleActivators, activator)

				m := file.name
				j := i
				activator.SetActive(false)
				activator.ConnectActivate(func() {
					if j == 0 {
						log.Info().
							Msg("Disabling subtitles")

						if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
							if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "sid", "no"}}); err != nil {
								return err
							}

							var successResponse mpvSuccessResponse
							return decoder.Decode(&successResponse)
						}); err != nil {
							openErrorDialog(ctx, window, err)

							return
						}

						if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
							if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "sub-visibility", "no"}}); err != nil {
								return err
							}

							var successResponse mpvSuccessResponse
							return decoder.Decode(&successResponse)
						}); err != nil {
							openErrorDialog(ctx, window, err)

							return
						}

						return
					}

					streamURL, err := getStreamURL(apiAddr, magnetLink, m)
					if err != nil {
						openErrorDialog(ctx, window, err)

						return
					}

					log.Info().
						Str("streamURL", streamURL).
						Msg("Downloading subtitles")

					hc := &http.Client{}

					req, err := http.NewRequest(http.MethodGet, streamURL, http.NoBody)
					if err != nil {
						openErrorDialog(ctx, window, err)

						return
					}
					req.SetBasicAuth(apiUsername, apiPassword)

					res, err := hc.Do(req)
					if err != nil {
						openErrorDialog(ctx, window, err)

						return
					}
					if res.Body != nil {
						defer res.Body.Close()
					}
					if res.StatusCode != http.StatusOK {
						openErrorDialog(ctx, window, errors.New(res.Status))

						return
					}

					setSubtitles(ctx, window, m, res.Body, tmpDir, ipcFile, subtitleActivators[0], subtitlesOverlay)
				})

				if i == 0 {
					row.SetTitle(file.name)
					row.SetSubtitle("Disable subtitles")

					activator.SetActive(true)
				} else if file.priority == 0 {
					row.SetTitle(getDisplayPathWithoutRoot(file.name))
					row.SetSubtitle("Integrated subtitle")
				} else {
					row.SetTitle(getDisplayPathWithoutRoot(file.name))
					row.SetSubtitle("Extra file from media")
				}

				row.SetActivatable(true)

				row.AddPrefix(activator)
				row.SetActivatableWidget(activator)

				subtitlesSelectionGroup.Add(row)
			}

			audiotrackActivators := []*gtk.CheckButton{}

			for i, audiotrack := range audiotracks {
				row := adw.NewActionRow()

				activator := gtk.NewCheckButton()

				if len(audiotrackActivators) > 0 {
					activator.SetGroup(audiotrackActivators[i-1])
				}
				audiotrackActivators = append(audiotrackActivators, activator)

				a := audiotrack
				activator.SetActive(false)
				activator.ConnectActivate(func() {
					log.Info().
						Int("aid", a.id).
						Msg("Selecting audio track")
				})

				row.SetSubtitle(fmt.Sprintf("Track %v", a.id))
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
					var durationResponse mpvFloat64Response
					if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "duration"}}); err != nil {
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
						openErrorDialog(ctx, window, err)

						return
					}

					if total != 0 && !preparingClosed {
						preparingWindow.Close()

						preparingClosed = true
					}

					var elapsedResponse mpvFloat64Response
					if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "time-pos"}}); err != nil {
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
						openErrorDialog(ctx, window, err)

						return
					}

					var pausedResponse mpvBoolResponse
					if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "core-idle"}}); err != nil {
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

			volumeButton.ConnectValueChanged(func(value float64) {
				if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Info().
						Float64("value", value).
						Msg("Setting volume")

					if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "volume", value * 100}}); err != nil {
						return err
					}

					var successResponse mpvSuccessResponse
					return decoder.Decode(&successResponse)
				}); err != nil {
					openErrorDialog(ctx, window, err)

					return
				}
			})

			subtitleButton.ConnectClicked(func() {
				subtitlesDialog.Show()
			})

			audiotracksButton.ConnectClicked(func() {
				audiotracksDialog.Show()
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

				if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "sid", "no"}}); err != nil {
						return err
					}

					var successResponse mpvSuccessResponse
					return decoder.Decode(&successResponse)
				}); err != nil {
					openErrorDialog(ctx, window, err)

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
							openErrorDialog(ctx, window, err)

							return
						}
						defer subtitlesFile.Close()

						setSubtitles(ctx, window, m, subtitlesFile, tmpDir, ipcFile, subtitleActivators[0], subtitlesOverlay)

						row := adw.NewActionRow()

						activator := gtk.NewCheckButton()

						activator.SetGroup(subtitleActivators[len(subtitleActivators)-1])
						subtitleActivators = append(subtitleActivators, activator)

						activator.SetActive(true)
						activator.ConnectActivate(func() {
							m := filePicker.File().Path()
							subtitlesFile, err := os.Open(m)
							if err != nil {
								openErrorDialog(ctx, window, err)

								return
							}
							defer subtitlesFile.Close()

							setSubtitles(ctx, window, m, subtitlesFile, tmpDir, ipcFile, subtitleActivators[0], subtitlesOverlay)
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
					if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
						log.Info().Msg("Enabling fullscreen")

						if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "fullscreen", true}}); err != nil {
							return err
						}

						var successResponse mpvSuccessResponse
						return decoder.Decode(&successResponse)
					}); err != nil {
						openErrorDialog(ctx, window, err)

						return
					}

					return
				}

				if err := runMPVCommand(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Info().Msg("Disabling fullscreen")

					if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "fullscreen", false}}); err != nil {
						return err
					}

					var successResponse mpvSuccessResponse
					return decoder.Decode(&successResponse)
				}); err != nil {
					openErrorDialog(ctx, window, err)

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
					openErrorDialog(ctx, window, err)

					return
				}

				done <- struct{}{}

				window.Destroy()
			}()

			playButton.GrabFocus()
		}()
	})

	window.Show()

	return nil
}

func addMainMenu(ctx context.Context, app *adw.Application, window *adw.ApplicationWindow, settings *gio.Settings, menuButton *gtk.MenuButton, overlay *adw.ToastOverlay, gateway *server.Gateway, getMagnetLink func() string, cancel func()) (*adw.PreferencesWindow, *gtk.Entry) {
	menuBuilder := gtk.NewBuilderFromString(menuUI, len(menuUI))
	menu := menuBuilder.GetObject("main-menu").Cast().(*gio.Menu)

	aboutBuilder := gtk.NewBuilderFromString(aboutUI, len(aboutUI))
	aboutDialog := aboutBuilder.GetObject("about-dialog").Cast().(*gtk.AboutDialog)

	preferencesBuilder := gtk.NewBuilderFromString(preferencesUI, len(preferencesUI))
	preferencesWindow := preferencesBuilder.GetObject("preferences-window").Cast().(*adw.PreferencesWindow)
	storageLocationInput := preferencesBuilder.GetObject("storage-location-input").Cast().(*gtk.Button)
	mpvCommandInput := preferencesBuilder.GetObject("mpv-command-input").Cast().(*gtk.Entry)
	verbosityLevelInput := preferencesBuilder.GetObject("verbosity-level-input").Cast().(*gtk.SpinButton)
	remoteGatewaySwitchInput := preferencesBuilder.GetObject("htorrent-remote-gateway-switch").Cast().(*gtk.Switch)
	remoteGatewayURLInput := preferencesBuilder.GetObject("htorrent-url-input").Cast().(*gtk.Entry)
	remoteGatewayUsernameInput := preferencesBuilder.GetObject("htorrent-username-input").Cast().(*gtk.Entry)
	remoteGatewayPasswordInput := preferencesBuilder.GetObject("htorrent-password-input").Cast().(*gtk.Entry)
	remoteGatewayURLRow := preferencesBuilder.GetObject("htorrent-url-row").Cast().(*adw.ActionRow)
	remoteGatewayUsernameRow := preferencesBuilder.GetObject("htorrent-username-row").Cast().(*adw.ActionRow)
	remoteGatewayPasswordRow := preferencesBuilder.GetObject("htorrent-password-row").Cast().(*adw.ActionRow)
	weronURLInput := preferencesBuilder.GetObject("weron-url-input").Cast().(*gtk.Entry)
	weronICEInput := preferencesBuilder.GetObject("weron-ice-input").Cast().(*gtk.Entry)
	weronTimeoutInput := preferencesBuilder.GetObject("weron-timeout-input").Cast().(*gtk.SpinButton)
	weronForceRelayInput := preferencesBuilder.GetObject("weron-force-relay-input").Cast().(*gtk.Switch)

	preferencesHaveChanged := false

	preferencesAction := gio.NewSimpleAction(preferencesActionName, nil)
	preferencesAction.ConnectActivate(func(parameter *glib.Variant) {
		preferencesWindow.Show()
	})
	app.SetAccelsForAction(preferencesActionName, []string{`<Primary>comma`})
	window.AddAction(preferencesAction)

	openDownloadsAction := gio.NewSimpleAction(openDownloadsActionName, nil)
	openDownloadsAction.ConnectActivate(func(parameter *glib.Variant) {
		if err := gio.AppInfoLaunchDefaultForURI(fmt.Sprintf("file://%v", settings.String(storageFlag)), nil); err != nil {
			openErrorDialog(ctx, window, err)

			return
		}
	})
	window.AddAction(openDownloadsAction)

	if getMagnetLink != nil {
		copyMagnetLinkAction := gio.NewSimpleAction(copyMagnetLinkActionName, nil)
		copyMagnetLinkAction.ConnectActivate(func(parameter *glib.Variant) {
			window.Clipboard().SetText(getMagnetLink())
		})
		window.AddAction(copyMagnetLinkAction)
	}

	preferencesWindow.SetTransientFor(&window.Window)
	preferencesWindow.ConnectCloseRequest(func() (ok bool) {
		preferencesWindow.Close()
		preferencesWindow.SetVisible(false)

		if preferencesHaveChanged {
			settings.Apply()

			toast := adw.NewToast("Reopen to apply the changes.")
			toast.SetButtonLabel("Reopen")
			toast.SetActionName("win." + applyPreferencesActionName)

			overlay.AddToast(toast)
		}

		preferencesHaveChanged = false

		return ok
	})

	syncSensitivityState := func() {
		if remoteGatewaySwitchInput.State() {
			remoteGatewayURLRow.SetSensitive(true)
			remoteGatewayUsernameRow.SetSensitive(true)
			remoteGatewayPasswordRow.SetSensitive(true)
		} else {
			remoteGatewayURLRow.SetSensitive(false)
			remoteGatewayUsernameRow.SetSensitive(false)
			remoteGatewayPasswordRow.SetSensitive(false)
		}
	}
	preferencesWindow.ConnectShow(syncSensitivityState)

	applyPreferencesAction := gio.NewSimpleAction(applyPreferencesActionName, nil)
	applyPreferencesAction.ConnectActivate(func(parameter *glib.Variant) {
		cancel()

		if gateway != nil {
			if err := gateway.Close(); err != nil {
				openErrorDialog(ctx, window, err)

				return
			}
		}

		ex, err := os.Executable()
		if err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		if _, err := syscall.ForkExec(
			ex,
			os.Args,
			&syscall.ProcAttr{
				Env:   os.Environ(),
				Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
			},
		); err != nil {
			openErrorDialog(ctx, window, err)

			return
		}

		os.Exit(0)
	})
	window.AddAction(applyPreferencesAction)

	storageLocationInput.ConnectClicked(func() {
		filePicker := gtk.NewFileChooserNative(
			"Select storage location",
			&preferencesWindow.Window.Window,
			gtk.FileChooserActionSelectFolder,
			"",
			"")
		filePicker.SetModal(true)
		filePicker.ConnectResponse(func(responseId int) {
			if responseId == int(gtk.ResponseAccept) {
				settings.SetString(storageFlag, filePicker.File().Path())

				preferencesHaveChanged = true
			}

			filePicker.Destroy()
		})

		filePicker.Show()
	})

	settings.Bind(mpvFlag, mpvCommandInput.Object, "text", gio.SettingsBindDefault)

	verbosityLevelInput.SetAdjustment(gtk.NewAdjustment(0, 0, 8, 1, 1, 1))
	settings.Bind(verboseFlag, verbosityLevelInput.Object, "value", gio.SettingsBindDefault)

	settings.Bind(gatewayRemoteFlag, remoteGatewaySwitchInput.Object, "active", gio.SettingsBindDefault)
	settings.Bind(gatewayURLFlag, remoteGatewayURLInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(gatewayUsernameFlag, remoteGatewayUsernameInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(gatewayPasswordFlag, remoteGatewayPasswordInput.Object, "text", gio.SettingsBindDefault)

	settings.Bind(weronURLFlag, weronURLInput.Object, "text", gio.SettingsBindDefault)

	weronTimeoutInput.SetAdjustment(gtk.NewAdjustment(0, 0, math.MaxFloat64, 1, 1, 1))
	settings.Bind(weronTimeoutFlag, weronTimeoutInput.Object, "value", gio.SettingsBindDefault)

	settings.Bind(weronICEFlag, weronICEInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(weronForceRelayFlag, weronForceRelayInput.Object, "active", gio.SettingsBindDefault)

	mpvCommandInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	verbosityLevelInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})

	remoteGatewaySwitchInput.ConnectStateSet(func(state bool) (ok bool) {
		preferencesHaveChanged = true

		remoteGatewaySwitchInput.SetState(state)

		syncSensitivityState()

		return true
	})

	remoteGatewayURLInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	remoteGatewayUsernameInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	remoteGatewayPasswordInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})

	weronURLInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	weronTimeoutInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	weronICEInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})

	weronForceRelayInput.ConnectStateSet(func(state bool) (ok bool) {
		preferencesHaveChanged = true

		weronForceRelayInput.SetState(state)

		return true
	})

	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(parameter *glib.Variant) {
		aboutDialog.Show()
	})
	window.AddAction(aboutAction)

	aboutDialog.SetTransientFor(&window.Window)
	aboutDialog.ConnectCloseRequest(func() (ok bool) {
		aboutDialog.Close()
		aboutDialog.SetVisible(false)

		return ok
	})

	menuButton.SetMenuModel(menu)

	return preferencesWindow, mpvCommandInput
}

func openErrorDialog(ctx context.Context, window *adw.ApplicationWindow, err error) {
	log.Error().
		Err(err).
		Msg("Could not continue due to a fatal error")

	errorBuilder := gtk.NewBuilderFromString(errorUI, len(errorUI))
	errorDialog := errorBuilder.GetObject("error-dialog").Cast().(*gtk.MessageDialog)
	reportErrorButton := errorBuilder.GetObject("report-error-button").Cast().(*gtk.Button)
	closeVintangleButton := errorBuilder.GetObject("close-vintangle-button").Cast().(*gtk.Button)

	errorDialog.Object.SetObjectProperty("secondary-text", err.Error())

	errorDialog.SetDefaultWidget(reportErrorButton)
	errorDialog.SetTransientFor(&window.Window)
	errorDialog.ConnectCloseRequest(func() (ok bool) {
		errorDialog.Close()
		errorDialog.SetVisible(false)

		return ok
	})

	reportErrorButton.ConnectClicked(func() {
		gtk.ShowURIFull(ctx, &window.Window, issuesURL, gdk.CURRENT_TIME, func(res gio.AsyncResulter) {
			errorDialog.Close()

			os.Exit(1)
		})
	})

	closeVintangleButton.ConnectClicked(func() {
		errorDialog.Close()

		os.Exit(1)
	})

	errorDialog.Show()
}

func main() {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "vintangle-gschemas")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "gschemas.compiled"), geschemas, os.ModePerm); err != nil {
		panic(err)
	}

	if err := os.Setenv(schemaDirEnvVar, tmpDir); err != nil {
		panic(err)
	}

	settings := gio.NewSettings(stateID)

	if storage := settings.String(storageFlag); strings.TrimSpace(storage) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}

		downloadPath := filepath.Join(home, "Downloads", "Vintangle")

		settings.SetString(storageFlag, downloadPath)

		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			panic(err)
		}

		settings.Apply()
	}

	configureZerolog := func(verbose int64) {
		switch verbose {
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
	}

	configureZerolog(settings.Int64(verboseFlag))
	settings.ConnectChanged(func(key string) {
		if key == verboseFlag {
			configureZerolog(settings.Int64(verboseFlag))
		}
	})

	app := adw.NewApplication(appID, gio.ApplicationNonUnique)

	prov := gtk.NewCSSProvider()
	prov.LoadFromData(styleCSS)

	var gateway *server.Gateway
	ctx, cancel := context.WithCancel(context.Background())

	app.ConnectActivate(func() {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
		)

		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			panic(err)
		}

		port, err := freeport.GetFreePort()
		if err != nil {
			panic(err)
		}
		addr.Port = port

		rand.Seed(time.Now().UnixNano())

		if err := os.MkdirAll(settings.String(storageFlag), os.ModePerm); err != nil {
			panic(err)
		}

		apiAddr := settings.String(gatewayURLFlag)
		apiUsername := settings.String(gatewayUsernameFlag)
		apiPassword := settings.String(gatewayPasswordFlag)
		if !settings.Boolean(gatewayRemoteFlag) {
			apiUsername = randSeq(20)
			apiPassword = randSeq(20)

			gateway = server.NewGateway(
				addr.String(),
				settings.String(storageFlag),
				apiUsername,
				apiPassword,
				"",
				"",
				settings.Int64(verboseFlag) > 5,
				func(torrentMetrics v1.TorrentMetrics, fileMetrics v1.FileMetrics) {
					log.Info().
						Str("magnet", torrentMetrics.Magnet).
						Int("peers", torrentMetrics.Peers).
						Str("path", fileMetrics.Path).
						Int64("length", fileMetrics.Length).
						Int64("completed", fileMetrics.Completed).
						Msg("Streaming")
				},
				ctx,
			)

			if err := gateway.Open(); err != nil {
				panic(err)
			}

			go func() {
				log.Info().
					Str("address", addr.String()).
					Msg("Gateway listening")

				if err := gateway.Wait(); err != nil {
					panic(err)
				}
			}()

			apiAddr = "http://" + addr.String()
		}

		manager := client.NewManager(
			apiAddr,
			apiUsername,
			apiPassword,
			ctx,
		)

		if err := openAssistantWindow(ctx, app, manager, apiAddr, apiUsername, apiPassword, settings, gateway, cancel, tmpDir); err != nil {
			panic(err)
		}
	})

	app.ConnectShutdown(func() {
		cancel()

		if gateway != nil {
			if err := gateway.Close(); err != nil {
				panic(err)
			}
		}
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
