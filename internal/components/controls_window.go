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
	"unsafe"

	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"github.com/anacrolix/torrent"
	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/assets/resources"
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
	playIcon  = "media-playback-start-symbolic"
	pauseIcon = "media-playback-pause-symbolic"

	keycodeEscape = 66
)

var (
	readmePlaceholder   = "No README found."
	errKilled           = errors.New("signal: killed")
	gTypeControlsWindow gobject.Type
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

type ControlsWindow struct {
	adw.ApplicationWindow

	overlay                 *adw.ToastOverlay
	buttonHeaderbarTitle    *gtk.Label
	buttonHeaderbarSubtitle *gtk.Label
	playButton              *gtk.Button
	stopButton              *gtk.Button
	volumeScale             *gtk.Scale
	volumeButton            *gtk.MenuButton
	volumeMuteButton        *gtk.Button
	subtitleButton          *gtk.Button
	audiotracksButton       *gtk.Button
	fullscreenButton        *gtk.ToggleButton
	mediaInfoButton         *gtk.Button
	headerbarSpinner        *adw.Spinner
	menuButton              *gtk.MenuButton
	elapsedTrackLabel       *gtk.Label
	remainingTrackLabel     *gtk.Label
	seeker                  *gtk.Scale
	watchingWithTitleLabel  *gtk.Label
	streamCodeInput         *gtk.Entry
	copyStreamCodeButton    *gtk.Button

	ctx                  context.Context
	app                  *adw.Application
	manager              *client.Manager
	apiAddr              string
	apiUsername          string
	apiPassword          string
	magnetLink           string
	streamURL            string
	settings             *gio.Settings
	gateway              *server.Gateway
	cancel               func()
	tmpDir               string
	torrentTitle         string
	subtitles            []mediaWithPriorityAndID
	selectedTorrentMedia string
	torrentReadme        string
	ready                chan struct{}
	cancelDownload       func()
	adapter              *wrtcconn.Adapter
	ids                  chan string
	adapterCtx           context.Context
	cancelAdapterCtx     func()
	community            string
	password             string
	key                  string
	bufferedMessages     []interface{}
	bufferedPeer         *wrtcconn.Peer
	bufferedDecoder      *json.Decoder
	command              *exec.Cmd
	ipcFile              string
	ipcDir               string
}

func NewControlsWindow(
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
) (ControlsWindow, error) {
	obj := gobject.NewObject(gTypeControlsWindow, "application", app)

	var v ControlsWindow
	obj.Cast(&v)

	controlsW := (*ControlsWindow)(unsafe.Pointer(v.Widget.GetData(dataKeyGoInstance)))
	controlsW.ctx = ctx
	controlsW.app = app
	controlsW.manager = manager
	controlsW.apiAddr = apiAddr
	controlsW.apiUsername = apiUsername
	controlsW.apiPassword = apiPassword
	controlsW.magnetLink = magnetLink
	controlsW.streamURL = streamURL
	controlsW.settings = settings
	controlsW.gateway = gateway
	controlsW.cancel = cancel
	controlsW.tmpDir = tmpDir
	controlsW.torrentTitle = torrentTitle
	controlsW.subtitles = subtitles
	controlsW.selectedTorrentMedia = selectedTorrentMedia
	controlsW.torrentReadme = torrentReadme
	controlsW.ready = ready
	controlsW.cancelDownload = cancelDownload
	controlsW.adapter = adapter
	controlsW.ids = ids
	controlsW.adapterCtx = adapterCtx
	controlsW.cancelAdapterCtx = cancelAdapterCtx
	controlsW.community = community
	controlsW.password = password
	controlsW.key = key
	controlsW.bufferedMessages = bufferedMessages
	controlsW.bufferedPeer = bufferedPeer
	controlsW.bufferedDecoder = bufferedDecoder

	if err := controlsW.setup(); err != nil {
		return v, err
	}

	return v, nil
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

func (c *ControlsWindow) setup() error {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	controlsW.app.GetStyleManager().SetColorScheme(adw.ColorSchemePreferDarkValue)

	descriptionWindow := NewDescriptionWindow(&controlsW.ApplicationWindow)
	subtitlesDialog := NewSubtitlesDialog(&controlsW.ApplicationWindow)
	audiotracksDialog := NewAudioTracksDialog(&controlsW.ApplicationWindow)
	preparingWindow := NewPreparingWindow(&controlsW.ApplicationWindow)

	controlsW.buttonHeaderbarTitle.SetLabel(controlsW.torrentTitle)
	descriptionWindow.HeaderbarTitle().SetLabel(controlsW.torrentTitle)
	controlsW.buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(controlsW.selectedTorrentMedia))
	descriptionWindow.HeaderbarSubtitle().SetVisible(true)
	descriptionWindow.HeaderbarSubtitle().SetLabel(getDisplayPathWithoutRoot(controlsW.selectedTorrentMedia))

	descriptionWindow.PreparingProgressBar().SetVisible(true)

	if controlsW.community == "" || controlsW.password == "" || controlsW.key == "" {
		sid, err := shortid.New(1, shortid.DefaultABC, uint64(time.Now().UnixNano()))
		if err != nil {
			return err
		}

		controlsW.community, err = sid.Generate()
		if err != nil {
			return err
		}

		controlsW.password, err = sid.Generate()
		if err != nil {
			return err
		}

		controlsW.key, err = sid.Generate()
		if err != nil {
			return err
		}
	}

	if controlsW.adapter == nil {
		controlsW.adapterCtx, controlsW.cancelAdapterCtx = context.WithCancel(context.Background())
	}

	u, err := url.Parse(controlsW.settings.GetString(resources.SchemaWeronURLKey))
	if err != nil {
		controlsW.cancelAdapterCtx()
		return err
	}

	q := u.Query()
	q.Set("community", controlsW.community)
	q.Set("password", controlsW.password)
	u.RawQuery = q.Encode()

	pauses := broadcast.NewRelay[bool]()
	positions := broadcast.NewRelay[float64]()
	buffering := broadcast.NewRelay[bool]()

	if controlsW.adapter == nil {
		controlsW.adapter = wrtcconn.NewAdapter(
			u.String(),
			controlsW.key,
			strings.Split(controlsW.settings.GetString(resources.SchemaWeronICEKey), ","),
			[]string{"multiplex/sync"},
			&wrtcconn.AdapterConfig{
				Timeout:    time.Duration(time.Second * time.Duration(controlsW.settings.GetInt64(resources.SchemaWeronTimeoutKey))),
				ForceRelay: controlsW.settings.GetBoolean(resources.SchemaWeronForceRelayKey),
				OnSignalerReconnect: func() {
					log.Info().
						Str("raddr", controlsW.settings.GetString(resources.SchemaWeronURLKey)).
						Msg("Reconnecting to signaler")
				},
			},
			controlsW.adapterCtx,
		)

		controlsW.ids, err = controlsW.adapter.Open()
		if err != nil {
			controlsW.cancelAdapterCtx()
			return err
		}
	}

	controlsW.streamCodeInput.SetText(fmt.Sprintf("%v:%v:%v", controlsW.community, controlsW.password, controlsW.key))

	connectedPeers := new(int32)
	syncWatchingWithLabel := func(connected bool) {
		if connected {
			atomic.AddInt32(connectedPeers, 1)

			toast := adw.NewToast(L("Someone joined the session."))
			controlsW.overlay.AddToast(toast)
		} else {
			atomic.AddInt32(connectedPeers, -1)

			toast := adw.NewToast(L("Someone left the session."))
			controlsW.overlay.AddToast(toast)
		}

		if *connectedPeers <= 0 {
			controlsW.watchingWithTitleLabel.SetText(L("You're currently watching alone."))
			return
		}

		if *connectedPeers == 1 {
			controlsW.watchingWithTitleLabel.SetText(fmt.Sprintf(L("You're currently watching with %v other person."), *connectedPeers))
			return
		}

		controlsW.watchingWithTitleLabel.SetText(fmt.Sprintf(L("You're currently watching with %v other people."), *connectedPeers))
	}

	onCopyStreamCode := func(gtk.Button) {
		controlsW.ApplicationWindow.GetClipboard().SetText(controlsW.streamCodeInput.GetText())
	}
	controlsW.copyStreamCodeButton.ConnectClicked(&onCopyStreamCode)

	onStopButton := func(gtk.Button) {
		controlsW.ApplicationWindow.Close()

		mainWindow := NewMainWindow(controlsW.ctx, controlsW.app, controlsW.manager, controlsW.apiAddr, controlsW.apiUsername, controlsW.apiPassword, controlsW.settings, controlsW.gateway, controlsW.cancel, controlsW.tmpDir)

		controlsW.app.AddWindow(&mainWindow.ApplicationWindow.Window)
		mainWindow.SetVisible(true)
	}
	controlsW.stopButton.ConnectClicked(&onStopButton)

	onMediaInfoButton := func(gtk.Button) {
		descriptionWindow.SetVisible(true)
	}
	controlsW.mediaInfoButton.ConnectClicked(&onMediaInfoButton)

	ctrl := gtk.NewEventControllerKey()
	descriptionWindow.AddController(&ctrl.EventController)
	descriptionWindow.SetTransientFor(&controlsW.ApplicationWindow.Window)

	onDescCloseRequest := func(gtk.Window) bool {
		descriptionWindow.Close()
		descriptionWindow.SetVisible(false)
		return true
	}
	descriptionWindow.ConnectCloseRequest(&onDescCloseRequest)

	onDescKeyReleased := func(ctrl gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) {
		if keycode == keycodeEscape {
			descriptionWindow.Close()
			descriptionWindow.SetVisible(false)
		}
	}
	ctrl.ConnectKeyReleased(&onDescKeyReleased)

	descriptionWindow.Text().SetWrapMode(gtk.WrapWordValue)
	if !utf8.Valid([]byte(controlsW.torrentReadme)) || strings.TrimSpace(controlsW.torrentReadme) == "" {
		descriptionWindow.Text().GetBuffer().SetText(L(readmePlaceholder), -1)
	} else {
		descriptionWindow.Text().GetBuffer().SetText(controlsW.torrentReadme, -1)
	}

	preparingWindow.SetTransientFor(&controlsW.ApplicationWindow.Window)

	descriptionProgressBar := descriptionWindow.PreparingProgressBar()
	progressBarTicker := time.NewTicker(time.Millisecond * 500)
	go func() {
		for range progressBarTicker.C {
			metrics, err := controlsW.manager.GetMetrics()
			if err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}

			length := float64(0)
			completed := float64(0)
			peers := 0

		l:
			for _, t := range metrics {
				selectedTorrent, err := torrent.TorrentSpecFromMagnetUri(controlsW.magnetLink)
				if err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				if selectedTorrent.InfoHash.HexString() == t.InfoHash {
					peers = t.Peers

					for _, f := range t.Files {
						if f.Path == controlsW.selectedTorrentMedia {
							length = float64(f.Length)
							completed = float64(f.Completed)
							break l
						}
					}
				}
			}

		n:
			for _, progressBar := range []*gtk.ProgressBar{preparingWindow.ProgressBar(), descriptionProgressBar} {
				if length > 0 {
					progressBar.SetFraction(completed / length)
					progressBar.SetText(fmt.Sprintf(L("%v MB/%v MB (%v peers)"), int(completed/1000/1000), int(length/1000/1000), peers))
					continue n
				}

				progressBar.SetText(L("Searching for peers"))
			}
		}
	}()

	onPrepCloseRequest := func(gtk.Window) bool {
		preparingWindow.Close()
		preparingWindow.SetVisible(false)

		return true
	}
	preparingWindow.SetCloseRequestCallback(func() bool {
		return onPrepCloseRequest(gtk.Window{})
	})

	onPrepCancel := func(gtk.Button) {
		controlsW.adapter.Close()
		controlsW.cancelAdapterCtx()

		pauses.Close()
		positions.Close()
		buffering.Close()

		progressBarTicker.Stop()

		controlsW.cancelDownload()

		controlsW.ApplicationWindow.Destroy()

		preparingWindow.Close()

		mainWindow := NewMainWindow(controlsW.ctx, controlsW.app, controlsW.manager, controlsW.apiAddr, controlsW.apiUsername, controlsW.apiPassword, controlsW.settings, controlsW.gateway, controlsW.cancel, controlsW.tmpDir)

		controlsW.app.AddWindow(&mainWindow.ApplicationWindow.Window)
		mainWindow.SetVisible(true)
	}
	preparingWindow.SetCancelCallback(func() {
		onPrepCancel(gtk.Button{})
	})

	usernameAndPassword := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v:%v", controlsW.apiUsername, controlsW.apiPassword)))

	controlsW.ipcDir, err = os.MkdirTemp(os.TempDir(), "mpv-ipc")
	if err != nil {
		return err
	}

	controlsW.ipcFile = filepath.Join(controlsW.ipcDir, "mpv.sock")

	shell := []string{"sh", "-c"}
	if runtime.GOOS == "windows" {
		shell = []string{"cmd.exe", "/c", "start"}
	}
	commandLine := append(shell, fmt.Sprintf("%v '--no-sub-visibility' '--keep-open=always' '--no-osc' '--no-input-default-bindings' '--pause' '--input-ipc-server=%v' '--http-header-fields=Authorization: Basic %v' '%v'", controlsW.settings.GetString(resources.SchemaMPVKey), controlsW.ipcFile, usernameAndPassword, controlsW.streamURL))

	controlsW.command = exec.Command(
		commandLine[0],
		commandLine[1:]...,
	)
	utils.AddSysProcAttr(controlsW.command)

	controlsW.command.Stdin = os.Stdin
	controlsW.command.Stdout = os.Stdout
	controlsW.command.Stderr = os.Stderr

	AddMainMenu(
		controlsW.ctx,
		controlsW.app,
		&controlsW.ApplicationWindow,
		controlsW.settings,
		controlsW.menuButton,
		controlsW.overlay,
		controlsW.gateway,
		func() string {
			return controlsW.magnetLink
		},
		func() {
			controlsW.cancel()

			if controlsW.command.Process != nil {
				if err := controlsW.command.Process.Kill(); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}
			}
		},
	)

	controlsW.app.AddWindow(&controlsW.ApplicationWindow.Window)

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		log.Debug().Msg("Gracefully shutting down")

		controlsW.ApplicationWindow.Close()
	}()

	onShow := func(gtk.Widget) {
		preparingWindow.SetVisible(true)

		go func() {
			<-controlsW.ready

			if err := controlsW.command.Start(); err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}
		}()

		onCloseRequest := func(gtk.Window) bool {
			controlsW.adapter.Close()
			controlsW.cancelAdapterCtx()

			pauses.Close()
			positions.Close()
			buffering.Close()

			progressBarTicker.Stop()

			if controlsW.command.Process != nil {
				if err := utils.Kill(controlsW.command.Process); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return false
				}
			}

			if err := os.RemoveAll(controlsW.ipcDir); err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return false
			}

			return true
		}
		controlsW.ApplicationWindow.ConnectCloseRequest(&onCloseRequest)

		go func() {
			<-controlsW.ready

			for {
				sock, err := net.Dial("unix", controlsW.ipcFile)
				if err == nil {
					_ = sock.Close()
					break
				}

				time.Sleep(time.Millisecond * 100)

				log.Error().
					Str("path", controlsW.ipcFile).
					Err(err).
					Msg("Could not dial IPC socket, retrying in 100ms")
			}

			controlsW.setupPlaybackControls(
				pauses,
				positions,
				buffering,
				syncWatchingWithLabel,
				subtitlesDialog,
				audiotracksDialog,
				preparingWindow,
			)
		}()
	}
	controlsW.ApplicationWindow.ConnectShow(&onShow)

	controlsW.ApplicationWindow.SetVisible(true)

	return nil
}

