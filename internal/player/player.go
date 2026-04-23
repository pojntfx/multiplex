package player

import (
	"sync"
	"time"

	"codeberg.org/puregotk/puregotk/examples/gstreamer-go/gst"
	"codeberg.org/puregotk/puregotk/v4/gdk"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
)

const (
	playbinFlagVideo     = 0x00000001
	playbinFlagAudio     = 0x00000002
	playbinFlagText      = 0x00000004
	playbinFlagSoftVol   = 0x00000010
	playbinFlagNativeAud = 0x00000020
	playbinFlagNativeVid = 0x00000040
)

type Event int

const (
	EventBuffering Event = iota
	EventBufferingDone
	EventEOS
	EventError
	EventWarning
	EventInfo
	EventStateChanged
	EventDurationChanged
)

type Callback func(ev Event, data interface{})

type Player struct {
	picture *gtk.Picture

	mu       sync.Mutex
	pipeline *gst.Element
	uri      string
	suburi   string

	callback Callback
}

func New(picture *gtk.Picture) *Player {
	return &Player{picture: picture}
}

func (p *Player) SetCallback(cb Callback) {
	p.callback = cb
}

func (p *Player) Load(uri string, subtitleURI string) error {
	p.Stop()

	p.mu.Lock()
	defer p.mu.Unlock()

	p.uri = uri
	p.suburi = subtitleURI

	videoSink := gst.ElementFactoryMake("gtk4paintablesink", "videosink")
	if videoSink == nil {
		return errGtk4PaintableSink
	}

	var sinkObj gobject.Object
	sinkObj.Ptr = videoSink.GoPointer()

	var paintableVal gobject.Value
	paintableVal.Init(gobject.ObjectGLibType())
	sinkObj.GetProperty("paintable", &paintableVal)
	paintableObj := paintableVal.GetObject()

	var paintable gdk.PaintableBase
	paintable.Ptr = paintableObj.GoPointer()
	p.picture.SetPaintable(&paintable)

	pipeline := gst.ElementFactoryMake("playbin", "player")
	if pipeline == nil {
		return errPlaybin
	}

	var pipelineObj gobject.Object
	pipelineObj.Ptr = pipeline.GoPointer()

	var uriVal gobject.Value
	uriVal.Init(gobject.TypeStringVal)
	uriVal.SetString(uri)
	pipelineObj.SetProperty("uri", &uriVal)

	if subtitleURI != "" {
		var subVal gobject.Value
		subVal.Init(gobject.TypeStringVal)
		subVal.SetString(subtitleURI)
		pipelineObj.SetProperty("suburi", &subVal)
	}

	var videoSinkVal gobject.Value
	videoSinkVal.Init(gobject.ObjectGLibType())
	videoSinkVal.SetObject(&sinkObj)
	pipelineObj.SetProperty("video-sink", &videoSinkVal)

	p.pipeline = pipeline

	if bus := pipeline.GetBus(); bus != nil {
		var busWatch gst.BusFunc = func(_ uintptr, msg *gst.Message, _ uintptr) bool {
			p.dispatchMessage(msg)
			return true
		}
		bus.AddWatch(&busWatch, 0)
	}

	return nil
}

func (p *Player) dispatchMessage(msg *gst.Message) {
	switch msg.Type {
	case gst.MessageEosValue:
		if p.callback != nil {
			p.callback(EventEOS, nil)
		}
	case gst.MessageErrorValue:
		var gerr *glib.Error
		var debug string
		msg.ParseError(&gerr, &debug)
		if p.callback != nil {
			p.callback(EventError, gerr)
		}
	case gst.MessageWarningValue:
		var gerr *glib.Error
		var debug string
		msg.ParseWarning(&gerr, &debug)
		if p.callback != nil {
			p.callback(EventWarning, gerr)
		}
	case gst.MessageInfoValue:
		var gerr *glib.Error
		var debug string
		msg.ParseInfo(&gerr, &debug)
		if p.callback != nil {
			p.callback(EventInfo, gerr)
		}
	case gst.MessageBufferingValue:
		if p.callback != nil {
			var percent int32
			msg.ParseBuffering(&percent)
			if percent < 100 {
				p.callback(EventBuffering, percent)
			} else {
				p.callback(EventBufferingDone, percent)
			}
		}
	case gst.MessageStateChangedValue:
		if p.callback != nil {
			p.callback(EventStateChanged, nil)
		}
	case gst.MessageDurationChangedValue:
		if p.callback != nil {
			p.callback(EventDurationChanged, nil)
		}
	}
}

func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	p.pipeline.SetState(gst.StatePlayingValue)
}

func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	p.pipeline.SetState(gst.StatePausedValue)
}

func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline != nil {
		p.pipeline.SetState(gst.StateNullValue)
		p.pipeline = nil
	}
}

func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return false
	}
	var state, pending gst.State
	p.pipeline.GetState(&state, &pending, 0)
	return state == gst.StatePlayingValue
}

