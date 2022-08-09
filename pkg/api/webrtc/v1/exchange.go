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
