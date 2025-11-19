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
	"unsafe"

	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"github.com/anacrolix/torrent"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/assets/resources"
	api "github.com/pojntfx/multiplex/pkg/api/webrtc/v1"
	mpvClient "github.com/pojntfx/multiplex/pkg/client"
	"github.com/pojntfx/weron/pkg/wrtcconn"
	"github.com/rs/zerolog/log"
	"github.com/rymdport/portal/openuri"
)

var (
	gTypeMainWindow gobject.Type
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

type MainWindow struct {
	adw.ApplicationWindow

	overlay                        *adw.ToastOverlay
	buttonHeaderbarTitle           *gtk.Label
	buttonHeaderbarSubtitle        *gtk.Label
	previousButton                 *gtk.Button
	nextButton                     *gtk.Button
	menuButton                     *gtk.MenuButton
	headerbarSpinner               *gtk.Spinner
	stack                          *gtk.Stack
	magnetLinkEntry                *gtk.Entry
	mediaSelectionGroup            *adw.PreferencesGroup
	rightsConfirmationButton       *gtk.CheckButton
	downloadAndPlayButton          *adw.SplitButton
	streamWithoutDownloadingButton *gtk.Button
	streamPopover                  *gtk.Popover
	mediaInfoDisplay               *gtk.Box
	mediaInfoButton                *gtk.Button

	ctx         context.Context
	app         *adw.Application
	manager     *client.Manager
	apiAddr     string
	apiUsername string
	apiPassword string
	settings    *gio.Settings
	gateway     *server.Gateway
	cancel      func()
	tmpDir      string

	magnetLink           string
	torrentTitle         string
	torrentMedia         []media
	torrentReadme        string
	isNewSession         bool
	selectedTorrentMedia string
	activators           []*gtk.CheckButton
	mediaRows            []*adw.ActionRow
	subtitles            []mediaWithPriorityAndID
	community            string
	password             string
	key                  string
	bufferedMessages     []interface{}
	bufferedPeer         *wrtcconn.Peer
	bufferedDecoder      *json.Decoder
	adapter              *wrtcconn.Adapter
	ids                  chan string
	adapterCtx           context.Context
	cancelAdapterCtx     func()

	descriptionWindow DescriptionWindow
	warningDialog     WarningDialog
	preferencesDialog *adw.PreferencesWindow
	mpvCommandInput   *adw.EntryRow
}

func NewMainWindow(
	ctx context.Context,
	app *adw.Application,
	manager *client.Manager,
	apiAddr, apiUsername, apiPassword string,
	settings *gio.Settings,
	gateway *server.Gateway,
	cancel func(),
	tmpDir string,
) *MainWindow {
	var a gtk.Application
	app.Cast(&a)

	obj := gobject.NewObject(gTypeMainWindow, "application", a)

	v := (*MainWindow)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))

	v.ctx = ctx
	v.app = app
	v.manager = manager
	v.apiAddr = apiAddr
	v.apiUsername = apiUsername
	v.apiPassword = apiPassword
	v.settings = settings
	v.gateway = gateway
	v.cancel = cancel
	v.tmpDir = tmpDir

	v.descriptionWindow = NewDescriptionWindow(&v.ApplicationWindow)
	v.warningDialog = NewWarningDialog()
	v.warningDialog.SetResponseCallback(v.onWarningDialogResponse)

	v.preferencesDialog, v.mpvCommandInput = AddMainMenu(
		ctx,
		app,
		&v.ApplicationWindow,
		settings,
		v.menuButton,
		v.overlay,
		gateway,
		nil,
		cancel,
	)

	v.stack.SetVisibleChildName(welcomePageName)
	v.app.GetStyleManager().SetColorScheme(adw.ColorSchemeDefaultValue)

	return v
}

