// Package mpris exposes an org.mpris.MediaPlayer2 service on the session bus
// so GNOME Shell, the media-keys plugin and other MPRIS consumers can observe
// and control playback. Only the subset of the spec we can meaningfully
// implement is exported.
package mpris

import (
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
	mpris "github.com/leberKleber/go-mpris"
)

const (
	objectPath     = "/org/mpris/MediaPlayer2"
	rootInterface  = "org.mpris.MediaPlayer2"
	playerIface    = "org.mpris.MediaPlayer2.Player"
	busNamePrefix  = "org.mpris.MediaPlayer2."
)

// Controller is implemented by the application to service MPRIS requests.
// All methods must be safe to call from a non-main goroutine — the D-Bus
// handler runs on its own thread.
type Controller interface {
	Play()
	Pause()
	PlayPause()
	Stop()
	Seek(offset time.Duration) // positive forwards, negative backwards
	SetPosition(pos time.Duration)
	Raise() // bring window to front
	Quit()

	Position() time.Duration
	Duration() time.Duration
	IsPlaying() bool
	IsPaused() bool

	// Title is shown in GNOME Shell under the player entry.
	Title() string
}

// Service wraps a D-Bus connection exposing the MPRIS interface.
// A nil *Service is a no-op so callers can treat it as always-on.
type Service struct {
	mu     sync.Mutex
	conn   *dbus.Conn
	props  *prop.Properties
	ctrl   Controller
	appID  string
}

// New registers the MPRIS service on the session bus. `appID` is the flatpak
// application id, used to build the well-known bus name. `desktopEntry` is
// the .desktop file basename without the extension.
func New(appID, desktopEntry string, ctrl Controller) (*Service, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}

	s := &Service{conn: conn, ctrl: ctrl, appID: appID}

	root := &rootHandler{s: s, desktopEntry: desktopEntry}
	player := &playerHandler{s: s}

	if err := conn.Export(root, objectPath, rootInterface); err != nil {
		return nil, err
	}
	if err := conn.Export(player, objectPath, playerIface); err != nil {
		return nil, err
	}

	props := prop.New(conn, objectPath, map[string]map[string]*prop.Prop{
		rootInterface: {
			"CanQuit":             readOnly(true),
			"CanRaise":            readOnly(true),
			"HasTrackList":        readOnly(false),
			"Identity":            readOnly("Multiplex"),
			"DesktopEntry":        readOnly(desktopEntry),
			"SupportedUriSchemes": readOnly([]string{"magnet", "http", "https", "file"}),
			"SupportedMimeTypes":  readOnly([]string{"video/mp4", "video/x-matroska", "video/webm", "video/quicktime", "video/x-msvideo"}),
		},
		playerIface: {
			"PlaybackStatus": readOnly(string(mpris.PlaybackStatusStopped)),
			"Rate":           readOnly(1.0),
			"MinimumRate":    readOnly(1.0),
			"MaximumRate":    readOnly(1.0),
			"Metadata":       readOnly(mpris.Metadata{}),
			"Volume":         readOnly(1.0),
			"Position":       readOnly(int64(0)),
			"CanGoNext":      readOnly(false),
			"CanGoPrevious":  readOnly(false),
			"CanPlay":        readOnly(true),
			"CanPause":       readOnly(true),
			"CanSeek":        readOnly(true),
			"CanControl":     readOnly(true),
		},
	})
	s.props = props

	node := &introspect.Node{
		Name: objectPath,
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			prop.IntrospectData,
			{
				Name:       rootInterface,
				Methods:    introspect.Methods(root),
				Properties: props.Introspection(rootInterface),
			},
			{
				Name:       playerIface,
				Methods:    introspect.Methods(player),
				Properties: props.Introspection(playerIface),
				Signals: []introspect.Signal{
					{Name: "Seeked", Args: []introspect.Arg{{Name: "Position", Type: "x"}}},
				},
			},
		},
	}
	if err := conn.Export(introspect.NewIntrospectable(node), objectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return nil, err
	}

	reply, err := conn.RequestName(busNamePrefix+appID, dbus.NameFlagDoNotQueue)
	if err != nil {
		return nil, err
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		// Bus name taken — still functional, just won't be the primary player.
	}

	return s, nil
}

// Close releases the bus name and cleans up.
func (s *Service) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _ = s.conn.ReleaseName(busNamePrefix + s.appID)
	return s.conn.Close()
}

// Refresh pushes the current player state onto the bus, emitting
// PropertiesChanged so GNOME Shell updates.
func (s *Service) Refresh() {
	if s == nil || s.props == nil || s.ctrl == nil {
		return
	}

	status := mpris.PlaybackStatusStopped
	switch {
	case s.ctrl.IsPlaying():
		status = mpris.PlaybackStatusPlaying
	case s.ctrl.IsPaused():
		status = mpris.PlaybackStatusPaused
	}

	meta := mpris.Metadata{
		"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/Multiplex/CurrentTrack")),
		"mpris:length":  dbus.MakeVariant(s.ctrl.Duration().Microseconds()),
		"xesam:title":   dbus.MakeVariant(s.ctrl.Title()),
	}

	s.props.SetMust(playerIface, "PlaybackStatus", string(status))
	s.props.SetMust(playerIface, "Metadata", meta)
	s.props.SetMust(playerIface, "Position", s.ctrl.Position().Microseconds())
}

// EmitSeeked publishes a Seeked signal with the new position (used when the
// app seeks internally so consumers can re-sync their position display).
func (s *Service) EmitSeeked(pos time.Duration) {
	if s == nil || s.conn == nil {
		return
	}
	_ = s.conn.Emit(objectPath, playerIface+".Seeked", pos.Microseconds())
}

func readOnly(v interface{}) *prop.Prop {
	return &prop.Prop{
		Value:    v,
		Writable: false,
		Emit:     prop.EmitTrue,
	}
}

// --- D-Bus method exports ---

type rootHandler struct {
	s            *Service
	desktopEntry string
}

func (r *rootHandler) Raise() *dbus.Error {
	r.s.ctrl.Raise()
	return nil
}

func (r *rootHandler) Quit() *dbus.Error {
	r.s.ctrl.Quit()
	return nil
}

type playerHandler struct{ s *Service }

func (p *playerHandler) Play() *dbus.Error      { p.s.ctrl.Play(); return nil }
func (p *playerHandler) Pause() *dbus.Error     { p.s.ctrl.Pause(); return nil }
func (p *playerHandler) PlayPause() *dbus.Error { p.s.ctrl.PlayPause(); return nil }
func (p *playerHandler) Stop() *dbus.Error      { p.s.ctrl.Stop(); return nil }

// Next/Previous are part of the spec but have no meaning for us — MPRIS
// requires them to exist; we return without erroring so media keys don't
// blow up.
func (p *playerHandler) Next() *dbus.Error     { return nil }
func (p *playerHandler) Previous() *dbus.Error { return nil }

// Seek is called by D-Bus with a microsecond offset (positive or negative).
func (p *playerHandler) Seek(offsetUsec int64) *dbus.Error {
	p.s.ctrl.Seek(time.Duration(offsetUsec) * time.Microsecond)
	return nil
}

// SetPosition is called with (trackId, position-microseconds). We accept any
// trackId since we only have one track.
func (p *playerHandler) SetPosition(_ dbus.ObjectPath, posUsec int64) *dbus.Error {
	p.s.ctrl.SetPosition(time.Duration(posUsec) * time.Microsecond)
	return nil
}

// OpenUri is not supported; the app does its own content acquisition.
func (p *playerHandler) OpenUri(_ string) *dbus.Error { return nil }