func (c *ControlsWindow) setupPlaybackControls(
	pauses *broadcast.Relay[bool],
	positions *broadcast.Relay[float64],
	buffering *broadcast.Relay[bool],
	syncWatchingWithLabel func(bool),
	subtitlesDialog SubtitlesDialog,
	audiotracksDialog AudioTracksDialog,
	preparingWindow PreparingWindow,
) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	startPlayback := func() {
		controlsW.playButton.SetIconName(pauseIcon)

		if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
			log.Info().Msg("Starting playback")

			if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "pause", false}}); err != nil {
				return err
			}

			var successResponse mpv.ResponseSuccess
			return decoder.Decode(&successResponse)
		}); err != nil {
			OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
			return
		}
	}

	pausePlayback := func() {
		controlsW.playButton.SetIconName(playIcon)

		if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
			log.Info().Msg("Pausing playback")

			if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "pause", true}}); err != nil {
				return err
			}

			var successResponse mpv.ResponseSuccess
			return decoder.Decode(&successResponse)
		}); err != nil {
			OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
			return
		}
	}

	var trackListResponse mpv.ResponseTrackList
	if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().Msg("Getting tracklist")

		if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "track-list"}}); err != nil {
			return err
		}

		return decoder.Decode(&trackListResponse)
	}); err != nil {
		OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
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

		controlsW.seeker.SetValue(position)

		elapsed := time.Duration(int64(position))

		if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
			if err := encoder.Encode(mpv.Request{[]interface{}{"seek", int64(elapsed.Seconds()), "absolute"}}); err != nil {
				return err
			}

			var successResponse mpv.ResponseSuccess
			return decoder.Decode(&successResponse)
		}); err != nil {
			OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
			return
		}

		log.Info().
			Dur("duration", elapsed).
			Msg("Seeking")

		remaining := total - elapsed

		controlsW.elapsedTrackLabel.SetLabel(formatDuration(elapsed))
		controlsW.remainingTrackLabel.SetLabel("-" + formatDuration(remaining))

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

			controlsW.headerbarSpinner.SetVisible(false)

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
				case <-controlsW.ctx.Done():
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
		for _, subtitle := range controlsW.subtitles {
			s = append(s, api.Subtitle{
				Name: subtitle.name,
				Size: subtitle.size,
			})
		}

		if err := encoder.Encode(api.NewMagnetLink(controlsW.magnetLink, controlsW.selectedTorrentMedia, controlsW.torrentTitle, controlsW.torrentReadme, s)); err != nil {
			log.Debug().
				Err(err).
				Msg("Could not encode magnet link, stopping")

			return
		}

		var elapsedResponse mpv.ResponseFloat64
		if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
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
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}

			positions.Broadcast(float64(elapsed.Nanoseconds()))
		}

		for {
			var j interface{}
			if len(controlsW.bufferedMessages) > 0 {
				j = controlsW.bufferedMessages[len(controlsW.bufferedMessages)-1]
				controlsW.bufferedMessages = controlsW.bufferedMessages[:len(controlsW.bufferedMessages)-1]
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
					controlsW.headerbarSpinner.SetVisible(true)

					if pausePlayback != nil {
						pausePlayback()
					}

					controlsW.playButton.SetIconName(pauseIcon)
				} else {
					controlsW.headerbarSpinner.SetVisible(false)

					if startPlayback != nil {
						startPlayback()
					}
				}
			}
		}
	}

	if controlsW.bufferedPeer != nil {
		go handlePeer(controlsW.bufferedPeer, controlsW.bufferedDecoder)
	}

	go func() {
		for {
			select {
			case <-controlsW.ctx.Done():
				if err := controlsW.ctx.Err(); err != context.Canceled {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				return
			case rid := <-controlsW.ids:
				log.Info().
					Str("raddr", controlsW.settings.GetString(resources.SchemaWeronURLKey)).
					Str("id", rid).
					Msg("Reconnecting to signaler")
			case peer := <-controlsW.adapter.Accept():
				go handlePeer(peer, nil)
			}
		}
	}()

	if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "volume", 100}}); err != nil {
			return err
		}

		var successResponse mpv.ResponseSuccess
		return decoder.Decode(&successResponse)
	}); err != nil {
		OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
		return
	}

	controlsW.setupSubtitleHandlers(subtracks, subtitlesDialog)

	controlsW.setupAudioTrackHandlers(audiotracks, audiotracksDialog)

	controlsW.setupSeekerHandlers(seekToPosition, positions, &seekerIsSeeking, &seekerIsUnderPointer)

	controlsW.setupMonitoringTicker(&total, &seekerIsSeeking, preparingWindow, pauses, buffering, positions)

	controlsW.setupVolumeControls()

	controlsW.setupMediaControls(subtitlesDialog, audiotracksDialog)

	controlsW.setupFullscreenControl()

	onPlayClicked := func(gtk.Button) {
		if !controlsW.headerbarSpinner.GetVisible() {
			if controlsW.playButton.GetIconName() == playIcon {
				pauses.Broadcast(false)
				startPlayback()
				return
			}

			pauses.Broadcast(true)
			pausePlayback()
		}
	}
	controlsW.playButton.ConnectClicked(&onPlayClicked)

	go func() {
		if err := controlsW.command.Wait(); err != nil && err.Error() != errKilled.Error() {
			OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
			return
		}

		controlsW.ApplicationWindow.Destroy()
	}()

	controlsW.playButton.GrabFocus()
}

