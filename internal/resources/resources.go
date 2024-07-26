package resources

import (
	_ "embed"
	"path"
)

//go:generate glib-compile-schemas .
//go:embed gschemas.compiled
var GSchema []byte

const GAppID = "com.pojtinger.felicitas.Multiplex"

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

const gResourceAppPath = "/com/pojtinger/felicitas/Multiplex/"

//go:generate blueprint-compiler compile --output assistant.ui assistant.blp
var GResourceAssistantPath = path.Join(gResourceAppPath, "assistant.ui")

//go:generate blueprint-compiler compile --output controls.ui controls.blp
var GResourceControlsPath = path.Join(gResourceAppPath, "controls.ui")

//go:generate blueprint-compiler compile --output description.ui description.blp
var GResourceDescriptionPath = path.Join(gResourceAppPath, "description.ui")

//go:generate blueprint-compiler compile --output menu.ui menu.blp
var GResourceMenuPath = path.Join(gResourceAppPath, "menu.ui")

//go:generate blueprint-compiler compile --output preferences.ui preferences.blp
var GResourcePreferencesPath = path.Join(gResourceAppPath, "preferences.ui")

//go:generate blueprint-compiler compile --output preparing.ui preparing.blp
var GResourcePreparingPath = path.Join(gResourceAppPath, "preparing.ui")

//go:generate blueprint-compiler compile --output subtitles.ui subtitles.blp
var GResourceSubtitlesPath = path.Join(gResourceAppPath, "subtitles.ui")

//go:generate blueprint-compiler compile --output audiotracks.ui audiotracks.blp
var GResourceAudiotracksPath = path.Join(gResourceAppPath, "audiotracks.ui")

var GResourceErrorPath = path.Join(gResourceAppPath, "error.ui")
var GResourceWarningPath = path.Join(gResourceAppPath, "warning.ui")

var GResourceStyleCSSPath = path.Join(gResourceAppPath, "style.css")
var GResourceMetainfoPath = path.Join(gResourceAppPath, "com.pojtinger.felicitas.Multiplex.metainfo.xml")

//go:generate glib-compile-resources com.pojtinger.felicitas.Multiplex.gresource.xml
//go:embed com.pojtinger.felicitas.Multiplex.gresource
var GResource []byte
