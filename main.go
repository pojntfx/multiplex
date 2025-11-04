package main

import (
	"context"
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
	v1 "github.com/pojntfx/htorrent/pkg/api/http/v1"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/internal/components"
	"github.com/pojntfx/multiplex/internal/crypto"
	"github.com/pojntfx/multiplex/internal/resources"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

//go:generate sh -c "if [ -z \"$FLATPAK_ID\" ]; then go tool github.com/dennwc/flatpak-go-mod --json .; fi"

const (
	schemaDirEnvVar = "GSETTINGS_SCHEMA_DIR"
)

func main() {
	gresources, err := gio.NewResourceFromData(glib.NewBytes(resources.GResource, uint(len(resources.GResource))))
	if err != nil {
		panic(err)
	}
	gio.ResourcesRegister(gresources)

	tmpDir, err := os.MkdirTemp(os.TempDir(), "multiplex-gschemas")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "gschemas.compiled"), resources.GSchema, os.ModePerm); err != nil {
		panic(err)
	}

	if err := os.Setenv(schemaDirEnvVar, tmpDir); err != nil {
		panic(err)
	}

	settings := gio.NewSettings(resources.GAppID)

	if storage := settings.GetString(resources.GSchemaStorageKey); strings.TrimSpace(storage) == "" {
		downloadPath := glib.GetUserSpecialDir(glib.GUserDirectoryDownloadValue)
		if downloadPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				panic(err)
			}

			downloadPath = filepath.Join(home, "Downloads")
		}

		settings.SetString(resources.GSchemaStorageKey, downloadPath)

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

	configureZerolog(settings.GetInt64(resources.GSchemaVerboseKey))
	changedCallback := func(s gio.Settings, key string) {
		if key == resources.GSchemaVerboseKey {
			configureZerolog(settings.GetInt64(resources.GSchemaVerboseKey))
		}
	}
	settings.ConnectChanged(&changedCallback)

	app := adw.NewApplication(resources.GAppID, gio.GApplicationNonUniqueValue)

	prov := gtk.NewCssProvider()
	prov.LoadFromResource(resources.GResourceStyleCSSPath)

	var gateway *server.Gateway
	ctx, cancel := context.WithCancel(context.Background())

	activateCallback := func(_ gio.Application) {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION),
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

		if err := os.MkdirAll(settings.GetString(resources.GSchemaStorageKey), os.ModePerm); err != nil {
			panic(err)
		}

		apiAddr := settings.GetString(resources.GSchemaGatewayURLKey)
		apiUsername := settings.GetString(resources.GSchemaGatewayUsernameKey)
		apiPassword := settings.GetString(resources.GSchemaGatewayPasswordKey)
		if !settings.GetBoolean(resources.GSchemaGatewayRemoteKey) {
			apiUsername = crypto.RandomString(20)
			apiPassword = crypto.RandomString(20)

			gateway = server.NewGateway(
				addr.String(),
				settings.GetString(resources.GSchemaStorageKey),
				apiUsername,
				apiPassword,
				"",
				"",
				settings.GetInt64(resources.GSchemaVerboseKey) > 5,
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

		if err := components.OpenAssistantWindow(ctx, app, manager, apiAddr, apiUsername, apiPassword, settings, gateway, cancel, tmpDir); err != nil {
			panic(err)
		}
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

	if code := app.Run(len(os.Args), os.Args); code > 0 {
		os.Exit(code)
	}
}