func (c *ControlsWindow) setupSubtitleHandlers(subtracks []mediaWithPriorityAndID, subtitlesDialog SubtitlesDialog) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	subtitleActivators := []gtk.CheckButton{}

	for i, file := range append(
		append([]mediaWithPriorityAndID{
			{media: media{
				name: L("None"),
				size: 0,
			},
				priority: -1,
			},
		},
			subtracks...,
		), controlsW.subtitles...) {
		row := adw.NewActionRow()

		activator := gtk.NewCheckButton()

		if len(subtitleActivators) > 0 {
			activator.SetGroup(&subtitleActivators[i-1])
			activator.SetActive(false)
		} else {
			activator.SetActive(true)
		}
		subtitleActivators = append(subtitleActivators, *activator)

		m := file.name
		p := file.priority
		sid := file.id
		j := i
		onSubtitleActivate := func(gtk.CheckButton) {
			defer func() {
				if len(subtitleActivators) <= 1 {
					activator.SetActive(true)
				}
			}()

			if j == 0 {
				log.Info().
					Msg("Disabling subtitles")

				if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", "no"}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sub-visibility", "no"}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				return
			}

			if p == 0 {
				if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					log.Debug().
						Msg("Setting subtitle ID")

					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", sid}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sub-visibility", "yes"}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				return
			}

			go func() {
				defer func() {
					subtitlesDialog.DisableSpinner()
					subtitlesDialog.EnableOKButton()
				}()

				subtitlesDialog.DisableOKButton()
				subtitlesDialog.EnableSpinner()

				streamURL, err := getStreamURL(controlsW.apiAddr, controlsW.magnetLink, m)
				if err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				log.Info().
					Str("streamURL", streamURL).
					Msg("Downloading subtitles")

				hc := &http.Client{}

				req, err := http.NewRequest(http.MethodGet, streamURL, http.NoBody)
				if err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}
				req.SetBasicAuth(controlsW.apiUsername, controlsW.apiPassword)

				res, err := hc.Do(req)
				if err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}
				if res.Body != nil {
					defer res.Body.Close()
				}
				if res.StatusCode != http.StatusOK {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, errors.New(res.Status))
					return
				}

				log.Info().
					Str("streamURL", streamURL).
					Msg("Finished downloading subtitles")

				if err := utils.SetSubtitles(m, res.Body, controlsW.tmpDir, controlsW.ipcFile, &subtitleActivators[0], subtitlesDialog.Overlay()); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}
			}()
		}
		activator.ConnectActivate(&onSubtitleActivate)

		if i == 0 {
			row.SetTitle(file.name)
			row.SetSubtitle(L("Disable subtitles"))

			activator.SetActive(true)
		} else if file.priority == 0 {
			row.SetTitle(getDisplayPathWithoutRoot(file.name))
			row.SetSubtitle(L("Integrated subtitle"))
		} else if file.priority == 1 {
			row.SetTitle(getDisplayPathWithoutRoot(file.name))
			row.SetSubtitle(L("Subtitle from torrent"))
		} else {
			row.SetTitle(getDisplayPathWithoutRoot(file.name))
			row.SetSubtitle(L("Extra file from torrent"))
		}

		row.SetActivatable(true)

		row.AddPrefix(&activator.Widget)
		row.SetActivatableWidget(&activator.Widget)

		subtitlesDialog.AddSubtitleTrack(row)
	}

	onAddSubtitlesFromFileClicked := func(gtk.Button) {
		filePicker := gtk.NewFileChooserNative(
			L("Select subtitle file"),
			&controlsW.ApplicationWindow.Window,
			gtk.FileChooserActionOpenValue,
			"",
			"")
		filePicker.SetModal(true)
		onFilePickerResponse := func(dialog gtk.NativeDialog, responseId int) {
			if responseId == int(gtk.ResponseAcceptValue) {
				log.Info().
					Str("path", filePicker.GetFile().GetPath()).
					Msg("Setting subtitles")

				m := filePicker.GetFile().GetPath()
				subtitlesFile, err := os.Open(m)
				if err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}
				defer subtitlesFile.Close()

				if err := utils.SetSubtitles(m, subtitlesFile, controlsW.tmpDir, controlsW.ipcFile, &subtitleActivators[0], subtitlesDialog.Overlay()); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				row := adw.NewActionRow()

				activator := gtk.NewCheckButton()

				activator.SetGroup(&subtitleActivators[len(subtitleActivators)-1])
				subtitleActivators = append(subtitleActivators, *activator)

				activator.SetActive(true)
				onFileSubtitleActivate := func(gtk.CheckButton) {
					m := filePicker.GetFile().GetPath()
					subtitlesFile, err := os.Open(m)
					if err != nil {
						OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
						return
					}
					defer subtitlesFile.Close()

					if err := utils.SetSubtitles(m, subtitlesFile, controlsW.tmpDir, controlsW.ipcFile, &subtitleActivators[0], subtitlesDialog.Overlay()); err != nil {
						OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
						return
					}
				}
				activator.ConnectActivate(&onFileSubtitleActivate)

				row.SetTitle(filePicker.GetFile().GetBasename())
				row.SetSubtitle(L("Manually added"))

				row.SetActivatable(true)

				row.AddPrefix(&activator.Widget)
				row.SetActivatableWidget(&activator.Widget)

				subtitlesDialog.AddSubtitleTrack(row)
			}

			filePicker.Destroy()
		}
		filePicker.ConnectResponse(&onFilePickerResponse)

		filePicker.Show()
	}
	subtitlesDialog.SetAddFromFileCallback(func() {
		onAddSubtitlesFromFileClicked(gtk.Button{})
	})
}

