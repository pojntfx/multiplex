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
	Magnet string `json:"magnet"` // Encapsulated magnet link
	Path   string `json:"path"`   // Path of the media to play
}

func NewMagnetLink(magnet string, path string) *Magnet {
	return &Magnet{
		Message: Message{
			Type: TypeMagnet,
		},
		Magnet: magnet,
		Path:   path,
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
