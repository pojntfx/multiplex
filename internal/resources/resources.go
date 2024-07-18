package resources

import (
	_ "embed"
)

//go:generate glib-compile-schemas .
//go:generate glib-compile-resources com.pojtinger.felicitas.Multiplex.gresource.xml

//go:embed com.pojtinger.felicitas.Multiplex.gresource
var Resources []byte

const (
	AppID   = "com.pojtinger.felicitas.Multiplex"
	AppPath = "/com/pojtinger/felicitas/Multiplex/"
)

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
