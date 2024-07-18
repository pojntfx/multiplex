package resources

import (
	_ "embed"
	"path"
)

const GAppID = "com.pojtinger.felicitas.Multiplex"

//go:generate glib-compile-schemas .
//go:embed gschemas.compiled
var GSchema []byte

const (
	GSchemaVerboseKey = "verbose"
	GSchemaStorageKey = "storage"
	GSchemaMPVKey     = "mpv"

	GSchemaGatewayRemoteKey   = "gatewayremote"
	GSchemaGatewayURLKey      = "gatewayurl"
	GSchemaGatewayUsernameKey = "gatewayusername"
	GSchemaGatewayPasswordKey = "gatewaypassword"

	GSchemaWeronURLKey        = "weronurl"
	GSchemaWeronTimeoutKey    = "werontimeout"
	GSchemaWeronICEKey        = "weronice"
	GSchemaWeronForceRelayKey = "weronforcerelay"
)

//go:generate glib-compile-resources com.pojtinger.felicitas.Multiplex.gresource.xml
//go:embed com.pojtinger.felicitas.Multiplex.gresource
var GResource []byte

var (
	gResourceAppPath = "/com/pojtinger/felicitas/Multiplex/"

	GResourceAssistantPath   = path.Join(gResourceAppPath, "assistant.ui")
	GResourceControlsPath    = path.Join(gResourceAppPath, "controls.ui")
	GResourceDescriptionPath = path.Join(gResourceAppPath, "description.ui")
	GResourceWarningPath     = path.Join(gResourceAppPath, "warning.ui")
	GResourceErrorPath       = path.Join(gResourceAppPath, "error.ui")
	GResourceMenuPath        = path.Join(gResourceAppPath, "menu.ui")
	GResourceAboutPath       = path.Join(gResourceAppPath, "about.ui")
	GResourcePreferencesPath = path.Join(gResourceAppPath, "preferences.ui")
	GResourceSubtitlesPath   = path.Join(gResourceAppPath, "subtitles.ui")
	GResourceAudiotracksPath = path.Join(gResourceAppPath, "audiotracks.ui")
	GResourcePreparingPath   = path.Join(gResourceAppPath, "preparing.ui")
	GResourceStyleCSSPath    = path.Join(gResourceAppPath, "style.css")
)
