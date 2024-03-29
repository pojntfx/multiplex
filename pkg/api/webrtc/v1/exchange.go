package v1

// Pause synchronizes play/pause state
type Pause struct {
	Message
	Pause bool `json:"pause"` // Whether to pause or play
}

func NewPause(pause bool) *Pause {
	return &Pause{
		Message: Message{
			Type: TypePause,
		},
		Pause: pause,
	}
}

// Magnet contains a magnet link
type Magnet struct {
	Message
	Magnet      string     `json:"magnet"`      // Encapsulated magnet link
	Path        string     `json:"path"`        // Path of the media to play
	Title       string     `json:"title"`       // Title of the media to play
	Description string     `json:"description"` // Description of the media to play
	Subtitles   []Subtitle `json:"subtitles"`   // Subtitles of the media to play
}

// Subtitle describes a subtitle file
type Subtitle struct {
	Name string `json:"name"` // Name of the subtitle
	Size int    `json:"size"` // Size of the subtitle file
}

func NewMagnetLink(magnet, path, title, description string, subtitles []Subtitle) *Magnet {
	return &Magnet{
		Message: Message{
			Type: TypeMagnet,
		},
		Magnet:      magnet,
		Path:        path,
		Title:       title,
		Description: description,
		Subtitles:   subtitles,
	}
}

// Position synchronizes seek positions
type Position struct {
	Message
	Position float64 `json:"position"` // Position to seek to
}

func NewPosition(position float64) *Position {
	return &Position{
		Message: Message{
			Type: TypePosition,
		},
		Position: position,
	}
}

// Buffering synchronizes buffering state
type Buffering struct {
	Message
	Buffering bool `json:"buffering"` // Whether to show the buffering state
}

func NewBuffering(buffering bool) *Buffering {
	return &Buffering{
		Message: Message{
			Type: TypeBuffering,
		},
		Buffering: buffering,
	}
}
