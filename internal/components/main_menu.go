package components

import (
	"context"
	"fmt"
	"os"

	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/assets/resources"
	"github.com/pojntfx/multiplex/internal/utils"
)

func AddMainMenu(
	ctx context.Context,
	app *adw.Application,
	window *adw.ApplicationWindow,

	settings *gio.Settings,

	menuButton *gtk.MenuButton,
	overlay *adw.ToastOverlay,
	gateway *server.Gateway,
	getMagnetLink func() string,
	cancel func(),
) (*PreferencesDialog, *adw.EntryRow) {
	menuBuilder := gtk.NewBuilderFromResource(resources.ResourceMenuPath)
	defer menuBuilder.Unref()
	var menu gio.Menu
	menuBuilder.GetObject("main-menu").Cast(&menu)
	defer menu.Unref()

	aboutDialog := adw.NewAboutDialogFromAppdata(resources.ResourceMetainfoPath, resources.AppVersion)
	aboutDialog.SetDevelopers(resources.AppDevelopers)
	aboutDialog.SetArtists(resources.AppArtists)
	aboutDialog.SetCopyright(resources.AppCopyright)

	preferencesDialog := NewPreferencesDialog(ctx, settings, window, overlay, gateway, cancel)

	preferencesAction := gio.NewSimpleAction(preferencesActionName, nil)
	preferencesCallback := func(action gio.SimpleAction, parameter uintptr) {
		preferencesDialog.Present()
	}
	preferencesAction.ConnectActivate(&preferencesCallback)
	app.SetAccelsForAction("win."+preferencesActionName, []string{`<Primary>comma`})
	window.AddAction(preferencesAction)

	openDownloadsAction := gio.NewSimpleAction(openDownloadsActionName, nil)
	openDownloadsCallback := func(action gio.SimpleAction, parameter uintptr) {
		_, err := gio.AppInfoLaunchDefaultForUri(fmt.Sprintf("file://%v", settings.GetString(resources.SchemaStorageKey)), nil)
		if err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}
	}
	openDownloadsAction.ConnectActivate(&openDownloadsCallback)
	window.AddAction(openDownloadsAction)

	if getMagnetLink != nil {
		copyMagnetLinkAction := gio.NewSimpleAction(copyMagnetLinkActionName, nil)
		copyMagnetLinkCallback := func(action gio.SimpleAction, parameter uintptr) {
			window.GetClipboard().SetText(getMagnetLink())
		}
		copyMagnetLinkAction.ConnectActivate(&copyMagnetLinkCallback)
		window.AddAction(copyMagnetLinkAction)
	}

	applyPreferencesAction := gio.NewSimpleAction(applyPreferencesActionName, nil)
	applyPreferencesCallback := func(action gio.SimpleAction, parameter uintptr) {
		cancel()

		if gateway != nil {
			if err := gateway.Close(); err != nil {
				OpenErrorDialog(ctx, window, err)

				return
			}
		}

		ex, err := os.Executable()
		if err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}

		if err := utils.ForkExec(ex, os.Args); err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}

		os.Exit(0)
	}
	applyPreferencesAction.ConnectActivate(&applyPreferencesCallback)
	window.AddAction(applyPreferencesAction)

	aboutAction := gio.NewSimpleAction("about", nil)
	aboutCallback := func(action gio.SimpleAction, parameter uintptr) {
		aboutDialog.Present(&window.Window.Widget)
	}
	aboutAction.ConnectActivate(&aboutCallback)
	window.AddAction(aboutAction)

	menuButton.SetMenuModel(&menu.MenuModel)

	return preferencesDialog, preferencesDialog.MpvCommandInput()
}
