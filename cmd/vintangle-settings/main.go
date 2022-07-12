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

	settings := gio.NewSettings("com.pojtinger.felicitas.vintangle.state")

	fmt.Println(settings.Int64("verbose"))

	settings.SetInt64("verbose", 10)

	fmt.Println(settings.Int64("verbose"))

	gio.SettingsSync()
}
