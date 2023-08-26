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
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/internal/gschema"
	"github.com/pojntfx/multiplex/internal/ressources"
	api "github.com/pojntfx/multiplex/pkg/api/webrtc/v1"
	mpvClient "github.com/pojntfx/multiplex/pkg/client"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog/log"
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
	app.StyleManager().SetColorScheme(adw.ColorSchemeDefault)

	builder := gtk.NewBuilderFromString(ressources.AssistantUI, len(ressources.AssistantUI))

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

	descriptionBuilder := gtk.NewBuilderFromString(ressources.DescriptionUI, len(ressources.DescriptionUI))
	descriptionWindow := descriptionBuilder.GetObject("description-window").Cast().(*adw.Window)
	descriptionText := descriptionBuilder.GetObject("description-text").Cast().(*gtk.TextView)
	descriptionHeaderbarTitle := descriptionBuilder.GetObject("headerbar-title").Cast().(*gtk.Label)
	descriptionHeaderbarSubtitle := descriptionBuilder.GetObject("headerbar-subtitle").Cast().(*gtk.Label)

	warningBuilder := gtk.NewBuilderFromString(ressources.WarningUI, len(ressources.WarningUI))
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
						mediaSelectionGroup.Remove(row)
					}
					mediaRows = []*adw.ActionRow{}

					activators = []*gtk.CheckButton{}
					for i, file := range append(knownMediaWithPriority, extraFilesWithPriority...) {
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
						if file.priority == 0 {
							row.SetSubtitle(fmt.Sprintf("Media (%v MB)", file.size/1000/1000))
						} else {
							row.SetSubtitle(fmt.Sprintf("Extra file (%v MB)", file.size/1000/1000))
						}
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

					wu, err := url.Parse(settings.String(gschema.WeronURLFlag))
					if err != nil {
						OpenErrorDialog(ctx, window, err)

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
						strings.Split(settings.String(gschema.WeronICEFlag), ","),
						[]string{"multiplex/sync"},
						&wrtcconn.AdapterConfig{
							Timeout:    time.Duration(time.Second * time.Duration(settings.Int64(gschema.WeronTimeoutFlag))),
							ForceRelay: settings.Boolean(gschema.WeronForceRelayFlag),
							OnSignalerReconnect: func() {
								log.Info().
									Str("raddr", settings.String(gschema.WeronURLFlag)).
									Msg("Reconnecting to signaler")
							},
						},
						adapterCtx,
					)

					ids, err = adapter.Open()
					if err != nil {
						cancelAdapterCtx()

						OpenErrorDialog(ctx, window, err)

						return
					}

					var receivedMagnetLink api.Magnet
				l:
					for {
						select {
						case <-ctx.Done():
							if err := ctx.Err(); err != context.Canceled {
								OpenErrorDialog(ctx, window, err)

								adapter.Close()
								cancelAdapterCtx()

								return
							}

							adapter.Close()
							cancelAdapterCtx()

							return
						case rid := <-ids:
							log.Info().
								Str("raddr", settings.String(gschema.WeronURLFlag)).
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

	preferencesWindow, mpvCommandInput := AddMainMenu(ctx, app, window, settings, menuButton, overlay, gateway, nil, cancel)

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

	downloadAndPlayButton.ConnectClicked(func() {
		window.Close()
		refreshSubtitles()

		streamURL, err := getStreamURL(apiAddr, magnetLink, selectedTorrentMedia)
		if err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}

		selectedTorrent, err := torrent.TorrentSpecFromMagnetUri(magnetLink)
		if err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}

		dstFile := filepath.Join(settings.String(gschema.StorageFlag), "Manual Downloads", selectedTorrent.InfoHash.HexString(), selectedTorrentMedia)

		if err := os.MkdirAll(filepath.Dir(dstFile), os.ModePerm); err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}

		ctxDownload, cancel := context.WithCancel(context.Background())
		ready := make(chan struct{})
		if err := OpenControlsWindow(ctx, app, torrentTitle, subtitles, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, magnetLink, dstFile, settings, gateway, cancel, tmpDir, ready, cancel, adapter, ids, adapterCtx, cancelAdapterCtx, community, password, key, bufferedMessages, bufferedPeer, bufferedDecoder); err != nil {
			OpenErrorDialog(ctx, window, err)

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

				OpenErrorDialog(ctx, window, err)

				return
			}
			req.SetBasicAuth(apiUsername, apiPassword)

			res, err := hc.Do(req.WithContext(ctxDownload))
			if err != nil {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, window, err)

				return
			}
			if res.Body != nil {
				defer res.Body.Close()
			}
			if res.StatusCode != http.StatusOK {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, window, errors.New(res.Status))

				return
			}

			f, err := os.Create(dstFile)
			if err != nil {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, window, err)

				return
			}
			defer f.Close()

			if _, err := io.Copy(f, res.Body); err != nil {
				if err == context.Canceled {
					return
				}

				OpenErrorDialog(ctx, window, err)

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
			OpenErrorDialog(ctx, window, err)

			return
		}

		ready := make(chan struct{})
		if err := OpenControlsWindow(ctx, app, torrentTitle, subtitles, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, magnetLink, streamURL, settings, gateway, cancel, tmpDir, ready, func() {}, adapter, ids, adapterCtx, cancelAdapterCtx, community, password, key, bufferedMessages, bufferedPeer, bufferedDecoder); err != nil {
			OpenErrorDialog(ctx, window, err)

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
		if oldMPVCommand := settings.String(gschema.MPVFlag); strings.TrimSpace(oldMPVCommand) == "" {
			newMPVCommand, err := mpvClient.DiscoverMPVExecutable()
			if err != nil {
				warningDialog.Show()

				return
			}

			settings.SetString(gschema.MPVFlag, newMPVCommand)
			settings.Apply()
		}

		magnetLinkEntry.GrabFocus()
	})

	window.Show()

	return nil
}