func (w *MainWindow) setupSignalHandlers() {
	onMagnetLinkEntryChanged := func() {
		if w.magnetLinkEntry.GetTextLength() > 0 {
			w.nextButton.SetSensitive(true)
		} else {
			w.nextButton.SetSensitive(false)
		}
	}
	w.magnetLinkEntry.ConnectSignal("changed", &onMagnetLinkEntryChanged)

	onMagnetLinkEntrySubmit := func(gtk.Entry) {
		w.onNext()
	}
	w.magnetLinkEntry.ConnectActivate(&onMagnetLinkEntrySubmit)

	onNextButton := func(gtk.Button) {
		w.onNext()
	}
	w.nextButton.ConnectClicked(&onNextButton)

	onPrevious := w.onPrevious
	w.previousButton.ConnectClicked(&onPrevious)

	onMediaInfo := func(gtk.Button) {
		w.descriptionWindow.SetVisible(true)
	}
	w.mediaInfoButton.ConnectClicked(&onMediaInfo)

	onRightsConfirmation := func(gtk.CheckButton) {
		if w.rightsConfirmationButton.GetActive() {
			w.downloadAndPlayButton.AddCssClass("suggested-action")
			w.downloadAndPlayButton.SetSensitive(true)
		} else {
			w.downloadAndPlayButton.RemoveCssClass("suggested-action")
			w.downloadAndPlayButton.SetSensitive(false)
		}
	}
	w.rightsConfirmationButton.ConnectToggled(&onRightsConfirmation)

	onDownloadAndPlay := w.onDownloadAndPlay
	w.downloadAndPlayButton.ConnectClicked(&onDownloadAndPlay)

	onStreamWithoutDownloading := w.onStreamWithoutDownloading
	w.streamWithoutDownloadingButton.ConnectClicked(&onStreamWithoutDownloading)

	onShow := w.onShow
	w.ApplicationWindow.ConnectShow(&onShow)
}