func (c *ControlsWindow) setupAudioTrackHandlers(audiotracks []audioTrack, audiotracksDialog AudioTracksDialog) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	audiotrackActivators := []gtk.CheckButton{}

	for i, audiotrack := range append(
		[]audioTrack{
			{
				lang: L("None"),
				id:   -1,
			},
		},
		audiotracks...,
	) {
		row := adw.NewActionRow()

		activator := gtk.NewCheckButton()

		if len(audiotrackActivators) > 0 {
			activator.SetGroup(&audiotrackActivators[i-1])
			activator.SetActive(false)
		} else {
			activator.SetActive(true)
		}
		audiotrackActivators = append(audiotrackActivators, *activator)

		a := audiotrack
		j := i
		onAudiotrackActivate := func(gtk.CheckButton) {
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

				if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
					if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "aid", "no"}}); err != nil {
						return err
					}

					var successResponse mpv.ResponseSuccess
					return decoder.Decode(&successResponse)
				}); err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				return
			}

			if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
				log.Debug().
					Int("aid", a.id).
					Msg("Setting audio ID")

				if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "aid", a.id}}); err != nil {
					return err
				}

				var successResponse mpv.ResponseSuccess
				return decoder.Decode(&successResponse)
			}); err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}
		}
		activator.ConnectActivate(&onAudiotrackActivate)

		if j == 0 {
			row.SetSubtitle(L("Disable audio"))
		} else {
			row.SetSubtitle(fmt.Sprintf(L("Track %v"), a.id))
		}

		if i == 1 {
			activator.SetActive(true)
		}

		if strings.TrimSpace(a.lang) == "" {
			row.SetTitle(L("Untitled Track"))
		} else {
			row.SetTitle(a.lang)
		}

		row.SetActivatable(true)

		row.AddPrefix(&activator.Widget)
		row.SetActivatableWidget(&activator.Widget)

		audiotracksDialog.AddAudioTrack(row)
	}
}

