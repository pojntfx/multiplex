package ressources

import (
	_ "embed"
)

//go:generate glib-compile-schemas .

var (
	//go:embed assistant.ui
	AssistantUI string

	//go:embed controls.ui
	ControlsUI string

	//go:embed description.ui
	DescriptionUI string

	//go:embed warning.ui
	WarningUI string

	//go:embed error.ui
	ErrorUI string

	//go:embed menu.ui
	MenuUI string

	//go:embed about.ui
	AboutUI string

	//go:embed preferences.ui
	PreferencesUI string

	//go:embed subtitles.ui
	SubtitlesUI string

	//go:embed audiotracks.ui
	AudiotracksUI string

	//go:embed preparing.ui
	PreparingUI string

	//go:embed style.css
	StyleCSS string

	//go:embed gschemas.compiled
	Geschemas []byte
)