func (w *MainWindow) onNext() {
	switch w.stack.GetVisibleChildName() {
	case welcomePageName:
		go func() {
			magnetLinkOrStreamCode := w.magnetLinkEntry.GetText()
			u, err := url.Parse(magnetLinkOrStreamCode)
			if err == nil && u != nil && u.Scheme == "magnet" {
				w.isNewSession = true

				if w.selectedTorrentMedia == "" {
					w.nextButton.SetSensitive(false)
				}

				w.headerbarSpinner.SetSpinning(true)
				w.magnetLinkEntry.SetSensitive(false)

				log.Info().
					Str("magnetLink", magnetLinkOrStreamCode).
					Msg("Getting info for magnet link")

				info, err := w.manager.GetInfo(magnetLinkOrStreamCode)
				if err != nil {
					log.Warn().
						Str("magnetLink", magnetLinkOrStreamCode).
						Err(err).
						Msg("Could not get info for magnet link")

					toast := adw.NewToast(L("Could not get info for this magnet link."))

					w.overlay.AddToast(toast)

					w.headerbarSpinner.SetSpinning(false)
					w.magnetLinkEntry.SetSensitive(true)

					w.magnetLinkEntry.GrabFocus()

					return
				}

				w.torrentTitle = info.Name
				w.torrentReadme = strings.Map(
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
				w.torrentMedia = append(knownMedia, extraFiles...)

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

				for _, row := range w.mediaRows {
					w.mediaSelectionGroup.Remove(&row.PreferencesRow.Widget)
				}
				w.mediaRows = []*adw.ActionRow{}

				w.activators = []*gtk.CheckButton{}
				for _, file := range append(knownMediaWithPriority, extraFilesWithPriority...) {
					row := adw.NewActionRow()

					activator := gtk.NewCheckButton()

					if len(w.activators) > 0 {
						activator.SetGroup(w.activators[len(w.activators)-1])
					}
					w.activators = append(w.activators, activator)

					m := file.name
					activator.SetActive(false)
					activateCallback := func(gtk.CheckButton) {
						if m != w.selectedTorrentMedia {
							w.selectedTorrentMedia = m

							w.rightsConfirmationButton.SetActive(false)
						}

						w.nextButton.SetSensitive(true)
					}
					activator.ConnectActivate(&activateCallback)

					row.SetTitle(getDisplayPathWithoutRoot(file.name))
					if file.priority == 0 {
						row.SetSubtitle(fmt.Sprintf(L("Media (%v MB)"), file.size/1000/1000))
					} else {
						row.SetSubtitle(fmt.Sprintf(L("Extra file (%v MB)"), file.size/1000/1000))
					}
					row.SetActivatable(true)

					row.AddPrefix(&activator.Widget)
					row.SetActivatableWidget(&activator.Widget)

					w.mediaRows = append(w.mediaRows, row)
					w.mediaSelectionGroup.Add(&row.PreferencesRow.Widget)
				}

				w.headerbarSpinner.SetSpinning(false)
				w.magnetLinkEntry.SetSensitive(true)
				w.previousButton.SetVisible(true)

				w.buttonHeaderbarTitle.SetLabel(w.torrentTitle)
				w.descriptionWindow.HeaderbarTitle().SetLabel(w.torrentTitle)

				w.mediaInfoDisplay.SetVisible(false)
				w.mediaInfoButton.SetVisible(true)

				w.descriptionWindow.Text().SetWrapMode(gtk.WrapWordValue)
				if !utf8.Valid([]byte(w.torrentReadme)) || strings.TrimSpace(w.torrentReadme) == "" {
					w.descriptionWindow.Text().GetBuffer().SetText(L(readmePlaceholder), -1)
				} else {
					w.descriptionWindow.Text().GetBuffer().SetText(w.torrentReadme, -1)
				}

				w.stack.SetVisibleChildName(mediaPageName)

				w.magnetLink = w.magnetLinkEntry.GetText()

				return
			}

			go func() {
				log.Info().
					Str("streamCode", magnetLinkOrStreamCode).
					Msg("Joining session for stream code")

				w.isNewSession = false

				streamCodeParts := strings.Split(magnetLinkOrStreamCode, ":")
				if len(streamCodeParts) < 3 {
					toast := adw.NewToast(L("This stream code is invalid."))

					w.overlay.AddToast(toast)

					return
				}
				w.community, w.password, w.key = streamCodeParts[0], streamCodeParts[1], streamCodeParts[2]

				wu, err := url.Parse(w.settings.GetString(resources.SchemaWeronURLKey))
				if err != nil {
					OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

					return
				}

				w.headerbarSpinner.SetSpinning(true)
				w.magnetLinkEntry.SetSensitive(false)

				q := wu.Query()
				q.Set("community", streamCodeParts[0])
				q.Set("password", streamCodeParts[1])
				wu.RawQuery = q.Encode()

				w.adapterCtx, w.cancelAdapterCtx = context.WithCancel(context.Background())

				w.adapter = wrtcconn.NewAdapter(
					wu.String(),
					streamCodeParts[2],
					strings.Split(w.settings.GetString(resources.SchemaWeronICEKey), ","),
					[]string{"multiplex/sync"},
					&wrtcconn.AdapterConfig{
						Timeout:    time.Duration(time.Second * time.Duration(w.settings.GetInt64(resources.SchemaWeronTimeoutKey))),
						ForceRelay: w.settings.GetBoolean(resources.SchemaWeronForceRelayKey),
						OnSignalerReconnect: func() {
							log.Info().
								Str("raddr", w.settings.GetString(resources.SchemaWeronURLKey)).
								Msg("Reconnecting to signaler")
						},
					},
					w.adapterCtx,
				)

				w.ids, err = w.adapter.Open()
				if err != nil {
					w.cancelAdapterCtx()

					OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

					return
				}

				var receivedMagnetLink api.Magnet
			l:
				for {
					select {
					case <-w.ctx.Done():
						if err := w.ctx.Err(); err != context.Canceled {
							OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

							w.adapter.Close()
							w.cancelAdapterCtx()

							return
						}

						w.adapter.Close()
						w.cancelAdapterCtx()

						return
					case rid := <-w.ids:
						log.Info().
							Str("raddr", w.settings.GetString(resources.SchemaWeronURLKey)).
							Str("id", rid).
							Msg("Reconnecting to signaler")
					case peer := <-w.adapter.Accept():
						log.Info().
							Str("peerID", peer.PeerID).
							Str("channel", peer.ChannelID).
							Msg("Connected to peer")

						w.bufferedPeer = peer
						w.bufferedDecoder = json.NewDecoder(peer.Conn)

						for {
							var j interface{}
							if err := w.bufferedDecoder.Decode(&j); err != nil {
								log.Debug().
									Err(err).
									Msg("Could not decode structure, skipping")

								w.adapter.Close()
								w.cancelAdapterCtx()

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
								w.bufferedMessages = append(w.bufferedMessages, j)
							}
						}
					}
				}

				w.magnetLink = receivedMagnetLink.Magnet
				w.torrentTitle = receivedMagnetLink.Title
				w.torrentReadme = receivedMagnetLink.Description
				w.selectedTorrentMedia = receivedMagnetLink.Path

				w.torrentMedia = []media{}
				for _, subtitle := range receivedMagnetLink.Subtitles {
					w.torrentMedia = append(w.torrentMedia, media{
						name: subtitle.Name,
						size: subtitle.Size,
					})
				}

				w.headerbarSpinner.SetSpinning(false)
				w.magnetLinkEntry.SetSensitive(true)
				w.previousButton.SetVisible(true)

				w.buttonHeaderbarTitle.SetLabel(w.torrentTitle)
				w.descriptionWindow.HeaderbarTitle().SetLabel(w.torrentTitle)

				w.mediaInfoDisplay.SetVisible(false)
				w.mediaInfoButton.SetVisible(true)

				w.descriptionWindow.Text().SetWrapMode(gtk.WrapWordValue)
				if !utf8.Valid([]byte(w.torrentReadme)) || strings.TrimSpace(w.torrentReadme) == "" {
					w.descriptionWindow.Text().GetBuffer().SetText(L("No README found."), -1)
				} else {
					w.descriptionWindow.Text().GetBuffer().SetText(w.torrentReadme, -1)
				}

				w.nextButton.SetVisible(false)

				w.buttonHeaderbarSubtitle.SetVisible(true)
				w.descriptionWindow.HeaderbarSubtitle().SetVisible(true)
				w.buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(w.selectedTorrentMedia))
				w.descriptionWindow.HeaderbarSubtitle().SetLabel(getDisplayPathWithoutRoot(w.selectedTorrentMedia))

				w.stack.SetVisibleChildName(readyPageName)
			}()
		}()
	case mediaPageName:
		w.nextButton.SetVisible(false)

		w.buttonHeaderbarSubtitle.SetVisible(true)
		w.descriptionWindow.HeaderbarSubtitle().SetVisible(true)
		w.buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(w.selectedTorrentMedia))
		w.descriptionWindow.HeaderbarSubtitle().SetLabel(getDisplayPathWithoutRoot(w.selectedTorrentMedia))

		w.stack.SetVisibleChildName(readyPageName)
	}
}