func (c *ControlsWindow) setupSeekerHandlers(seekToPosition func(float64), positions *broadcast.Relay[float64], seekerIsSeeking *bool, seekerIsUnderPointer *bool) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	ctrl := gtk.NewEventControllerMotion()
	onEnter := func(gtk.EventControllerMotion, float64, float64) {
		*seekerIsUnderPointer = true
	}
	ctrl.ConnectEnter(&onEnter)
	onLeave := func(gtk.EventControllerMotion) {
		*seekerIsUnderPointer = false
	}
	ctrl.ConnectLeave(&onLeave)
	controlsW.seeker.AddController(&ctrl.EventController)

	onChangeValue := func(r gtk.Range, scroll gtk.ScrollType, value float64) bool {
		seekToPosition(value)
		positions.Broadcast(value)
		return true
	}
	controlsW.seeker.ConnectChangeValue(&onChangeValue)
}

func (c *ControlsWindow) setupMonitoringTicker(total *time.Duration, seekerIsSeeking *bool, preparingWindow PreparingWindow, pauses *broadcast.Relay[bool], buffering *broadcast.Relay[bool], positions *broadcast.Relay[float64]) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	preparingClosed := false
	done := make(chan struct{})
	previouslyBuffered := false

	go func() {
		t := time.NewTicker(time.Millisecond * 200)

		updateSeeker := func() {
			var durationResponse mpv.ResponseFloat64
			if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
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

			var err error
			*total, err = time.ParseDuration(fmt.Sprintf("%vs", int64(durationResponse.Data)))
			if err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}

			if *total != 0 && !preparingClosed {
				preparingWindow.SetVisible(false)
				preparingClosed = true
			}

			var elapsedResponse mpv.ResponseFloat64
			if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
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
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}

			var pausedResponse mpv.ResponseBool
			if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
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
			if pausedResponse.Data == (controlsW.playButton.GetIconName() == pauseIcon) {
				if !previouslyBuffered {
					previouslyBuffered = true

					controlsW.headerbarSpinner.SetVisible(true)
					buffering.Broadcast(true)
					pauses.Broadcast(true)
					positions.Broadcast(float64(elapsed.Nanoseconds()))
				}
			} else {
				if previouslyBuffered {
					previouslyBuffered = false

					controlsW.headerbarSpinner.SetVisible(false)
					buffering.Broadcast(false)
					pauses.Broadcast(false)
					positions.Broadcast(float64(elapsed.Nanoseconds()))
				}
			}

			if !*seekerIsSeeking {
				controlsW.seeker.
					SetRange(0, float64((*total).Nanoseconds()))
				controlsW.seeker.
					SetValue(float64(elapsed.Nanoseconds()))

				remaining := *total - elapsed

				log.Trace().
					Float64("total", (*total).Seconds()).
					Float64("elapsed", elapsed.Seconds()).
					Float64("remaining", remaining.Seconds()).
					Msg("Updating scale")

				controlsW.elapsedTrackLabel.SetLabel(formatDuration(elapsed))
				controlsW.remainingTrackLabel.SetLabel("-" + formatDuration(remaining))
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
}

