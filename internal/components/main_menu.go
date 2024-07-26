package components

import (
	"context"
	"fmt"
	"math"
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/internal/resources"
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
) (*adw.PreferencesWindow, *adw.EntryRow) {
	menuBuilder := gtk.NewBuilderFromResource(resources.GResourceMenuPath)
	menu := menuBuilder.GetObject("main-menu").Cast().(*gio.Menu)

	aboutDialog := adw.NewAboutDialogFromAppdata(resources.GResourceMetainfoPath, "0.1.4")
	aboutDialog.SetDevelopers([]string{"Felicitas Pojtinger"})
	aboutDialog.SetArtists([]string{"Brage Fuglseth"})
	aboutDialog.SetCopyright("© 2024 Felicitas Pojtinger")

	preferencesBuilder := gtk.NewBuilderFromResource(resources.GResourcePreferencesPath)
	preferencesDialog := preferencesBuilder.GetObject("preferences-dialog").Cast().(*adw.PreferencesWindow)
	storageLocationInput := preferencesBuilder.GetObject("storage-location-input").Cast().(*gtk.Button)
	mpvCommandInput := preferencesBuilder.GetObject("mpv-command-input").Cast().(*adw.EntryRow)
	verbosityLevelInput := preferencesBuilder.GetObject("verbosity-level-input").Cast().(*adw.SpinRow)
	remoteGatewaySwitchInput := preferencesBuilder.GetObject("htorrent-remote-gateway-switch").Cast().(*gtk.Switch)
	remoteGatewayURLInput := preferencesBuilder.GetObject("htorrent-url-input").Cast().(*adw.EntryRow)
	remoteGatewayUsernameInput := preferencesBuilder.GetObject("htorrent-username-input").Cast().(*adw.EntryRow)
	remoteGatewayPasswordInput := preferencesBuilder.GetObject("htorrent-password-input").Cast().(*adw.PasswordEntryRow)
	weronURLInput := preferencesBuilder.GetObject("weron-url-input").Cast().(*adw.EntryRow)
	weronICEInput := preferencesBuilder.GetObject("weron-ice-input").Cast().(*adw.EntryRow)
	weronTimeoutInput := preferencesBuilder.GetObject("weron-timeout-input").Cast().(*adw.SpinRow)
	weronForceRelayInput := preferencesBuilder.GetObject("weron-force-relay-input").Cast().(*gtk.Switch)

	preferencesHaveChanged := false

	preferencesAction := gio.NewSimpleAction(preferencesActionName, nil)
	preferencesAction.ConnectActivate(func(parameter *glib.Variant) {
		preferencesDialog.Present()
	})
	app.SetAccelsForAction("win."+preferencesActionName, []string{`<Primary>comma`})
	window.AddAction(preferencesAction)

	openDownloadsAction := gio.NewSimpleAction(openDownloadsActionName, nil)
	openDownloadsAction.ConnectActivate(func(parameter *glib.Variant) {
		if err := gio.AppInfoLaunchDefaultForURI(fmt.Sprintf("file://%v", settings.String(resources.GSchemaStorageKey)), nil); err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}
	})
	window.AddAction(openDownloadsAction)

	if getMagnetLink != nil {
		copyMagnetLinkAction := gio.NewSimpleAction(copyMagnetLinkActionName, nil)
		copyMagnetLinkAction.ConnectActivate(func(parameter *glib.Variant) {
			window.Clipboard().SetText(getMagnetLink())
		})
		window.AddAction(copyMagnetLinkAction)
	}

	preferencesDialog.SetTransientFor(&window.Window)
	preferencesDialog.ConnectCloseRequest(func() (ok bool) {
		preferencesDialog.Close()
		preferencesDialog.SetVisible(false)

		if preferencesHaveChanged {
			settings.Apply()

			toast := adw.NewToast("Reopen to apply the changes.")
			toast.SetButtonLabel("Reopen")
			toast.SetActionName("win." + applyPreferencesActionName)

			overlay.AddToast(toast)
		}

		preferencesHaveChanged = false

		return ok
	})

	syncSensitivityState := func() {
		if remoteGatewaySwitchInput.State() {
			remoteGatewayURLInput.SetEditable(true)
			remoteGatewayUsernameInput.SetEditable(true)
			remoteGatewayPasswordInput.SetEditable(true)
		} else {
			remoteGatewayURLInput.SetEditable(false)
			remoteGatewayUsernameInput.SetEditable(false)
			remoteGatewayPasswordInput.SetEditable(false)
		}
	}
	preferencesDialog.ConnectShow(syncSensitivityState)

	applyPreferencesAction := gio.NewSimpleAction(applyPreferencesActionName, nil)
	applyPreferencesAction.ConnectActivate(func(parameter *glib.Variant) {
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
	})
	window.AddAction(applyPreferencesAction)

	storageLocationInput.ConnectClicked(func() {
		filePicker := gtk.NewFileChooserNative(
			"Select storage location",
			&window.Window,
			gtk.FileChooserActionSelectFolder,
			"",
			"")
		filePicker.SetModal(true)
		filePicker.ConnectResponse(func(responseId int) {
			if responseId == int(gtk.ResponseAccept) {
				settings.SetString(resources.GSchemaStorageKey, filePicker.File().Path())

				preferencesHaveChanged = true
			}

			filePicker.Destroy()
		})

		filePicker.Show()
	})

	settings.Bind(resources.GSchemaMPVKey, mpvCommandInput.Object, "text", gio.SettingsBindDefault)

	verbosityLevelInput.SetAdjustment(gtk.NewAdjustment(0, 0, 8, 1, 1, 1))
	settings.Bind(resources.GSchemaVerboseKey, verbosityLevelInput.Object, "value", gio.SettingsBindDefault)

	settings.Bind(resources.GSchemaGatewayRemoteKey, remoteGatewaySwitchInput.Object, "active", gio.SettingsBindDefault)
	settings.Bind(resources.GSchemaGatewayURLKey, remoteGatewayURLInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(resources.GSchemaGatewayUsernameKey, remoteGatewayUsernameInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(resources.GSchemaGatewayPasswordKey, remoteGatewayPasswordInput.Object, "text", gio.SettingsBindDefault)

	settings.Bind(resources.GSchemaWeronURLKey, weronURLInput.Object, "text", gio.SettingsBindDefault)

	weronTimeoutInput.SetAdjustment(gtk.NewAdjustment(0, 0, math.MaxFloat64, 1, 1, 1))
	settings.Bind(resources.GSchemaWeronTimeoutKey, weronTimeoutInput.Object, "value", gio.SettingsBindDefault)

	settings.Bind(resources.GSchemaWeronICEKey, weronICEInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(resources.GSchemaWeronForceRelayKey, weronForceRelayInput.Object, "active", gio.SettingsBindDefault)

	mpvCommandInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	verbosityLevelInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})

	remoteGatewaySwitchInput.ConnectStateSet(func(state bool) (ok bool) {
		preferencesHaveChanged = true

		remoteGatewaySwitchInput.SetState(state)

		syncSensitivityState()

		return true
	})

	remoteGatewayURLInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	remoteGatewayUsernameInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	remoteGatewayPasswordInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})

	weronURLInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	weronTimeoutInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	weronICEInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})

	weronForceRelayInput.ConnectStateSet(func(state bool) (ok bool) {
		preferencesHaveChanged = true

		weronForceRelayInput.SetState(state)

		return true
	})

	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(parameter *glib.Variant) {
		aboutDialog.Present(&window.Window)
	})
	window.AddAction(aboutAction)

	menuButton.SetMenuModel(menu)

	return preferencesDialog, mpvCommandInput
}