func (w *MainWindow) onPrevious(gtk.Button) {
	switch w.stack.GetVisibleChildName() {
	case mediaPageName:
		w.previousButton.SetVisible(false)
		w.nextButton.SetSensitive(true)

		w.mediaInfoDisplay.SetVisible(true)
		w.mediaInfoButton.SetVisible(false)

		w.stack.SetVisibleChildName(welcomePageName)
	case readyPageName:
		w.nextButton.SetVisible(true)

		w.buttonHeaderbarSubtitle.SetVisible(false)
		w.descriptionWindow.HeaderbarSubtitle().SetVisible(false)

		if !w.isNewSession {
			if w.adapter != nil {
				w.adapter.Close()
			}

			if w.cancelAdapterCtx != nil {
				w.cancelAdapterCtx()
			}

			w.adapter = nil
			w.ids = nil
			w.adapterCtx = nil
			w.cancelAdapterCtx = nil

			w.community = ""
			w.password = ""
			w.key = ""

			w.previousButton.SetVisible(false)
			w.nextButton.SetSensitive(true)

			w.mediaInfoDisplay.SetVisible(true)
			w.mediaInfoButton.SetVisible(false)

			w.stack.SetVisibleChildName(welcomePageName)

			return
		}

		w.stack.SetVisibleChildName(mediaPageName)
	}
}

func (w *MainWindow) onDownloadAndPlay(adw.SplitButton) {
	w.ApplicationWindow.Close()
	w.refreshSubtitles()

	streamURL, err := getStreamURL(w.apiAddr, w.magnetLink, w.selectedTorrentMedia)
	if err != nil {
		OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

		return
	}

	selectedTorrent, err := torrent.TorrentSpecFromMagnetUri(w.magnetLink)
	if err != nil {
		OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

		return
	}

	dstFile := filepath.Join(w.settings.GetString(resources.SchemaStorageKey), "Manual Downloads", selectedTorrent.InfoHash.HexString(), w.selectedTorrentMedia)

	if err := os.MkdirAll(filepath.Dir(dstFile), os.ModePerm); err != nil {
		OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

		return
	}

	ctxDownload, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	if err := OpenControlsWindow(w.ctx, w.app, w.torrentTitle, w.subtitles, w.selectedTorrentMedia, w.torrentReadme, w.manager, w.apiAddr, w.apiUsername, w.apiPassword, w.magnetLink, dstFile, w.settings, w.gateway, w.cancel, w.tmpDir, ready, cancel, w.adapter, w.ids, w.adapterCtx, w.cancelAdapterCtx, w.community, w.password, w.key, w.bufferedMessages, w.bufferedPeer, w.bufferedDecoder); err != nil {
		OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

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

			OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

			return
		}
		req.SetBasicAuth(w.apiUsername, w.apiPassword)

		res, err := hc.Do(req.WithContext(ctxDownload))
		if err != nil {
			if err == context.Canceled {
				return
			}

			OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

			return
		}
		if res.Body != nil {
			defer res.Body.Close()
		}
		if res.StatusCode != http.StatusOK {
			if err == context.Canceled {
				return
			}

			OpenErrorDialog(w.ctx, &w.ApplicationWindow, errors.New(res.Status))

			return
		}

		f, err := os.Create(dstFile)
		if err != nil {
			if err == context.Canceled {
				return
			}

			OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

			return
		}
		defer f.Close()

		if _, err := io.Copy(f, res.Body); err != nil {
			if err == context.Canceled {
				return
			}

			OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

			return
		}

		close(ready)
	}()
}

