package components

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
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

	"codeberg.org/puregotk/puregotk/examples/gstreamer-go/gst"
	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"github.com/anacrolix/torrent"
	"github.com/mitchellh/mapstructure"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/assets/resources"
	"github.com/pojntfx/multiplex/internal/player"
	"github.com/pojntfx/multiplex/internal/utils"
	api "github.com/pojntfx/multiplex/pkg/api/webrtc/v1"
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
	pictureVideo            *gtk.Picture
	headerbarRevealer       *gtk.Revealer
	controlsRevealer        *gtk.Revealer
	buttonHeaderbarTitle    *gtk.Label
	buttonHeaderbarSubtitle *gtk.Label
	playButton              *gtk.Button
	stopButton              *gtk.Button
	volumeScale             *gtk.Scale
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

	player *player.Player

	// Dialog state (re-applied each time the dialog is re-presented, since
	// Adw.Dialog destroys itself on Close()).
	availableSubtracks   []mediaWithPriorityAndID
	availableAudiotracks []audioTrack
	manualSubtitles      []string
	selectedSubtitle     int
	selectedAudio        int

	// Progress state, populated by the metrics ticker so on-demand dialogs
	// (media info) can show the latest values.
	progressLength    float64
	progressCompleted float64
	progressPeers     int
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

// buildPlaybackURI turns a stream URL or local path into a URI GStreamer can consume.
// HTTP(S) URLs get basic-auth credentials embedded as userinfo; local filesystem paths
// get wrapped as file:// URIs.
func buildPlaybackURI(streamURL, username, password string) (string, error) {
	if strings.HasPrefix(streamURL, "http://") || strings.HasPrefix(streamURL, "https://") {
		u, err := url.Parse(streamURL)
		if err != nil {
			return "", err
		}
		if username != "" || password != "" {
			u.User = url.UserPassword(username, password)
		}
		return u.String(), nil
	}

	abs, err := filepath.Abs(streamURL)
	if err != nil {
		return "", err
	}
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	return u.String(), nil
}