func (c *ControlsWindow) setupVolumeControls() {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	onVolumeMuteClicked := func(gtk.Button) {
		if controlsW.volumeScale.GetValue() <= 0 {
			controlsW.volumeScale.SetValue(1)
		} else {
			controlsW.volumeScale.SetValue(0)
		}
	}
	controlsW.volumeMuteButton.ConnectClicked(&onVolumeMuteClicked)

	onVolumeValueChanged := func(gtk.Range) {
		value := controlsW.volumeScale.GetValue()

		if value <= 0 {
			controlsW.volumeButton.SetIconName("audio-volume-muted-symbolic")
			controlsW.volumeMuteButton.SetIconName("audio-volume-muted-symbolic")
		} else if value <= 0.3 {
			controlsW.volumeButton.SetIconName("audio-volume-low-symbolic")
			controlsW.volumeMuteButton.SetIconName("audio-volume-high-symbolic")
		} else if value <= 0.6 {
			controlsW.volumeButton.SetIconName("audio-volume-medium-symbolic")
			controlsW.volumeMuteButton.SetIconName("audio-volume-high-symbolic")
		} else {
			controlsW.volumeButton.SetIconName("audio-volume-high-symbolic")
			controlsW.volumeMuteButton.SetIconName("audio-volume-high-symbolic")
		}

		if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {

			log.Info().
				Float64("value", value).
				Msg("Setting volume")

			if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "volume", value * 100}}); err != nil {
				return err
			}

			var successResponse mpv.ResponseSuccess
			return decoder.Decode(&successResponse)
		}); err != nil {
			OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
		}
	}
	controlsW.volumeScale.ConnectValueChanged(&onVolumeValueChanged)
}