func (w *MainWindow) onStreamWithoutDownloading(gtk.Button) {
	w.streamPopover.SetVisible(false)

	w.ApplicationWindow.Close()
	w.refreshSubtitles()

	streamURL, err := getStreamURL(w.apiAddr, w.magnetLink, w.selectedTorrentMedia)
	if err != nil {
		OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

		return
	}

	ready := make(chan struct{})
	if err := OpenControlsWindow(w.ctx, w.app, w.torrentTitle, w.subtitles, w.selectedTorrentMedia, w.torrentReadme, w.manager, w.apiAddr, w.apiUsername, w.apiPassword, w.magnetLink, streamURL, w.settings, w.gateway, w.cancel, w.tmpDir, ready, func() {}, w.adapter, w.ids, w.adapterCtx, w.cancelAdapterCtx, w.community, w.password, w.key, w.bufferedMessages, w.bufferedPeer, w.bufferedDecoder); err != nil {
		OpenErrorDialog(w.ctx, &w.ApplicationWindow, err)

		return
	}

	close(ready)
}

func (w *MainWindow) onWarningDialogResponse(response string) {
	switch response {
	case responseDownloadFlathub:
		_ = openuri.OpenURI("", mpvFlathubURL, nil)

		w.warningDialog.Close()

		os.Exit(0)

	case responseDownloadWebsite:
		_ = openuri.OpenURI("", mpvWebsiteURL, nil)

		w.warningDialog.Close()

		os.Exit(0)

	default:
		w.warningDialog.Close()

		w.preferencesDialog.SetTransientFor(&w.ApplicationWindow.Window)
		w.preferencesDialog.Present()
		w.mpvCommandInput.GrabFocus()
	}
}

func (w *MainWindow) onShow(gtk.Widget) {
	if oldMPVCommand := w.settings.GetString(resources.SchemaMPVKey); strings.TrimSpace(oldMPVCommand) == "" {
		newMPVCommand, err := mpvClient.DiscoverMPVExecutable()
		if err != nil {
			if runtime.GOOS == "linux" {
				w.warningDialog.SetResponseEnabled(responseDownloadFlathub, true)
				w.warningDialog.SetDefaultResponse(responseDownloadFlathub)
			}

			w.warningDialog.Present(&w.ApplicationWindow.Window.Widget)

			return
		}

		w.settings.SetString(resources.SchemaMPVKey, newMPVCommand)
		w.settings.Apply()
	}

	w.magnetLinkEntry.GrabFocus()
}

// refreshSubtitles refreshes the subtitle list from the torrent media
func (w *MainWindow) refreshSubtitles() {
	w.subtitles = []mediaWithPriorityAndID{}
	for _, media := range w.torrentMedia {
		if media.name != w.selectedTorrentMedia {
			if strings.HasSuffix(media.name, ".srt") || strings.HasSuffix(media.name, ".vtt") || strings.HasSuffix(media.name, ".ass") {
				w.subtitles = append(w.subtitles, mediaWithPriorityAndID{
					media:    media,
					priority: 1,
				})
			} else {
				w.subtitles = append(w.subtitles, mediaWithPriorityAndID{
					media:    media,
					priority: 2,
				})
			}
		}
	}
}