func (c *ControlsWindow) setup() error {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	controlsW.app.GetStyleManager().SetColorScheme(adw.ColorSchemePreferDarkValue)

	preparingDialog := NewPreparingDialog()

	controlsW.buttonHeaderbarTitle.SetLabel(controlsW.torrentTitle)
	controlsW.buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(controlsW.selectedTorrentMedia))

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
		d := NewDescriptionDialog()
		d.HeaderbarTitle().SetLabel(controlsW.torrentTitle)
		d.HeaderbarSubtitle().SetLabel(getDisplayPathWithoutRoot(controlsW.selectedTorrentMedia))

		d.Text().SetWrapMode(gtk.WrapWordValue)
		if !utf8.Valid([]byte(controlsW.torrentReadme)) || strings.TrimSpace(controlsW.torrentReadme) == "" {
			d.Text().GetBuffer().SetText(L(readmePlaceholder), -1)
		} else {
			d.Text().GetBuffer().SetText(controlsW.torrentReadme, -1)
		}

		pb := d.PreparingProgressBar()
		pb.SetVisible(true)
		if controlsW.progressLength > 0 {
			pb.SetFraction(controlsW.progressCompleted / controlsW.progressLength)
			pb.SetText(fmt.Sprintf(L("%v MB/%v MB (%v peers)"),
				int(controlsW.progressCompleted/1000/1000),
				int(controlsW.progressLength/1000/1000),
				controlsW.progressPeers))
		} else {
			pb.SetText(L("Searching for peers"))
		}

		d.Present(&controlsW.ApplicationWindow.Widget)
	}
	controlsW.mediaInfoButton.ConnectClicked(&onMediaInfoButton)

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

			controlsW.progressLength = length
			controlsW.progressCompleted = completed
			controlsW.progressPeers = peers

			pb := preparingDialog.ProgressBar()
			if length > 0 {
				pb.SetFraction(completed / length)
				pb.SetText(fmt.Sprintf(L("%v MB/%v MB (%v peers)"), int(completed/1000/1000), int(length/1000/1000), peers))
			} else {
				pb.SetText(L("Searching for peers"))
			}
		}
	}()

	preparingDialog.SetCloseRequestCallback(func() bool {
		return true
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

		preparingDialog.Close()

		mainWindow := NewMainWindow(controlsW.ctx, controlsW.app, controlsW.manager, controlsW.apiAddr, controlsW.apiUsername, controlsW.apiPassword, controlsW.settings, controlsW.gateway, controlsW.cancel, controlsW.tmpDir)

		controlsW.app.AddWindow(&mainWindow.ApplicationWindow.Window)
		mainWindow.SetVisible(true)
	}
	preparingDialog.SetCancelCallback(func() {
		onPrepCancel(gtk.Button{})
	})

	controlsW.player = player.New(controlsW.pictureVideo)

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
			controlsW.player.Stop()
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
		preparingDialog.Present(&controlsW.ApplicationWindow.Widget)

		onCloseRequest := func(gtk.Window) bool {
			controlsW.adapter.Close()
			controlsW.cancelAdapterCtx()

			pauses.Close()
			positions.Close()
			buffering.Close()

			progressBarTicker.Stop()

			controlsW.player.Stop()

			return false
		}
		controlsW.ApplicationWindow.ConnectCloseRequest(&onCloseRequest)

		go func() {
			<-controlsW.ready

			uri, err := buildPlaybackURI(controlsW.streamURL, controlsW.apiUsername, controlsW.apiPassword)
			if err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}

			// playbin's gtk4paintablesink requires paintable retrieval on the main
			// thread, so drop back onto the GTK main loop before loading.
			loaded := make(chan error, 1)
			var loadOnMain glib.SourceFunc = func(uintptr) bool {
				loaded <- controlsW.player.Load(uri, "")
				return false
			}
			glib.IdleAdd(&loadOnMain, 0)
			if err := <-loaded; err != nil {
				OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
				return
			}

			controlsW.player.SetCallback(func(ev player.Event, data interface{}) {
				switch ev {
				case player.EventError:
					if gerr, ok := data.(*glib.Error); ok && gerr != nil {
						log.Error().Str("err", gerr.Error()).Msg("pipeline error")
						OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, errors.New(gerr.Error()))
					}
				case player.EventWarning:
					if gerr, ok := data.(*glib.Error); ok && gerr != nil {
						log.Warn().Str("err", gerr.Error()).Msg("pipeline warning")
					}
				case player.EventInfo:
					if gerr, ok := data.(*glib.Error); ok && gerr != nil {
						log.Info().Str("msg", gerr.Error()).Msg("pipeline info")
					}
				case player.EventBuffering:
					controlsW.headerbarSpinner.SetVisible(true)
					buffering.Broadcast(true)
				case player.EventBufferingDone:
					controlsW.headerbarSpinner.SetVisible(false)
					buffering.Broadcast(false)
				case player.EventEOS:
					log.Info().Msg("end of stream")
				}
			})

			// Start paused so setupPlaybackControls drives state changes.
			controlsW.player.Pause()

			controlsW.setupPlaybackControls(
				pauses,
				positions,
				buffering,
				syncWatchingWithLabel,
				preparingDialog,
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
	preparingDialog *PreparingDialog,
) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	startPlayback := func() {
		controlsW.playButton.SetIconName(pauseIcon)
		log.Info().Msg("Starting playback")
		controlsW.player.Play()
	}

	pausePlayback := func() {
		controlsW.playButton.SetIconName(playIcon)
		log.Info().Msg("Pausing playback")
		controlsW.player.Pause()
	}

	// Wait for tracks to be discovered before enumerating counts.
	// playbin populates n-audio/n-text once moved past READY.
	{
		deadline := time.Now().Add(5 * time.Second)
		for {
			aCount, tCount := controlsW.player.TrackCounts()
			if aCount+tCount > 0 || time.Now().After(deadline) {
				controlsW.availableAudiotracks = nil
				for i := 0; i < aCount; i++ {
					controlsW.availableAudiotracks = append(controlsW.availableAudiotracks, audioTrack{
						lang: fmt.Sprintf(L("Track %v"), i+1),
						id:   i,
					})
				}
				controlsW.availableSubtracks = nil
				for i := 0; i < tCount; i++ {
					controlsW.availableSubtracks = append(controlsW.availableSubtracks, mediaWithPriorityAndID{
						media: media{
							name: fmt.Sprintf(L("Embedded subtitle %v"), i+1),
							size: 0,
						},
						id:       i,
						priority: 0,
					})
				}
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	controlsW.selectedSubtitle = 0
	if len(controlsW.availableAudiotracks) > 0 {
		controlsW.selectedAudio = 1
	}

	seekerIsSeeking := false
	seekerIsUnderPointer := false
	total := time.Duration(0)
	seekToPosition := func(position float64) {
		seekerIsSeeking = true

		controlsW.seeker.SetValue(position)

		elapsed := time.Duration(int64(position))

		controlsW.player.Seek(elapsed)

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

		positions.Broadcast(float64(controlsW.player.Position().Nanoseconds()))

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

	controlsW.player.SetVolume(1.0)

	controlsW.setupSeekerHandlers(seekToPosition, positions, &seekerIsSeeking, &seekerIsUnderPointer)

	controlsW.setupMonitoringTicker(&total, &seekerIsSeeking, preparingDialog, pauses, buffering, positions)

	controlsW.setupVolumeControls()

	controlsW.setupMediaControls()

	controlsW.setupFullscreenControl()

	controlsW.setupOSDRevealers()

	togglePlayback := func() {
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

	onPlayClicked := func(gtk.Button) {
		togglePlayback()
	}
	controlsW.playButton.ConnectClicked(&onPlayClicked)

	togglePlaybackAction := gio.NewSimpleAction("togglePlayback", nil)
	onTogglePlayback := func(action gio.SimpleAction, parameter uintptr) {
		togglePlayback()
	}
	togglePlaybackAction.ConnectActivate(&onTogglePlayback)
	controlsW.ApplicationWindow.AddAction(togglePlaybackAction)
	controlsW.app.SetAccelsForAction("win.togglePlayback", []string{"<Ctrl>space"})

	toggleFullscreenAction := gio.NewSimpleAction("toggleFullscreen", nil)
	onToggleFullscreen := func(action gio.SimpleAction, parameter uintptr) {
		// We call `.Activate` on the button here instead of the actual handler for
		// toggling fullscreen mode so that we also change the fullscreen button's state
		controlsW.fullscreenButton.Activate()
	}
	toggleFullscreenAction.ConnectActivate(&onToggleFullscreen)
	controlsW.ApplicationWindow.AddAction(toggleFullscreenAction)
	controlsW.app.SetAccelsForAction("win.toggleFullscreen", []string{"F11"})

	controlsW.playButton.GrabFocus()
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

func (c *ControlsWindow) setupMonitoringTicker(total *time.Duration, seekerIsSeeking *bool, preparingDialog *PreparingDialog, pauses *broadcast.Relay[bool], buffering *broadcast.Relay[bool], positions *broadcast.Relay[float64]) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	preparingClosed := false
	done := make(chan struct{})
	previouslyBuffered := false

	go func() {
		t := time.NewTicker(time.Millisecond * 200)

		updateSeeker := func() {
			*total = controlsW.player.Duration()

			if *total != 0 && !preparingClosed {
				preparingDialog.ForceClose()
				preparingClosed = true
			}

			elapsed := controlsW.player.Position()
			state := controlsW.player.State()
			isPaused := state != gst.StatePlayingValue

			// If GStreamer is paused, but the GUI shows the playing state, we're buffering
			if isPaused == (controlsW.playButton.GetIconName() == pauseIcon) {
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

		switch {
		case value <= 0:
			controlsW.volumeMuteButton.SetIconName("audio-volume-muted-symbolic")
		case value <= 0.3:
			controlsW.volumeMuteButton.SetIconName("audio-volume-low-symbolic")
		case value <= 0.6:
			controlsW.volumeMuteButton.SetIconName("audio-volume-medium-symbolic")
		default:
			controlsW.volumeMuteButton.SetIconName("audio-volume-high-symbolic")
		}

		log.Info().
			Float64("value", value).
			Msg("Setting volume")

		controlsW.player.SetVolume(value)
	}
	controlsW.volumeScale.ConnectValueChanged(&onVolumeValueChanged)
}


func (c *ControlsWindow) buildSubtitleOptions() []mediaWithPriorityAndID {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	opts := []mediaWithPriorityAndID{{
		media:    media{name: L("None"), size: 0},
		priority: -1,
	}}
	opts = append(opts, controlsW.availableSubtracks...)
	opts = append(opts, controlsW.subtitles...)
	for _, path := range controlsW.manualSubtitles {
		opts = append(opts, mediaWithPriorityAndID{
			media:    media{name: path, size: 0},
			priority: 2,
		})
	}
	return opts
}

func (c *ControlsWindow) populateSubtitlesDialog(dialog *SubtitlesDialog) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	options := controlsW.buildSubtitleOptions()

	activators := make([]*gtk.CheckButton, 0, len(options))
	for i, file := range options {
		row := adw.NewActionRow()

		activator := gtk.NewCheckButton()
		if len(activators) > 0 {
			activator.SetGroup(activators[i-1])
		}
		activators = append(activators, activator)
		activator.SetActive(i == controlsW.selectedSubtitle)

		idx := i
		entry := file
		onActivate := func(gtk.CheckButton) {
			if !activator.GetActive() {
				return
			}

			controlsW.selectedSubtitle = idx

			switch {
			case idx == 0:
				log.Info().Msg("Disabling subtitles")
				controlsW.player.SetSubtitleTrack(-1)
			case entry.priority == 0:
				log.Debug().Int("sid", entry.id).Msg("Setting subtitle ID")
				controlsW.player.SetSubtitleTrack(entry.id)
			case entry.priority == 2 && isLocalFile(entry.name):
				subURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(entry.name)}).String()
				controlsW.player.SetSubtitleURI(subURI)
			case entry.priority == 1 || entry.priority == 2:
				go func() {
					defer func() {
						dialog.DisableSpinner()
						dialog.EnableOKButton()
					}()

					dialog.DisableOKButton()
					dialog.EnableSpinner()

					streamURL, err := getStreamURL(controlsW.apiAddr, controlsW.magnetLink, entry.name)
					if err != nil {
						OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
						return
					}

					log.Info().Str("streamURL", streamURL).Msg("Downloading subtitles")

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

					subtitlePath, err := utils.SaveSubtitles(entry.name, res.Body, controlsW.tmpDir)
					if err != nil {
						OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
						return
					}

					subURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(subtitlePath)}).String()
					controlsW.player.SetSubtitleURI(subURI)
				}()
			}
		}
		activator.ConnectActivate(&onActivate)

		switch {
		case i == 0:
			row.SetTitle(file.name)
			row.SetSubtitle(L("Disable subtitles"))
		case file.priority == 0:
			row.SetTitle(getDisplayPathWithoutRoot(file.name))
			row.SetSubtitle(L("Integrated subtitle"))
		case file.priority == 1:
			row.SetTitle(getDisplayPathWithoutRoot(file.name))
			row.SetSubtitle(L("Subtitle from torrent"))
		default:
			row.SetTitle(filepath.Base(file.name))
			row.SetSubtitle(L("Manually added"))
		}

		row.SetActivatable(true)
		row.AddPrefix(&activator.Widget)
		row.SetActivatableWidget(&activator.Widget)

		dialog.AddSubtitleTrack(row)
	}

	dialog.SetAddFromFileCallback(func() {
		filePicker := gtk.NewFileChooserNative(
			L("Select subtitle file"),
			&controlsW.ApplicationWindow.Window,
			gtk.FileChooserActionOpenValue,
			"",
			"")
		filePicker.SetModal(true)
		onFilePickerResponse := func(_ gtk.NativeDialog, responseId int32) {
			if responseId == int32(gtk.ResponseAcceptValue) {
				m := filePicker.GetFile().GetPath()
				log.Info().Str("path", m).Msg("Setting subtitles")

				subtitlesFile, err := os.Open(m)
				if err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}
				defer subtitlesFile.Close()

				subtitlePath, err := utils.SaveSubtitles(m, subtitlesFile, controlsW.tmpDir)
				if err != nil {
					OpenErrorDialog(controlsW.ctx, &controlsW.ApplicationWindow, err)
					return
				}

				controlsW.manualSubtitles = append(controlsW.manualSubtitles, subtitlePath)
				controlsW.selectedSubtitle = len(controlsW.buildSubtitleOptions()) - 1

				subURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(subtitlePath)}).String()
				controlsW.player.SetSubtitleURI(subURI)

				dialog.Close()
			}

			filePicker.Destroy()
		}
		filePicker.ConnectResponse(&onFilePickerResponse)

		filePicker.Show()
	})
}