func (c *ControlsWindow) setupMediaControls(subtitlesDialog SubtitlesDialog, audiotracksDialog AudioTracksDialog) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	onSubtitleClicked := func(gtk.Button) {
		subtitlesDialog.Present()
	}
	controlsW.subtitleButton.ConnectClicked(&onSubtitleClicked)

	onAudiotracksClicked := func(gtk.Button) {
		audiotracksDialog.Present()
	}
	controlsW.audiotracksButton.ConnectClicked(&onAudiotracksClicked)

	for _, d := range []adw.Window{subtitlesDialog.Window, audiotracksDialog.Window} {
		dialog := d

		escCtrl := gtk.NewEventControllerKey()
		dialog.AddController(&escCtrl.EventController)
		dialog.SetTransientFor(&controlsW.ApplicationWindow.Window)

		onEscKeyReleased := func(ctrl gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) {
			if keycode == keycodeEscape {
				dialog.Close()
				dialog.SetVisible(false)
			}
		}
		escCtrl.ConnectKeyReleased(&onEscKeyReleased)
	}

	onSubtitlesCancelClicked := func(gtk.Button) {
		log.Info().
			Msg("Disabling subtitles")

		if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
			if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", "no"}}); err != nil {
				return err
			}

			var successResponse mpv.ResponseSuccess
			return decoder.Decode(&successResponse)
		}); err != nil {
			OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
			return
		}

		subtitlesDialog.Close()
	}
	subtitlesDialog.SetCancelCallback(func() {
		onSubtitlesCancelClicked(gtk.Button{})
	})

	onSubtitlesOKClicked := func(gtk.Button) {
		subtitlesDialog.Close()
		subtitlesDialog.SetVisible(false)
	}
	subtitlesDialog.SetOKCallback(func() {
		onSubtitlesOKClicked(gtk.Button{})
	})

	onAudiotracksCancelClicked := func(gtk.Button) {
		audiotracksDialog.Close()
		subtitlesDialog.SetVisible(false)
	}
	audiotracksDialog.SetCancelCallback(func() {
		onAudiotracksCancelClicked(gtk.Button{})
	})

	onAudiotracksOKClicked := func(gtk.Button) {
		audiotracksDialog.Close()
		subtitlesDialog.SetVisible(false)
	}
	audiotracksDialog.SetOKCallback(func() {
		onAudiotracksOKClicked(gtk.Button{})
	})
}

