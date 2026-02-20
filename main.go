package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/phayes/freeport"
	"github.com/pojntfx/go-gettext/pkg/i18n"
	v1 "github.com/pojntfx/htorrent/pkg/api/http/v1"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/assets/resources"
	"github.com/pojntfx/multiplex/internal/components"
	"github.com/pojntfx/multiplex/internal/crypto"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

//go:generate sh -c "if [ -z \"$FLATPAK_ID\" ]; then go tool github.com/dennwc/flatpak-go-mod --json .; fi"

const (
	gettextPackage = "multiplex"
)

var (
	LocaleDir = "/usr/share/locale"
	SchemaDir = ""
)

func init() {
	if err := i18n.InitI18n(gettextPackage, LocaleDir, slog.Default()); err != nil {
		panic(err)
	}

	gresources, err := gio.NewResourceFromData(glib.NewBytes(resources.ResourceContents, uint(len(resources.ResourceContents))))
	if err != nil {
		panic(err)
	}
	gio.ResourcesRegister(gresources)
}

func main() {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "multiplex")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	var settings gio.Settings
	if SchemaDir == "" {
		settings = *gio.NewSettings(resources.AppID)
	} else {
		source, err := gio.NewSettingsSchemaSourceFromDirectory(SchemaDir, gio.SettingsSchemaSourceGetDefault(), true)
		if err != nil {
			panic(err)
		}

		schema := source.Lookup(resources.AppID, false)
		if schema == nil {
			panic(errors.New("could not find schema"))
		}

		settings = *gio.NewSettingsFull(schema, nil, schema.GetPath())
	}

	if storage := settings.GetString(resources.SchemaStorageKey); strings.TrimSpace(storage) == "" {
		downloadPath := glib.GetUserSpecialDir(glib.GUserDirectoryDownloadValue)
		if downloadPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				panic(err)
			}

			downloadPath = filepath.Join(home, "Downloads")
		}

		settings.SetString(resources.SchemaStorageKey, downloadPath)

		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			panic(err)
		}

		settings.Apply()
	}

	configureZerolog := func(verbose int64) {
		switch verbose {
		case 0:
			zerolog.SetGlobalLevel(zerolog.Disabled)
		case 1:
			zerolog.SetGlobalLevel(zerolog.PanicLevel)
		case 2:
			zerolog.SetGlobalLevel(zerolog.FatalLevel)
		case 3:
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)
		case 4:
			zerolog.SetGlobalLevel(zerolog.WarnLevel)
		case 5:
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		case 6:
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		default:
			zerolog.SetGlobalLevel(zerolog.TraceLevel)
		}
	}

	configureZerolog(settings.GetInt64(resources.SchemaVerboseKey))
	changedCallback := func(s gio.Settings, key string) {
		if key == resources.SchemaVerboseKey {
			configureZerolog(settings.GetInt64(resources.SchemaVerboseKey))
		}
	}
	settings.ConnectChanged(&changedCallback)

	app := adw.NewApplication(resources.AppID, gio.GApplicationNonUniqueValue)

	prov := gtk.NewCssProvider()
	prov.LoadFromResource(resources.ResourceStyleCSSPath)

	var gateway *server.Gateway
	ctx, cancel := context.WithCancel(context.Background())

	activateCallback := func(_ gio.Application) {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			uint32(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION),
		)

		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			panic(err)
		}

		port, err := freeport.GetFreePort()
		if err != nil {
			panic(err)
		}
		addr.Port = port

		if err := os.MkdirAll(settings.GetString(resources.SchemaStorageKey), os.ModePerm); err != nil {
			panic(err)
		}

		apiAddr := settings.GetString(resources.SchemaGatewayURLKey)
		apiUsername := settings.GetString(resources.SchemaGatewayUsernameKey)
		apiPassword := settings.GetString(resources.SchemaGatewayPasswordKey)
		if !settings.GetBoolean(resources.SchemaGatewayRemoteKey) {
			apiUsername = crypto.RandomString(20)
			apiPassword = crypto.RandomString(20)

			gateway = server.NewGateway(
				addr.String(),
				settings.GetString(resources.SchemaStorageKey),
				apiUsername,
				apiPassword,
				"",
				"",
				settings.GetInt64(resources.SchemaVerboseKey) > 5,
				func(torrentMetrics v1.TorrentMetrics, fileMetrics v1.FileMetrics) {
					log.Info().
						Str("magnet", torrentMetrics.Magnet).
						Int("peers", torrentMetrics.Peers).
						Str("path", fileMetrics.Path).
						Int64("length", fileMetrics.Length).
						Int64("completed", fileMetrics.Completed).
						Msg("Streaming")
				},
				ctx,
			)

			if err := gateway.Open(); err != nil {
				panic(err)
			}

			go func() {
				log.Info().
					Str("address", addr.String()).
					Msg("Gateway listening")

				if err := gateway.Wait(); err != nil {
					panic(err)
				}
			}()

			apiAddr = "http://" + addr.String()
		}

		manager := client.NewManager(
			apiAddr,
			apiUsername,
			apiPassword,
			ctx,
		)

		mainWindow := components.NewMainWindow(ctx, app, manager, apiAddr, apiUsername, apiPassword, &settings, gateway, cancel, tmpDir)

		app.AddWindow(&mainWindow.ApplicationWindow.Window)
		mainWindow.SetVisible(true)
	}
	app.ConnectActivate(&activateCallback)

	shutdownCallback := func(_ gio.Application) {
		cancel()

		if gateway != nil {
			if err := gateway.Close(); err != nil {
				panic(err)
			}
		}
	}
	app.ConnectShutdown(&shutdownCallback)

	if code := app.Run(int32(len(os.Args)), os.Args); code > 0 {
		os.Exit(int(code))
	}
}
