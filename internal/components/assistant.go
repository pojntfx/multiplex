package components

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/anacrolix/torrent"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/internal/resources"
	api "github.com/pojntfx/multiplex/pkg/api/webrtc/v1"
	mpvClient "github.com/pojntfx/multiplex/pkg/client"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog/log"
	"github.com/rymdport/portal/openuri"
)

const (
	welcomePageName = "welcome-page"
	mediaPageName   = "media-page"
	readyPageName   = "ready-page"

	mpvFlathubURL = "https://flathub.org/apps/details/io.mpv.Mpv"
	mpvWebsiteURL = "https://mpv.io/installation/"

	preferencesActionName      = "preferences"
	applyPreferencesActionName = "applypreferences"
	openDownloadsActionName    = "opendownloads"
	copyMagnetLinkActionName   = "copymagnetlink"

	responseDownloadFlathub     = "download-flathub"
	responseDownloadWebsite     = "download-website"
	responseManualConfiguration = "manual-configuration"
)

func OpenAssistantWindow(
	ctx context.Context,
	app *adw.Application,

	manager *client.Manager,
	apiAddr, apiUsername,
	apiPassword string,

	settings *gio.Settings,
	gateway *server.Gateway,
	cancel func(),
	tmpDir string,
) error {
	app.GetStyleManager().SetColorScheme(adw.ColorSchemeDefaultValue)

	builder := gtk.NewBuilderFromResource(resources.GResourceAssistantPath)

	var window adw.ApplicationWindow
	builder.GetObject("main-window").Cast(&window)
	var overlay adw.ToastOverlay
	builder.GetObject("toast-overlay").Cast(&overlay)
	var buttonHeaderbarTitle gtk.Label
	builder.GetObject("button-headerbar-title").Cast(&buttonHeaderbarTitle)
	var buttonHeaderbarSubtitle gtk.Label
	builder.GetObject("button-headerbar-subtitle").Cast(&buttonHeaderbarSubtitle)
	var previousButton gtk.Button
	builder.GetObject("previous-button").Cast(&previousButton)
	var nextButton gtk.Button
	builder.GetObject("next-button").Cast(&nextButton)
	var menuButton gtk.MenuButton
	builder.GetObject("menu-button").Cast(&menuButton)
	var headerbarSpinner gtk.Spinner
	builder.GetObject("headerbar-spinner").Cast(&headerbarSpinner)
	var stack gtk.Stack
	builder.GetObject("stack").Cast(&stack)
	var magnetLinkEntry gtk.Entry
	builder.GetObject("magnet-link-entry").Cast(&magnetLinkEntry)
	var mediaSelectionGroup adw.PreferencesGroup
	builder.GetObject("media-selection-group").Cast(&mediaSelectionGroup)
	var rightsConfirmationButton gtk.CheckButton
	builder.GetObject("rights-confirmation-button").Cast(&rightsConfirmationButton)
	var downloadAndPlayButton adw.SplitButton
	builder.GetObject("download-and-play-button").Cast(&downloadAndPlayButton)
	var streamWithoutDownloadingButton gtk.Button
	builder.GetObject("stream-without-downloading-button").Cast(&streamWithoutDownloadingButton)
	var streamPopover gtk.Popover
	builder.GetObject("stream-popover").Cast(&streamPopover)
	var mediaInfoDisplay gtk.Box
	builder.GetObject("media-info-display").Cast(&mediaInfoDisplay)
	var mediaInfoButton gtk.Button
	builder.GetObject("media-info-button").Cast(&mediaInfoButton)

	descriptionBuilder := gtk.NewBuilderFromResource(resources.GResourceDescriptionPath)
	var descriptionWindow adw.Window
	descriptionBuilder.GetObject("description-window").Cast(&descriptionWindow)
	var descriptionText gtk.TextView
	descriptionBuilder.GetObject("description-text").Cast(&descriptionText)
	var descriptionHeaderbarTitle gtk.Label
	descriptionBuilder.GetObject("headerbar-title").Cast(&descriptionHeaderbarTitle)
	var descriptionHeaderbarSubtitle gtk.Label
	descriptionBuilder.GetObject("headerbar-subtitle").Cast(&descriptionHeaderbarSubtitle)

	warningBuilder := gtk.NewBuilderFromResource(resources.GResourceWarningPath)
	var warningDialog adw.AlertDialog
	warningBuilder.GetObject("warning-dialog").Cast(&warningDialog)

	magnetLink := ""
	torrentTitle := ""
	torrentMedia := []media{}
	torrentReadme := ""
	isNewSession := true

	selectedTorrentMedia := ""
	activators := []*gtk.CheckButton{}
	mediaRows := []*adw.ActionRow{}

	subtitles := []mediaWithPriorityAndID{}

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

	// Note: Entry doesn't have ConnectChanged in puregotk, text changes handled via ConnectActivate

	onNext := func() {
		switch stack.GetVisibleChildName() {
		case welcomePageName:
			go func() {
				magnetLinkOrStreamCode := magnetLinkEntry.GetText()
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

					knownMedia := []media{}
					extraFiles := []media{}
					for _, file := range info.Files {
						m := media{
							name: file.Path,
							size: int(file.Length),
						}

						if strings.HasSuffix(file.Path, ".mkv") || strings.HasSuffix(file.Path, ".mp4") || strings.HasSuffix(file.Path, ".m4v") || strings.HasSuffix(file.Path, ".mov") || strings.HasSuffix(file.Path, ".avi") || strings.HasSuffix(file.Path, ".webm") {
							knownMedia = append(knownMedia, m)
						} else {
							extraFiles = append(extraFiles, m)
						}
					}

					sort.Slice(knownMedia, func(i, j int) bool {
						return knownMedia[i].size < knownMedia[j].size
					})
					sort.Slice(extraFiles, func(i, j int) bool {
						return extraFiles[i].size < extraFiles[j].size
					})
					torrentMedia = append(knownMedia, extraFiles...)

					knownMediaWithPriority := []mediaWithPriorityAndID{}
					for _, media := range knownMedia {
						knownMediaWithPriority = append(knownMediaWithPriority, mediaWithPriorityAndID{
							media:    media,
							priority: 0,
						})
					}

					extraFilesWithPriority := []mediaWithPriorityAndID{}
					for _, media := range extraFiles {
						extraFilesWithPriority = append(extraFilesWithPriority, mediaWithPriorityAndID{
							media:    media,
							priority: 1,
						})
					}

					for _, row := range mediaRows {
						mediaSelectionGroup.Remove(&row.PreferencesRow.Widget)
					}
					mediaRows = []*adw.ActionRow{}

					activators = []*gtk.CheckButton{}
					for _, file := range append(knownMediaWithPriority, extraFilesWithPriority...) {
						row := adw.NewActionRow()

						activator := gtk.NewCheckButton()

						if len(activators) > 0 {
							activator.SetGroup(activators[len(activators)-1])
						}
						activators = append(activators, activator)

						m := file.name
						activator.SetActive(false)
						activateCallback := func(gtk.CheckButton) {
							if m != selectedTorrentMedia {
								selectedTorrentMedia = m

								rightsConfirmationButton.SetActive(false)
							}

							nextButton.SetSensitive(true)
						}
						activator.ConnectActivate(&activateCallback)

						row.SetTitle(getDisplayPathWithoutRoot(file.name))
						if file.priority == 0 {
							row.SetSubtitle(fmt.Sprintf("Media (%v MB)", file.size/1000/1000))
						} else {
							row.SetSubtitle(fmt.Sprintf("Extra file (%v MB)", file.size/1000/1000))
						}
						row.SetActivatable(true)

						row.AddPrefix(&activator.Widget)
						row.SetActivatableWidget(&activator.Widget)

						mediaRows = append(mediaRows, row)
						mediaSelectionGroup.Add(&row.PreferencesRow.Widget)
					}

					headerbarSpinner.SetSpinning(false)
					magnetLinkEntry.SetSensitive(true)
					previousButton.SetVisible(true)

					buttonHeaderbarTitle.SetLabel(torrentTitle)
					descriptionHeaderbarTitle.SetLabel(torrentTitle)

					mediaInfoDisplay.SetVisible(false)
					mediaInfoButton.SetVisible(true)

					descriptionText.SetWrapMode(gtk.WrapWordValue)
					if !utf8.Valid([]byte(torrentReadme)) || strings.TrimSpace(torrentReadme) == "" {
						descriptionText.GetBuffer().SetText(readmePlaceholder, -1)
					} else {
						descriptionText.GetBuffer().SetText(torrentReadme, -1)
					}

					stack.SetVisibleChildName(mediaPageName)

					magnetLink = magnetLinkEntry.GetText()

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

					wu, err := url.Parse(settings.GetString(resources.GSchemaWeronURLKey))
					if err != nil {
						OpenErrorDialog(ctx, &window, err)

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
						strings.Split(settings.GetString(resources.GSchemaWeronICEKey), ","),
						[]string{"multiplex/sync"},
						&wrtcconn.AdapterConfig{
							Timeout:    time.Duration(time.Second * time.Duration(settings.GetInt64(resources.GSchemaWeronTimeoutKey))),
							ForceRelay: settings.GetBoolean(resources.GSchemaWeronForceRelayKey),
							OnSignalerReconnect: func() {
								log.Info().
									Str("raddr", settings.GetString(resources.GSchemaWeronURLKey)).
									Msg("Reconnecting to signaler")
							},
						},
						adapterCtx,
					)

					ids, err = adapter.Open()
					if err != nil {
						cancelAdapterCtx()

						OpenErrorDialog(ctx, &window, err)

						return
					}

					var receivedMagnetLink api.Magnet
				l:
					for {
						select {
						case <-ctx.Done():
							if err := ctx.Err(); err != context.Canceled {
								OpenErrorDialog(ctx, &window, err)

								adapter.Close()
								cancelAdapterCtx()

								return
							}

							adapter.Close()
							cancelAdapterCtx()

							return
						case rid := <-ids:
							log.Info().
								Str("raddr", settings.GetString(resources.GSchemaWeronURLKey)).
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

					descriptionText.SetWrapMode(gtk.WrapWordValue)
					if !utf8.Valid([]byte(torrentReadme)) || strings.TrimSpace(torrentReadme) == "" {
						descriptionText.GetBuffer().SetText(readmePlaceholder, -1)
					} else {
						descriptionText.GetBuffer().SetText(torrentReadme, -1)
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
		switch stack.GetVisibleChildName() {
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

	activateCallback := func(gtk.Entry) {
		onNext()
	}
	magnetLinkEntry.ConnectActivate(&activateCallback)

	clickedCallbackNext := func(gtk.Button) {
		onNext()
	}
	nextButton.ConnectClicked(&clickedCallbackNext)

	clickedCallbackPrevious := func(gtk.Button) {
		onPrevious()
	}
	previousButton.ConnectClicked(&clickedCallbackPrevious)

	preferencesDialog, mpvCommandInput := AddMainMenu(ctx, app, &window, settings, &menuButton, &overlay, gateway, nil, cancel)

	clickedCallback3 := func(gtk.Button) {
		descriptionWindow.SetVisible(true)
	}
	mediaInfoButton.ConnectClicked(&clickedCallback3)

	ctrl := gtk.NewEventControllerKey()
	descriptionWindow.AddController(&ctrl.EventController)
	descriptionWindow.SetTransientFor(&window.Window)

	closeRequestCallback := func(gtk.Window) bool {
		descriptionWindow.Close()
		descriptionWindow.SetVisible(false)

		return true
	}
	descriptionWindow.ConnectCloseRequest(&closeRequestCallback)

	keyReleasedCallback := func(ctrl gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) {
		if keycode == keycodeEscape {
			descriptionWindow.Close()
			descriptionWindow.SetVisible(false)
		}
	}
	ctrl.ConnectKeyReleased(&keyReleasedCallback)

	toggledCallback := func(gtk.CheckButton) {
		if rightsConfirmationButton.GetActive() {
			downloadAndPlayButton.AddCssClass("suggested-action")
			downloadAndPlayButton.SetSensitive(true)

			return
		}

		downloadAndPlayButton.RemoveCssClass("suggested-action")
		downloadAndPlayButton.SetSensitive(false)
	}
	rightsConfirmationButton.ConnectToggled(&toggledCallback)

	refreshSubtitles := func() {
		subtitles = []mediaWithPriorityAndID{}
		for _, media := range torrentMedia {
			if media.name != selectedTorrentMedia {
				if strings.HasSuffix(media.name, ".srt") || strings.HasSuffix(media.name, ".vtt") || strings.HasSuffix(media.name, ".ass") {
					subtitles = append(subtitles, mediaWithPriorityAndID{
						media:    media,
						priority: 1,
					})
				} else {
					subtitles = append(subtitles, mediaWithPriorityAndID{
						media:    media,
						priority: 2,
					})
				}
			}
		}
	}

	clickedCallback1 := func(adw.SplitButton) {
		window.Close()
		refreshSubtitles()

		streamURL, err := getStreamURL(apiAddr, magnetLink, selectedTorrentMedia)
		if err != nil {
			OpenErrorDialog(ctx, &window, err)

			return
		}

		selectedTorrent, err := torrent.TorrentSpecFromMagnetUri(magnetLink)
		if err != nil {
			OpenErrorDialog(ctx, &window, err)

			return
		}

		dstFile := filepath.Join(settings.GetString(resources.GSchemaStorageKey), "Manual Downloads", selectedTorrent.InfoHash.HexString(), selectedTorrentMedia)

		if err := os.MkdirAll(filepath.Dir(dstFile), os.ModePerm); err != nil {
			OpenErrorDialog(ctx, &window, err)

			return
		}

		ctxDownload, cancel := context.WithCancel(context.Background())
		ready := make(chan struct{})
		if err := OpenControlsWindow(ctx, app, torrentTitle, subtitles, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, magnetLink, dstFile, settings, gateway, cancel, tmpDir, ready, cancel, adapter, ids, adapterCtx, cancelAdapterCtx, community, password, key, bufferedMessages, bufferedPeer, bufferedDecoder); err != nil {
			OpenErrorDialog(ctx, &window, err)

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

				OpenErrorDialog(ctx, &window, err)

				return
			}
			req.SetBasicAuth(apiUsername, apiPassword)

			res, err := hc.Do(req.WithContext(ctxDownload))
			if err != nil {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, &window, err)

				return
			}
			if res.Body != nil {
				defer res.Body.Close()
			}
			if res.StatusCode != http.StatusOK {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, &window, errors.New(res.Status))

				return
			}

			f, err := os.Create(dstFile)
			if err != nil {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, &window, err)

				return
			}
			defer f.Close()

			if _, err := io.Copy(f, res.Body); err != nil {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, &window, err)

				return
			}

			close(ready)
		}()
	}
	downloadAndPlayButton.ConnectClicked(&clickedCallback1)

	clickedCallback2 := func(gtk.Button) {
		streamPopover.SetVisible(false)

		window.Close()
		refreshSubtitles()

		streamURL, err := getStreamURL(apiAddr, magnetLink, selectedTorrentMedia)
		if err != nil {
			OpenErrorDialog(ctx, &window, err)

			return
		}

		ready := make(chan struct{})
		if err := OpenControlsWindow(ctx, app, torrentTitle, subtitles, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, magnetLink, streamURL, settings, gateway, cancel, tmpDir, ready, func() {}, adapter, ids, adapterCtx, cancelAdapterCtx, community, password, key, bufferedMessages, bufferedPeer, bufferedDecoder); err != nil {
			OpenErrorDialog(ctx, &window, err)

			return
		}

		close(ready)
	}
	streamWithoutDownloadingButton.ConnectClicked(&clickedCallback2)

	if runtime.GOOS == "linux" {
		warningDialog.SetResponseEnabled(responseDownloadFlathub, true)
		warningDialog.SetDefaultResponse(responseDownloadFlathub)
	}

	responseCallback := func(dialog adw.AlertDialog, response string) {
		switch response {
		case responseDownloadFlathub:
			_ = openuri.OpenURI("", mpvFlathubURL, nil)

			warningDialog.Close()

			os.Exit(0)

		case responseDownloadWebsite:
			_ = openuri.OpenURI("", mpvWebsiteURL, nil)

			warningDialog.Close()

			os.Exit(0)

		default:
			warningDialog.Close()

			preferencesDialog.SetTransientFor(&window.Window)
			preferencesDialog.Present()
			mpvCommandInput.GrabFocus()
		}
	}
	warningDialog.ConnectResponse(&responseCallback)

	app.AddWindow(&window.Window)

	showCallback := func(gtk.Widget) {
		if oldMPVCommand := settings.GetString(resources.GSchemaMPVKey); strings.TrimSpace(oldMPVCommand) == "" {
			newMPVCommand, err := mpvClient.DiscoverMPVExecutable()
			if err != nil {
				warningDialog.Present(&window.Window.Widget)

				return
			}

			settings.SetString(resources.GSchemaMPVKey, newMPVCommand)
			settings.Apply()
		}

		magnetLinkEntry.GrabFocus()
	}
	window.ConnectShow(&showCallback)

	window.SetVisible(true)

	return nil
}
