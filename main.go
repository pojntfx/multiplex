package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
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

const (
	schemaDirEnvVar = "GSETTINGS_SCHEMA_DIR"
)

func main() {
	gresources, err := gio.NewResourceFromData(glib.NewBytesWithGo(resources.GResource))
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

	configureZerolog(settings.Int64(resources.GSchemaVerboseKey))
	settings.ConnectChanged(func(key string) {
		if key == resources.GSchemaVerboseKey {
			configureZerolog(settings.Int64(resources.GSchemaVerboseKey))
		}
	})

	app := adw.NewApplication(resources.GAppID, gio.ApplicationNonUnique)

	prov := gtk.NewCSSProvider()
	prov.LoadFromResource(resources.GResourceStyleCSSPath)

	var gateway *server.Gateway
	ctx, cancel := context.WithCancel(context.Background())

	app.ConnectActivate(func() {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
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

		if err := os.MkdirAll(settings.String(resources.GSchemaStorageKey), os.ModePerm); err != nil {
			panic(err)
		}

		apiAddr := settings.String(resources.GSchemaGatewayURLKey)
		apiUsername := settings.String(resources.GSchemaGatewayUsernameKey)
		apiPassword := settings.String(resources.GSchemaGatewayPasswordKey)
		if !settings.Boolean(resources.GSchemaGatewayRemoteKey) {
			apiUsername = crypto.RandomString(20)
			apiPassword = crypto.RandomString(20)

			gateway = server.NewGateway(
				addr.String(),
				settings.String(resources.GSchemaStorageKey),
				apiUsername,
				apiPassword,
				"",
				"",
				settings.Int64(resources.GSchemaVerboseKey) > 5,
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
	})

	app.ConnectShutdown(func() {
		cancel()

		if gateway != nil {
			if err := gateway.Close(); err != nil {
				panic(err)
			}
		}
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
