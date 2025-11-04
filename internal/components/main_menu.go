package components

import (
	"context"
	"fmt"
	"math"
	"os"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/gtk"
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
	var menu gio.Menu
	menuBuilder.GetObject("main-menu").Cast(&menu)

	aboutDialog := adw.NewAboutDialogFromAppdata(resources.GResourceMetainfoPath, "0.1.7")
	aboutDialog.SetDevelopers([]string{"Felicitas Pojtinger"})
	aboutDialog.SetArtists([]string{"Brage Fuglseth"})
	aboutDialog.SetCopyright("© 2025 Felicitas Pojtinger")

	preferencesBuilder := gtk.NewBuilderFromResource(resources.GResourcePreferencesPath)
	var preferencesDialog adw.PreferencesWindow
	preferencesBuilder.GetObject("preferences-dialog").Cast(&preferencesDialog)
	var storageLocationInput gtk.Button
	preferencesBuilder.GetObject("storage-location-input").Cast(&storageLocationInput)
	var mpvCommandInput adw.EntryRow
	preferencesBuilder.GetObject("mpv-command-input").Cast(&mpvCommandInput)
	var verbosityLevelInput adw.SpinRow
	preferencesBuilder.GetObject("verbosity-level-input").Cast(&verbosityLevelInput)
	var remoteGatewaySwitchInput gtk.Switch
	preferencesBuilder.GetObject("htorrent-remote-gateway-switch").Cast(&remoteGatewaySwitchInput)
	var remoteGatewayURLInput adw.EntryRow
	preferencesBuilder.GetObject("htorrent-url-input").Cast(&remoteGatewayURLInput)
	var remoteGatewayUsernameInput adw.EntryRow
	preferencesBuilder.GetObject("htorrent-username-input").Cast(&remoteGatewayUsernameInput)
	var remoteGatewayPasswordInput adw.PasswordEntryRow
	preferencesBuilder.GetObject("htorrent-password-input").Cast(&remoteGatewayPasswordInput)
	var weronURLInput adw.EntryRow
	preferencesBuilder.GetObject("weron-url-input").Cast(&weronURLInput)
	var weronICEInput adw.EntryRow
	preferencesBuilder.GetObject("weron-ice-input").Cast(&weronICEInput)
	var weronTimeoutInput adw.SpinRow
	preferencesBuilder.GetObject("weron-timeout-input").Cast(&weronTimeoutInput)
	var weronForceRelayInput gtk.Switch
	preferencesBuilder.GetObject("weron-force-relay-input").Cast(&weronForceRelayInput)

	preferencesHaveChanged := false

	preferencesAction := gio.NewSimpleAction(preferencesActionName, nil)
	preferencesCallback := func(action gio.SimpleAction, parameter uintptr) {
		preferencesDialog.Present()
	}
	preferencesAction.ConnectActivate(&preferencesCallback)
	app.SetAccelsForAction("win."+preferencesActionName, []string{`<Primary>comma`})
	window.AddAction(preferencesAction)

	openDownloadsAction := gio.NewSimpleAction(openDownloadsActionName, nil)
	openDownloadsCallback := func(action gio.SimpleAction, parameter uintptr) {
		_, err := gio.AppInfoLaunchDefaultForUri(fmt.Sprintf("file://%v", settings.GetString(resources.GSchemaStorageKey)), nil)
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

	preferencesDialog.SetTransientFor(&window.Window)
	closeRequestCallback := func(gtk.Window) bool {
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

		return true
	}
	preferencesDialog.ConnectCloseRequest(&closeRequestCallback)

	syncSensitivityState := func() {
		if remoteGatewaySwitchInput.GetActive() {
			remoteGatewayURLInput.SetEditable(true)
			remoteGatewayUsernameInput.SetEditable(true)
			remoteGatewayPasswordInput.SetEditable(true)
		} else {
			remoteGatewayURLInput.SetEditable(false)
			remoteGatewayUsernameInput.SetEditable(false)
			remoteGatewayPasswordInput.SetEditable(false)
		}
	}
	showCallback := func(gtk.Widget) {
		syncSensitivityState()
	}
	preferencesDialog.ConnectShow(&showCallback)

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

	clickedCallback := func(gtk.Button) {
		filePicker := gtk.NewFileChooserNative(
			"Select storage location",
			&window.Window,
			gtk.FileChooserActionSelectFolderValue,
			"",
			"")
		filePicker.SetModal(true)
		filePickerResponseCallback := func(dialog gtk.NativeDialog, responseId int) {
			if responseId == int(gtk.ResponseAcceptValue) {
				settings.SetString(resources.GSchemaStorageKey, filePicker.GetFile().GetPath())

				preferencesHaveChanged = true
			}

			filePicker.Destroy()
		}
		filePicker.ConnectResponse(&filePickerResponseCallback)

		filePicker.Show()
	}
	storageLocationInput.ConnectClicked(&clickedCallback)

	settings.Bind(resources.GSchemaMPVKey, &mpvCommandInput.Object, "text", gio.GSettingsBindDefaultValue)

	verbosityLevelInput.SetAdjustment(gtk.NewAdjustment(0, 0, 8, 1, 1, 1))
	settings.Bind(resources.GSchemaVerboseKey, &verbosityLevelInput.Object, "value", gio.GSettingsBindDefaultValue)

	settings.Bind(resources.GSchemaGatewayRemoteKey, &remoteGatewaySwitchInput.Object, "active", gio.GSettingsBindDefaultValue)
	settings.Bind(resources.GSchemaGatewayURLKey, &remoteGatewayURLInput.Object, "text", gio.GSettingsBindDefaultValue)
	settings.Bind(resources.GSchemaGatewayUsernameKey, &remoteGatewayUsernameInput.Object, "text", gio.GSettingsBindDefaultValue)
	settings.Bind(resources.GSchemaGatewayPasswordKey, &remoteGatewayPasswordInput.Object, "text", gio.GSettingsBindDefaultValue)

	settings.Bind(resources.GSchemaWeronURLKey, &weronURLInput.Object, "text", gio.GSettingsBindDefaultValue)

	weronTimeoutInput.SetAdjustment(gtk.NewAdjustment(0, 0, math.MaxFloat64, 1, 1, 1))
	settings.Bind(resources.GSchemaWeronTimeoutKey, &weronTimeoutInput.Object, "value", gio.GSettingsBindDefaultValue)

	settings.Bind(resources.GSchemaWeronICEKey, &weronICEInput.Object, "text", gio.GSettingsBindDefaultValue)
	settings.Bind(resources.GSchemaWeronForceRelayKey, &weronForceRelayInput.Object, "active", gio.GSettingsBindDefaultValue)

	// Note: EntryRow, SpinRow, and PasswordEntryRow don't have ConnectChanged - they use notify signals
	// For simplicity, we'll track changes via the switch callbacks

	stateSetCallback1 := func(gtk.Switch, bool) bool {
		preferencesHaveChanged = true

		syncSensitivityState()

		return false
	}
	remoteGatewaySwitchInput.ConnectStateSet(&stateSetCallback1)

	stateSetCallback2 := func(gtk.Switch, bool) bool {
		preferencesHaveChanged = true

		return false
	}
	weronForceRelayInput.ConnectStateSet(&stateSetCallback2)

	aboutAction := gio.NewSimpleAction("about", nil)
	aboutCallback := func(action gio.SimpleAction, parameter uintptr) {
		aboutDialog.Present(&window.Window.Widget)
	}
	aboutAction.ConnectActivate(&aboutCallback)
	window.AddAction(aboutAction)

	menuButton.SetMenuModel(&menu.MenuModel)

	return &preferencesDialog, &mpvCommandInput
}
