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
	"github.com/pojntfx/multiplex/internal/gschema"
	"github.com/pojntfx/multiplex/internal/ressources"
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
) (*adw.PreferencesWindow, *gtk.Entry) {
	menuBuilder := gtk.NewBuilderFromString(ressources.MenuUI, len(ressources.MenuUI))
	menu := menuBuilder.GetObject("main-menu").Cast().(*gio.Menu)

	aboutBuilder := gtk.NewBuilderFromString(ressources.AboutUI, len(ressources.AboutUI))
	aboutDialog := aboutBuilder.GetObject("about-dialog").Cast().(*adw.AboutWindow)

	preferencesBuilder := gtk.NewBuilderFromString(ressources.PreferencesUI, len(ressources.PreferencesUI))
	preferencesWindow := preferencesBuilder.GetObject("preferences-window").Cast().(*adw.PreferencesWindow)
	storageLocationInput := preferencesBuilder.GetObject("storage-location-input").Cast().(*gtk.Button)
	mpvCommandInput := preferencesBuilder.GetObject("mpv-command-input").Cast().(*gtk.Entry)
	verbosityLevelInput := preferencesBuilder.GetObject("verbosity-level-input").Cast().(*gtk.SpinButton)
	remoteGatewaySwitchInput := preferencesBuilder.GetObject("htorrent-remote-gateway-switch").Cast().(*gtk.Switch)
	remoteGatewayURLInput := preferencesBuilder.GetObject("htorrent-url-input").Cast().(*gtk.Entry)
	remoteGatewayUsernameInput := preferencesBuilder.GetObject("htorrent-username-input").Cast().(*gtk.Entry)
	remoteGatewayPasswordInput := preferencesBuilder.GetObject("htorrent-password-input").Cast().(*gtk.Entry)
	remoteGatewayURLRow := preferencesBuilder.GetObject("htorrent-url-row").Cast().(*adw.ActionRow)
	remoteGatewayUsernameRow := preferencesBuilder.GetObject("htorrent-username-row").Cast().(*adw.ActionRow)
	remoteGatewayPasswordRow := preferencesBuilder.GetObject("htorrent-password-row").Cast().(*adw.ActionRow)
	weronURLInput := preferencesBuilder.GetObject("weron-url-input").Cast().(*gtk.Entry)
	weronICEInput := preferencesBuilder.GetObject("weron-ice-input").Cast().(*gtk.Entry)
	weronTimeoutInput := preferencesBuilder.GetObject("weron-timeout-input").Cast().(*gtk.SpinButton)
	weronForceRelayInput := preferencesBuilder.GetObject("weron-force-relay-input").Cast().(*gtk.Switch)

	preferencesHaveChanged := false

	preferencesAction := gio.NewSimpleAction(preferencesActionName, nil)
	preferencesAction.ConnectActivate(func(parameter *glib.Variant) {
		preferencesWindow.Show()
	})
	app.SetAccelsForAction(preferencesActionName, []string{`<Primary>comma`})
	window.AddAction(preferencesAction)

	openDownloadsAction := gio.NewSimpleAction(openDownloadsActionName, nil)
	openDownloadsAction.ConnectActivate(func(parameter *glib.Variant) {
		if err := gio.AppInfoLaunchDefaultForURI(fmt.Sprintf("file://%v", settings.String(gschema.StorageFlag)), nil); err != nil {
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

	preferencesWindow.SetTransientFor(&window.Window)
	preferencesWindow.ConnectCloseRequest(func() (ok bool) {
		preferencesWindow.Close()
		preferencesWindow.SetVisible(false)

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
			remoteGatewayURLRow.SetSensitive(true)
			remoteGatewayUsernameRow.SetSensitive(true)
			remoteGatewayPasswordRow.SetSensitive(true)
		} else {
			remoteGatewayURLRow.SetSensitive(false)
			remoteGatewayUsernameRow.SetSensitive(false)
			remoteGatewayPasswordRow.SetSensitive(false)
		}
	}
	preferencesWindow.ConnectShow(syncSensitivityState)

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
			&preferencesWindow.Window.Window,
			gtk.FileChooserActionSelectFolder,
			"",
			"")
		filePicker.SetModal(true)
		filePicker.ConnectResponse(func(responseId int) {
			if responseId == int(gtk.ResponseAccept) {
				settings.SetString(gschema.StorageFlag, filePicker.File().Path())

				preferencesHaveChanged = true
			}

			filePicker.Destroy()
		})

		filePicker.Show()
	})

	settings.Bind(gschema.MPVFlag, mpvCommandInput.Object, "text", gio.SettingsBindDefault)

	verbosityLevelInput.SetAdjustment(gtk.NewAdjustment(0, 0, 8, 1, 1, 1))
	settings.Bind(gschema.VerboseFlag, verbosityLevelInput.Object, "value", gio.SettingsBindDefault)

	settings.Bind(gschema.GatewayRemoteFlag, remoteGatewaySwitchInput.Object, "active", gio.SettingsBindDefault)
	settings.Bind(gschema.GatewayURLFlag, remoteGatewayURLInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(gschema.GatewayUsernameFlag, remoteGatewayUsernameInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(gschema.GatewayPasswordFlag, remoteGatewayPasswordInput.Object, "text", gio.SettingsBindDefault)

	settings.Bind(gschema.WeronURLFlag, weronURLInput.Object, "text", gio.SettingsBindDefault)

	weronTimeoutInput.SetAdjustment(gtk.NewAdjustment(0, 0, math.MaxFloat64, 1, 1, 1))
	settings.Bind(gschema.WeronTimeoutFlag, weronTimeoutInput.Object, "value", gio.SettingsBindDefault)

	settings.Bind(gschema.WeronICEFlag, weronICEInput.Object, "text", gio.SettingsBindDefault)
	settings.Bind(gschema.WeronForceRelayFlag, weronForceRelayInput.Object, "active", gio.SettingsBindDefault)

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
		aboutDialog.Show()
	})
	window.AddAction(aboutAction)

	aboutDialog.SetTransientFor(&window.Window)
	aboutDialog.ConnectCloseRequest(func() (ok bool) {
		aboutDialog.Close()
		aboutDialog.SetVisible(false)

		return ok
	})

	menuButton.SetMenuModel(menu)

	return preferencesWindow, mpvCommandInput
}
