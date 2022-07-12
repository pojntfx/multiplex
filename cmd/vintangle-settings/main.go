package main

//go:generate glib-compile-schemas .

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gio/v2"

	_ "embed"
)

const (
	schemaDirEnvVar = "GSETTINGS_SCHEMA_DIR"

	stateID = "com.pojtinger.felicitas.vintangle.state"

	verboseFlag = "verbose"
	storageFlag = "storage"
	mpvFlag     = "mpv"
)

var (
	//go:embed gschemas.compiled
	geschemas []byte
)

func main() {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "vintangle-gschemas")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "gschemas.compiled"), geschemas, os.ModePerm); err != nil {
		panic(err)
	}

	if err := os.Setenv(schemaDirEnvVar, tmpDir); err != nil {
		panic(err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	settings := gio.NewSettings(stateID)

	fmt.Printf("verbose = %v storage = %v mpv = %v\n", settings.Int64(verboseFlag), settings.String(storageFlag), settings.String(mpvFlag))

	settings.SetInt64(verboseFlag, 5)
	settings.SetString(storageFlag, filepath.Join(home, ".local", "share", "htorrent", "var", "lib", "htorrent", "data"))
	settings.SetString(mpvFlag, "mpv2")

	fmt.Printf("verbose = %v storage = %v mpv = %v\n", settings.Int64(verboseFlag), settings.String(storageFlag), settings.String(mpvFlag))

	gio.SettingsSync()
}