func (p *Player) State() gst.State {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return gst.StateNullValue
	}
	var state, pending gst.State
	p.pipeline.GetState(&state, &pending, 0)
	return state
}

func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return 0
	}
	var pos int64
	if !p.pipeline.QueryPosition(gst.FormatTimeValue, &pos) {
		return 0
	}
	return time.Duration(pos)
}

func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return 0
	}
	var dur int64
	if !p.pipeline.QueryDuration(gst.FormatTimeValue, &dur) {
		return 0
	}
	return time.Duration(dur)
}

func (p *Player) Seek(position time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	p.pipeline.SeekSimple(gst.FormatTimeValue, gst.SeekFlagFlushValue|gst.SeekFlagKeyUnitValue, int64(position))
}

func (p *Player) SetVolume(v float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	var pipelineObj gobject.Object
	pipelineObj.Ptr = p.pipeline.GoPointer()
	var val gobject.Value
	val.Init(gobject.TypeDoubleVal)
	val.SetDouble(v)
	pipelineObj.SetProperty("volume", &val)
}

func (p *Player) SetMute(muted bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	var pipelineObj gobject.Object
	pipelineObj.Ptr = p.pipeline.GoPointer()
	var val gobject.Value
	val.Init(gobject.TypeBooleanVal)
	val.SetBoolean(muted)
	pipelineObj.SetProperty("mute", &val)
}

func (p *Player) TrackCounts() (audio, text int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return 0, 0
	}
	var pipelineObj gobject.Object
	pipelineObj.Ptr = p.pipeline.GoPointer()

	var aVal gobject.Value
	aVal.Init(gobject.TypeIntVal)
	pipelineObj.GetProperty("n-audio", &aVal)
	audio = int(aVal.GetInt())

	var tVal gobject.Value
	tVal.Init(gobject.TypeIntVal)
	pipelineObj.GetProperty("n-text", &tVal)
	text = int(tVal.GetInt())

	return audio, text
}

func (p *Player) SetAudioTrack(index int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	p.setFlagEnabled(playbinFlagAudio, index >= 0)
	if index < 0 {
		return
	}
	var pipelineObj gobject.Object
	pipelineObj.Ptr = p.pipeline.GoPointer()
	var val gobject.Value
	val.Init(gobject.TypeIntVal)
	val.SetInt(int32(index))
	pipelineObj.SetProperty("current-audio", &val)
}

func (p *Player) SetSubtitleTrack(index int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline == nil {
		return
	}
	p.setFlagEnabled(playbinFlagText, index >= 0)
	if index < 0 {
		return
	}
	var pipelineObj gobject.Object
	pipelineObj.Ptr = p.pipeline.GoPointer()
	var val gobject.Value
	val.Init(gobject.TypeIntVal)
	val.SetInt(int32(index))
	pipelineObj.SetProperty("current-text", &val)
}

// SetSubtitleURI replaces the subtitle stream. playbin only honours its
// `suburi` at preroll time, so this tears the pipeline down and rebuilds it
// with the same URI plus the new subtitle URI, seeking back to the position
// where playback was. Safe to call from any goroutine — the reload is
// scheduled onto the GTK main loop because gtk4paintablesink requires its
// paintable to be retrieved there.
func (p *Player) SetSubtitleURI(uri string) {
	p.mu.Lock()
	mainURI := p.uri
	var pos int64
	if p.pipeline != nil {
		p.pipeline.QueryPosition(gst.FormatTimeValue, &pos)
	}
	p.mu.Unlock()

	if mainURI == "" {
		return
	}

	var onMain glib.SourceFunc = func(uintptr) bool {
		if err := p.Load(mainURI, uri); err != nil {
			return false
		}

		p.mu.Lock()
		pipeline := p.pipeline
		p.mu.Unlock()
		if pipeline == nil {
			return false
		}

		pipeline.SetState(gst.StatePausedValue)
		var cur, pending gst.State
		pipeline.GetState(&cur, &pending, gst.CLOCK_TIME_NONE)
		if pos > 0 {
			pipeline.SeekSimple(gst.FormatTimeValue, gst.SeekFlagFlushValue|gst.SeekFlagKeyUnitValue, pos)
		}
		pipeline.SetState(gst.StatePlayingValue)
		return false
	}
	glib.IdleAdd(&onMain, 0)
}

func (p *Player) setFlagEnabled(flag int, enabled bool) {
	var pipelineObj gobject.Object
	pipelineObj.Ptr = p.pipeline.GoPointer()
	var val gobject.Value
	val.Init(gobject.TypeIntVal)
	pipelineObj.GetProperty("flags", &val)
	flags := int(val.GetInt())
	if enabled {
		flags |= flag
	} else {
		flags &^= flag
	}
	val.SetInt(int32(flags))
	pipelineObj.SetProperty("flags", &val)
}