func (c *ControlsWindow) setupFullscreenControl() {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	onFullscreenClicked := func(gtk.Button) {
		if controlsW.fullscreenButton.GetActive() {
			if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
				log.Info().Msg("Enabling fullscreen")

				if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "fullscreen", true}}); err != nil {
					return err
				}

				var successResponse mpv.ResponseSuccess
				return decoder.Decode(&successResponse)
			}); err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}

			return
		}

		if err := mpvClient.ExecuteMPVRequest(controlsW.ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
			log.Info().Msg("Disabling fullscreen")

			if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "fullscreen", false}}); err != nil {
				return err
			}

			var successResponse mpv.ResponseSuccess
			return decoder.Decode(&successResponse)
		}); err != nil {
			OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
			return
		}
	}
	controlsW.fullscreenButton.ConnectClicked(&onFullscreenClicked)
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceControlsPath)

		typeClass.BindTemplateChildFull("toast_overlay", false, 0)
		typeClass.BindTemplateChildFull("button_headerbar_title", false, 0)
		typeClass.BindTemplateChildFull("button_headerbar_subtitle", false, 0)
		typeClass.BindTemplateChildFull("play_button", false, 0)
		typeClass.BindTemplateChildFull("stop_button", false, 0)
		typeClass.BindTemplateChildFull("volume_scale", false, 0)
		typeClass.BindTemplateChildFull("volume_button", false, 0)
		typeClass.BindTemplateChildFull("audiovolume_button_mute_button", false, 0)
		typeClass.BindTemplateChildFull("subtitle_button", false, 0)
		typeClass.BindTemplateChildFull("audiotracks_button", false, 0)
		typeClass.BindTemplateChildFull("fullscreen_button", false, 0)
		typeClass.BindTemplateChildFull("media_info_button", false, 0)
		typeClass.BindTemplateChildFull("headerbar_spinner", false, 0)
		typeClass.BindTemplateChildFull("menu_button", false, 0)
		typeClass.BindTemplateChildFull("elapsed_track_label", false, 0)
		typeClass.BindTemplateChildFull("remaining_track_label", false, 0)
		typeClass.BindTemplateChildFull("seeker", false, 0)
		typeClass.BindTemplateChildFull("watching_with_title_label", false, 0)
		typeClass.BindTemplateChildFull("stream_code_input", false, 0)
		typeClass.BindTemplateChildFull("copy_stream_code_button", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.ApplicationWindow
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				overlay                 adw.ToastOverlay
				buttonHeaderbarTitle    gtk.Label
				buttonHeaderbarSubtitle gtk.Label
				playButton              gtk.Button
				stopButton              gtk.Button
				volumeScale             gtk.Scale
				volumeButton            gtk.MenuButton
				volumeMuteButton        gtk.Button
				subtitleButton          gtk.Button
				audiotracksButton       gtk.Button
				fullscreenButton        gtk.ToggleButton
				mediaInfoButton         gtk.Button
				headerbarSpinner        adw.Spinner
				menuButton              gtk.MenuButton
				elapsedTrackLabel       gtk.Label
				remainingTrackLabel     gtk.Label
				seeker                  gtk.Scale
				watchingWithTitleLabel  gtk.Label
				streamCodeInput         gtk.Entry
				copyStreamCodeButton    gtk.Button
			)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "toast_overlay").Cast(&overlay)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "button_headerbar_title").Cast(&buttonHeaderbarTitle)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "button_headerbar_subtitle").Cast(&buttonHeaderbarSubtitle)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "play_button").Cast(&playButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "stop_button").Cast(&stopButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "volume_scale").Cast(&volumeScale)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "volume_button").Cast(&volumeButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "audiovolume_button_mute_button").Cast(&volumeMuteButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "subtitle_button").Cast(&subtitleButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "audiotracks_button").Cast(&audiotracksButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "fullscreen_button").Cast(&fullscreenButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "media_info_button").Cast(&mediaInfoButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "headerbar_spinner").Cast(&headerbarSpinner)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "menu_button").Cast(&menuButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "elapsed_track_label").Cast(&elapsedTrackLabel)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "remaining_track_label").Cast(&remainingTrackLabel)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "seeker").Cast(&seeker)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "watching_with_title_label").Cast(&watchingWithTitleLabel)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "stream_code_input").Cast(&streamCodeInput)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "copy_stream_code_button").Cast(&copyStreamCodeButton)

			c := &ControlsWindow{
				ApplicationWindow:       parent,
				overlay:                 &overlay,
				buttonHeaderbarTitle:    &buttonHeaderbarTitle,
				buttonHeaderbarSubtitle: &buttonHeaderbarSubtitle,
				playButton:              &playButton,
				stopButton:              &stopButton,
				volumeScale:             &volumeScale,
				volumeButton:            &volumeButton,
				volumeMuteButton:        &volumeMuteButton,
				subtitleButton:          &subtitleButton,
				audiotracksButton:       &audiotracksButton,
				fullscreenButton:        &fullscreenButton,
				mediaInfoButton:         &mediaInfoButton,
				headerbarSpinner:        &headerbarSpinner,
				menuButton:              &menuButton,
				elapsedTrackLabel:       &elapsedTrackLabel,
				remainingTrackLabel:     &remainingTrackLabel,
				seeker:                  &seeker,
				watchingWithTitleLabel:  &watchingWithTitleLabel,
				streamCodeInput:         &streamCodeInput,
				copyStreamCodeButton:    &copyStreamCodeButton,
			}

			var pinner runtime.Pinner
			pinner.Pin(c)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(c)), &onCleanup)
		})
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	parentQuery := newTypeQuery(adw.ApplicationWindowGLibType())

	gTypeControlsWindow = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexControlsWindow",
		uint(parentQuery.ClassSize),
		&classInit,
		uint(parentQuery.InstanceSize)+uint(unsafe.Sizeof(ControlsWindow{}))+uint(unsafe.Sizeof(&ControlsWindow{})),
		&instanceInit,
		0,
	)
}
