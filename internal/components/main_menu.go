package components

import (
	"context"
	"fmt"
	"os"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	. "github.com/pojntfx/go-gettext/pkg/i18n"
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
	menuBuilder.GetObject("main_menu").Cast(&menu)
	defer menu.Unref()

	aboutDialog := adw.NewAboutDialogFromAppdata(resources.ResourceMetainfoPath, resources.AppVersion)
	aboutDialog.SetDevelopers(resources.AppDevelopers)
	aboutDialog.SetArtists(resources.AppArtists)
	aboutDialog.SetCopyright(resources.AppCopyright)
	// TRANSLATORS: Replace "translator-credits" with your name/username, and optionally an email or URL.
	aboutDialog.SetTranslatorCredits(L("translator-credits"))

	preferencesDialog := NewPreferencesDialog(ctx, settings, window, overlay, gateway, cancel)

	preferencesAction := gio.NewSimpleAction(preferencesActionName, nil)
	onPreferences := func(action gio.SimpleAction, parameter uintptr) {
		preferencesDialog.Present()
	}
	preferencesAction.ConnectActivate(&onPreferences)
	app.SetAccelsForAction("win."+preferencesActionName, []string{`<Primary>comma`})
	window.AddAction(preferencesAction)

	openDownloadsAction := gio.NewSimpleAction(openDownloadsActionName, nil)
	onOpenDownloads := func(action gio.SimpleAction, parameter uintptr) {
		_, err := gio.AppInfoLaunchDefaultForUri(fmt.Sprintf("file://%v", settings.GetString(resources.SchemaStorageKey)), nil)
		if err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}
	}
	openDownloadsAction.ConnectActivate(&onOpenDownloads)
	window.AddAction(openDownloadsAction)

	if getMagnetLink != nil {
		copyMagnetLinkAction := gio.NewSimpleAction(copyMagnetLinkActionName, nil)
		onCopyMagnetLink := func(action gio.SimpleAction, parameter uintptr) {
			window.GetClipboard().SetText(getMagnetLink())
		}
		copyMagnetLinkAction.ConnectActivate(&onCopyMagnetLink)
		window.AddAction(copyMagnetLinkAction)
	}

	applyPreferencesAction := gio.NewSimpleAction(applyPreferencesActionName, nil)
	onApplyPreferences := func(action gio.SimpleAction, parameter uintptr) {
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
	applyPreferencesAction.ConnectActivate(&onApplyPreferences)
	window.AddAction(applyPreferencesAction)

	aboutAction := gio.NewSimpleAction("about", nil)
	onAbout := func(action gio.SimpleAction, parameter uintptr) {
		aboutDialog.Present(&window.Window.Widget)
	}
	aboutAction.ConnectActivate(&onAbout)
	window.AddAction(aboutAction)

	shortcutsWindow := NewShortcutsWindow(window, app)

	shortcutsAction := gio.NewSimpleAction("shortcuts", nil)
	onShortcuts := func(action gio.SimpleAction, parameter uintptr) {
		shortcutsWindow.Present()
	}
	shortcutsAction.ConnectActivate(&onShortcuts)
	window.AddAction(shortcutsAction)
	app.SetAccelsForAction("win.shortcuts", []string{`<Primary>question`})

	closeWindowAction := gio.NewSimpleAction("closeWindow", nil)
	onCloseWindow := func(action gio.SimpleAction, parameter uintptr) {
		window.ApplicationWindow.Close()
	}
	closeWindowAction.ConnectActivate(&onCloseWindow)
	window.AddAction(closeWindowAction)
	app.SetAccelsForAction("win.closeWindow", []string{`<Primary>w`})

	quitAction := gio.NewSimpleAction("quit", nil)
	onQuit := func(action gio.SimpleAction, parameter uintptr) {
		app.Application.Quit()
	}
	quitAction.ConnectActivate(&onQuit)
	app.AddAction(quitAction)
	app.SetAccelsForAction("app.quit", []string{`<Primary>q`})

	menuButton.SetMenuModel(&menu.MenuModel)

	return preferencesDialog, preferencesDialog.MpvCommandInput()
}
