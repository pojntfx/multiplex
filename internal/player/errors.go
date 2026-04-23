package player

import "errors"

var (
	errGtk4PaintableSink = errors.New("failed to create gtk4paintablesink; install gstreamer1-plugins-gtk4 or equivalent")
	errPlaybin           = errors.New("failed to create playbin element")
)
