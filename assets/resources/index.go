package resources

import (
	_ "embed"
	"path"
	"strings"

	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
)

//go:generate sh -c "find ../../po -name '*.po' | sed 's|^\\../../po/||; s|\\.po$||' > ../../po/LINGUAS"
//go:generate sh -c "msgfmt --desktop --template ../../assets/meta/com.pojtinger.felicitas.Multiplex.desktop.in -d ../../po -o - -f | sed 's|/LC_MESSAGES/default||g' > ../../assets/meta/com.pojtinger.felicitas.Multiplex.desktop"
//go:generate sh -c "msgfmt --xml -L metainfo --template ../../assets/resources/metainfo.xml.in -d ../../po -o - -f | sed 's|/LC[-_]MESSAGES/default||g' > ../../assets/resources/metainfo.xml"

const (
	AppID      = "com.pojtinger.felicitas.Multiplex"
	AppVersion = "0.1.11"
)

//go:generate sh -c "blueprint-compiler batch-compile . . *.blp && glib-compile-resources *.gresource.xml"
//go:embed index.gresource
var ResourceContents []byte

var (
	AppPath = path.Join("/com", "pojtinger", "felicitas", "Multiplex")

	AppDevelopers = []string{"Felicitas Pojtinger"}
	AppArtists    = append(AppDevelopers, "Brage Fuglseth")
	AppCopyright  = "© 2026 " + strings.Join(AppDevelopers, ", ")

	ResourceAssistantPath   = path.Join(AppPath, "assistant.ui")
	ResourceControlsPath    = path.Join(AppPath, "controls.ui")
	ResourceDescriptionPath = path.Join(AppPath, "description.ui")
	ResourceMenuPath        = path.Join(AppPath, "menu.ui")
	ResourcePreferencesPath = path.Join(AppPath, "preferences.ui")
	ResourcePreparingPath   = path.Join(AppPath, "preparing.ui")
	ResourceSubtitlesPath   = path.Join(AppPath, "subtitles.ui")
	ResourceAudiotracksPath = path.Join(AppPath, "audiotracks.ui")
	ResourceErrorPath       = path.Join(AppPath, "error.ui")
	ResourceWarningPath     = path.Join(AppPath, "warning.ui")

	ResourceStyleCSSPath = path.Join(AppPath, "style.css")
	ResourceMetainfoPath = path.Join(AppPath, "metainfo.xml")
)

//go:generate glib-compile-schemas .
//go:embed gschemas.compiled
var Schema []byte

const (
	SchemaVerboseKey = "verbose"
	SchemaStorageKey = "storage"
	SchemaMPVKey     = "mpv"

	SchemaGatewayRemoteKey   = "gatewayremote"
	SchemaGatewayURLKey      = "gatewayurl"
	SchemaGatewayUsernameKey = "gatewayusername"
	SchemaGatewayPasswordKey = "gatewaypassword"

	SchemaWeronURLKey        = "weronurl"
	SchemaWeronTimeoutKey    = "werontimeout"
	SchemaWeronICEKey        = "weronice"
	SchemaWeronForceRelayKey = "weronforcerelay"
)

func init() {
	gresources, err := gio.NewResourceFromData(glib.NewBytes(ResourceContents, uint(len(ResourceContents))))
	if err != nil {
		panic(err)
	}
	gio.ResourcesRegister(gresources)
}