func init() {
	var windowClassInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceAssistantPath)

		typeClass.BindTemplateChildFull("toast-overlay", false, 0)
		typeClass.BindTemplateChildFull("button-headerbar-title", false, 0)
		typeClass.BindTemplateChildFull("button-headerbar-subtitle", false, 0)
		typeClass.BindTemplateChildFull("previous-button", false, 0)
		typeClass.BindTemplateChildFull("next-button", false, 0)
		typeClass.BindTemplateChildFull("menu-button", false, 0)
		typeClass.BindTemplateChildFull("headerbar-spinner", false, 0)
		typeClass.BindTemplateChildFull("stack", false, 0)
		typeClass.BindTemplateChildFull("magnet-link-entry", false, 0)
		typeClass.BindTemplateChildFull("media-selection-group", false, 0)
		typeClass.BindTemplateChildFull("rights-confirmation-button", false, 0)
		typeClass.BindTemplateChildFull("download-and-play-button", false, 0)
		typeClass.BindTemplateChildFull("stream-without-downloading-button", false, 0)
		typeClass.BindTemplateChildFull("stream-popover", false, 0)
		typeClass.BindTemplateChildFull("media-info-display", false, 0)
		typeClass.BindTemplateChildFull("media-info-button", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.ApplicationWindow
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				overlay                        adw.ToastOverlay
				buttonHeaderbarTitle           gtk.Label
				buttonHeaderbarSubtitle        gtk.Label
				previousButton                 gtk.Button
				nextButton                     gtk.Button
				menuButton                     gtk.MenuButton
				headerbarSpinner               gtk.Spinner
				stack                          gtk.Stack
				magnetLinkEntry                gtk.Entry
				mediaSelectionGroup            adw.PreferencesGroup
				rightsConfirmationButton       gtk.CheckButton
				downloadAndPlayButton          adw.SplitButton
				streamWithoutDownloadingButton gtk.Button
				streamPopover                  gtk.Popover
				mediaInfoDisplay               gtk.Box
				mediaInfoButton                gtk.Button
			)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "toast-overlay").Cast(&overlay)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "button-headerbar-title").Cast(&buttonHeaderbarTitle)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "button-headerbar-subtitle").Cast(&buttonHeaderbarSubtitle)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "previous-button").Cast(&previousButton)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "next-button").Cast(&nextButton)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "menu-button").Cast(&menuButton)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "headerbar-spinner").Cast(&headerbarSpinner)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "stack").Cast(&stack)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "magnet-link-entry").Cast(&magnetLinkEntry)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "media-selection-group").Cast(&mediaSelectionGroup)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "rights-confirmation-button").Cast(&rightsConfirmationButton)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "download-and-play-button").Cast(&downloadAndPlayButton)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "stream-without-downloading-button").Cast(&streamWithoutDownloadingButton)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "stream-popover").Cast(&streamPopover)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "media-info-display").Cast(&mediaInfoDisplay)
			parent.Widget.GetTemplateChild(gTypeMainWindow, "media-info-button").Cast(&mediaInfoButton)

			w := &MainWindow{
				ApplicationWindow: parent,

				overlay:                        &overlay,
				buttonHeaderbarTitle:           &buttonHeaderbarTitle,
				buttonHeaderbarSubtitle:        &buttonHeaderbarSubtitle,
				previousButton:                 &previousButton,
				nextButton:                     &nextButton,
				menuButton:                     &menuButton,
				headerbarSpinner:               &headerbarSpinner,
				stack:                          &stack,
				magnetLinkEntry:                &magnetLinkEntry,
				mediaSelectionGroup:            &mediaSelectionGroup,
				rightsConfirmationButton:       &rightsConfirmationButton,
				downloadAndPlayButton:          &downloadAndPlayButton,
				streamWithoutDownloadingButton: &streamWithoutDownloadingButton,
				streamPopover:                  &streamPopover,
				mediaInfoDisplay:               &mediaInfoDisplay,
				mediaInfoButton:                &mediaInfoButton,
				isNewSession:                   true,
				activators:                     []*gtk.CheckButton{},
				mediaRows:                      []*adw.ActionRow{},
				subtitles:                      []mediaWithPriorityAndID{},
				bufferedMessages:               []interface{}{},
			}

			var pinner runtime.Pinner
			pinner.Pin(w)

			var cleanupCallback glib.DestroyNotify = func(data uintptr) {
				pinner.Unpin()
			}
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(w)), &cleanupCallback)

			w.setupSignalHandlers()
		})
	}

	var windowInstanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	var windowParentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.ApplicationWindowGLibType(), &windowParentQuery)

	gTypeMainWindow = gobject.TypeRegisterStaticSimple(
		windowParentQuery.Type,
		"MainWindow",
		windowParentQuery.ClassSize,
		&windowClassInit,
		windowParentQuery.InstanceSize+uint(unsafe.Sizeof(MainWindow{}))+uint(unsafe.Sizeof(&MainWindow{})),
		&windowInstanceInit,
		0,
	)
}