func isLocalFile(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (c *ControlsWindow) buildAudioOptions() []audioTrack {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	return append([]audioTrack{{lang: L("None"), id: -1}}, controlsW.availableAudiotracks...)
}

func (c *ControlsWindow) populateAudioTracksDialog(dialog *AudioTracksDialog) {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	options := controlsW.buildAudioOptions()

	activators := make([]*gtk.CheckButton, 0, len(options))
	for i, track := range options {
		row := adw.NewActionRow()

		activator := gtk.NewCheckButton()
		if len(activators) > 0 {
			activator.SetGroup(activators[i-1])
		}
		activators = append(activators, activator)
		activator.SetActive(i == controlsW.selectedAudio)

		idx := i
		a := track
		onActivate := func(gtk.CheckButton) {
			if !activator.GetActive() {
				return
			}

			if len(options) <= 1 {
				activator.SetActive(true)
				return
			}

			controlsW.selectedAudio = idx

			if idx == 0 {
				log.Info().Msg("Disabling audio track")
				controlsW.player.SetAudioTrack(-1)
				return
			}

			log.Debug().Int("aid", a.id).Msg("Setting audio ID")
			controlsW.player.SetAudioTrack(a.id)
		}
		activator.ConnectActivate(&onActivate)

		if i == 0 {
			row.SetSubtitle(L("Disable audio"))
		} else {
			row.SetSubtitle(fmt.Sprintf(L("Track %v"), a.id+1))
		}

		if strings.TrimSpace(a.lang) == "" {
			row.SetTitle(L("Untitled Track"))
		} else {
			row.SetTitle(a.lang)
		}

		row.SetActivatable(true)
		row.AddPrefix(&activator.Widget)
		row.SetActivatableWidget(&activator.Widget)

		dialog.AddAudioTrack(row)
	}
}

func (c *ControlsWindow) setupMediaControls() {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	onSubtitleClicked := func(gtk.Button) {
		d := NewSubtitlesDialog()
		controlsW.populateSubtitlesDialog(d)
		d.SetCancelCallback(func() {
			log.Info().Msg("Disabling subtitles")
			controlsW.selectedSubtitle = 0
			controlsW.player.SetSubtitleTrack(-1)
			d.Close()
		})
		d.SetOKCallback(func() { d.Close() })
		d.Present(&controlsW.ApplicationWindow.Widget)
	}
	controlsW.subtitleButton.ConnectClicked(&onSubtitleClicked)

	onAudiotracksClicked := func(gtk.Button) {
		d := NewAudioTracksDialog()
		controlsW.populateAudioTracksDialog(d)
		d.SetCancelCallback(func() { d.Close() })
		d.SetOKCallback(func() { d.Close() })
		d.Present(&controlsW.ApplicationWindow.Widget)
	}
	controlsW.audiotracksButton.ConnectClicked(&onAudiotracksClicked)
}

func (c *ControlsWindow) setupFullscreenControl() {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	onFullscreenClicked := func(gtk.Button) {
		if controlsW.fullscreenButton.GetActive() {
			log.Info().Msg("Enabling fullscreen")
			controlsW.ApplicationWindow.Fullscreen()
			return
		}

		log.Info().Msg("Disabling fullscreen")
		controlsW.ApplicationWindow.Unfullscreen()
	}
	controlsW.fullscreenButton.ConnectClicked(&onFullscreenClicked)
}

// setupOSDRevealers hides the headerbar and playback toolbar after a short idle
// period and brings them back when the user moves the pointer over the video.
func (c *ControlsWindow) setupOSDRevealers() {
	controlsW := (*ControlsWindow)(unsafe.Pointer(c.Widget.GetData(dataKeyGoInstance)))

	const hideAfter = 3 * time.Second

	var hideTimer *time.Timer
	reveal := func(show bool) {
		controlsW.headerbarRevealer.SetRevealChild(show)
		controlsW.controlsRevealer.SetRevealChild(show)
	}

	scheduleHide := func() {
		if hideTimer != nil {
			hideTimer.Stop()
		}
		hideTimer = time.AfterFunc(hideAfter, func() {
			var hideOnMain glib.SourceFunc = func(uintptr) bool {
				reveal(false)
				return false
			}
			glib.IdleAdd(&hideOnMain, 0)
		})
	}

	motion := gtk.NewEventControllerMotion()
	motion.SetPropagationPhase(gtk.PhaseCaptureValue)

	var lastX, lastY float64
	onMotion := func(_ gtk.EventControllerMotion, x, y float64) {
		// GTK occasionally fires motion events with unchanged coordinates (e.g.
		// during relayout or fullscreen transitions). Ignore those so the hide
		// timer actually gets a chance to elapse.
		if x == lastX && y == lastY {
			return
		}
		lastX, lastY = x, y

		reveal(true)
		scheduleHide()
	}
	motion.ConnectMotion(&onMotion)
	onLeave := func(gtk.EventControllerMotion) {
		scheduleHide()
	}
	motion.ConnectLeave(&onLeave)
	controlsW.ApplicationWindow.Widget.AddController(&motion.EventController)

	scheduleHide()
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceControlsPath)

		typeClass.BindTemplateChildFull("toast_overlay", false, 0)
		typeClass.BindTemplateChildFull("picture_video", false, 0)
		typeClass.BindTemplateChildFull("headerbar_revealer", false, 0)
		typeClass.BindTemplateChildFull("controls_revealer", false, 0)
		typeClass.BindTemplateChildFull("button_headerbar_title", false, 0)
		typeClass.BindTemplateChildFull("button_headerbar_subtitle", false, 0)
		typeClass.BindTemplateChildFull("play_button", false, 0)
		typeClass.BindTemplateChildFull("stop_button", false, 0)
		typeClass.BindTemplateChildFull("volume_scale", false, 0)
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
				pictureVideo            gtk.Picture
				headerbarRevealer       gtk.Revealer
				controlsRevealer        gtk.Revealer
				buttonHeaderbarTitle    gtk.Label
				buttonHeaderbarSubtitle gtk.Label
				playButton              gtk.Button
				stopButton              gtk.Button
				volumeScale             gtk.Scale
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
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "picture_video").Cast(&pictureVideo)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "headerbar_revealer").Cast(&headerbarRevealer)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "controls_revealer").Cast(&controlsRevealer)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "button_headerbar_title").Cast(&buttonHeaderbarTitle)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "button_headerbar_subtitle").Cast(&buttonHeaderbarSubtitle)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "play_button").Cast(&playButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "stop_button").Cast(&stopButton)
			parent.Widget.GetTemplateChild(gTypeControlsWindow, "volume_scale").Cast(&volumeScale)
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
				pictureVideo:            &pictureVideo,
				headerbarRevealer:       &headerbarRevealer,
				controlsRevealer:        &controlsRevealer,
				buttonHeaderbarTitle:    &buttonHeaderbarTitle,
				buttonHeaderbarSubtitle: &buttonHeaderbarSubtitle,
				playButton:              &playButton,
				stopButton:              &stopButton,
				volumeScale:             &volumeScale,
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

	var parentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.ApplicationWindowGLibType(), &parentQuery)

	gTypeControlsWindow = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexControlsWindow",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
