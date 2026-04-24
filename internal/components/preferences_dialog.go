package components

import (
	"context"
	"runtime"
	"unsafe"

	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/gio"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/assets/resources"
)

var (
	gTypePreferencesDialog gobject.Type
)

type PreferencesDialog struct {
	adw.PreferencesDialog

	storageLocationInput       *gtk.Button
	verbosityLevelInput        *adw.SpinRow
	remoteGatewaySwitchInput   *gtk.Switch
	remoteGatewayURLInput      *adw.EntryRow
	remoteGatewayUsernameInput *adw.EntryRow
	remoteGatewayPasswordInput *adw.PasswordEntryRow
	p2pandaRelayInput          *adw.EntryRow
	p2pandaNetworkInput        *adw.EntryRow
	p2pandaBootstrapInput      *adw.EntryRow

	preferencesHaveChanged bool

	ctx      context.Context
	settings *gio.Settings
	window   *adw.ApplicationWindow
	overlay  *adw.ToastOverlay
	gateway  *server.Gateway
	cancel   func()
}

func NewPreferencesDialog(
	ctx context.Context,
	settings *gio.Settings,
	window *adw.ApplicationWindow,
	overlay *adw.ToastOverlay,
	gateway *server.Gateway,
	cancel func(),
) *PreferencesDialog {
	obj := gobject.NewObject(gTypePreferencesDialog, "css-name")

	v := (*PreferencesDialog)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))
	v.ctx = ctx
	v.settings = settings
	v.window = window
	v.overlay = overlay
	v.gateway = gateway
	v.cancel = cancel

	v.setupBindings()
	v.setupCallbacks()

	return v
}

func (p *PreferencesDialog) markPreferencesChanged() {
	p.preferencesHaveChanged = true
}

func (p *PreferencesDialog) setupBindings() {
	p.verbosityLevelInput.SetAdjustment(gtk.NewAdjustment(0, 0, 8, 1, 1, 1))
	p.settings.Bind(resources.SchemaVerboseKey, &p.verbosityLevelInput.Object, "value", gio.GSettingsBindDefaultValue)

	p.settings.Bind(resources.SchemaGatewayRemoteKey, &p.remoteGatewaySwitchInput.Object, "active", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaGatewayURLKey, &p.remoteGatewayURLInput.Object, "text", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaGatewayUsernameKey, &p.remoteGatewayUsernameInput.Object, "text", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaGatewayPasswordKey, &p.remoteGatewayPasswordInput.Object, "text", gio.GSettingsBindDefaultValue)

	p.settings.Bind(resources.SchemaP2pandaRelayKey, &p.p2pandaRelayInput.Object, "text", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaP2pandaNetworkKey, &p.p2pandaNetworkInput.Object, "text", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaP2pandaBootstrapKey, &p.p2pandaBootstrapInput.Object, "text", gio.GSettingsBindDefaultValue)
}

func (p *PreferencesDialog) setupCallbacks() {
	syncSensitivityState := func() {
		editable := p.remoteGatewaySwitchInput.GetActive()
		p.remoteGatewayURLInput.SetEditable(editable)
		p.remoteGatewayUsernameInput.SetEditable(editable)
		p.remoteGatewayPasswordInput.SetEditable(editable)
	}

	onClosed := func(adw.Dialog) {
		if !p.preferencesHaveChanged {
			return
		}

		p.settings.Apply()

		toast := adw.NewToast(L("Reopen to apply the changes."))
		toast.SetButtonLabel(L("Reopen"))
		toast.SetActionName("win." + applyPreferencesActionName)

		p.overlay.AddToast(toast)

		p.preferencesHaveChanged = false
	}
	p.PreferencesDialog.Dialog.ConnectClosed(&onClosed)

	onShow := func(gtk.Widget) {
		syncSensitivityState()
	}
	p.ConnectShow(&onShow)

	onClicked := func(gtk.Button) {
		filePicker := gtk.NewFileChooserNative(
			L("Select storage location"),
			&p.window.Window,
			gtk.FileChooserActionSelectFolderValue,
			"",
			"")
		filePicker.SetModal(true)
		onFilePickerResponse := func(dialog gtk.NativeDialog, responseId int32) {
			if responseId == int32(gtk.ResponseAcceptValue) {
				p.settings.SetString(resources.SchemaStorageKey, filePicker.GetFile().GetPath())

				p.markPreferencesChanged()
			}

			filePicker.Destroy()
		}
		filePicker.ConnectResponse(&onFilePickerResponse)

		filePicker.Show()
	}
	p.storageLocationInput.ConnectClicked(&onClicked)

	onRemoteGatewayStateSet := func(gtk.Switch, bool) bool {
		p.markPreferencesChanged()

		syncSensitivityState()

		return false
	}
	p.remoteGatewaySwitchInput.ConnectStateSet(&onRemoteGatewayStateSet)
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourcePreferencesPath)

		typeClass.BindTemplateChildFull("storage_location_input", false, 0)
		typeClass.BindTemplateChildFull("verbosity_level_input", false, 0)
		typeClass.BindTemplateChildFull("htorrent_remote_gateway_switch", false, 0)
		typeClass.BindTemplateChildFull("htorrent_url_input", false, 0)
		typeClass.BindTemplateChildFull("htorrent_username_input", false, 0)
		typeClass.BindTemplateChildFull("htorrent_password_input", false, 0)
		typeClass.BindTemplateChildFull("p2panda_relay_input", false, 0)
		typeClass.BindTemplateChildFull("p2panda_network_input", false, 0)
		typeClass.BindTemplateChildFull("p2panda_bootstrap_input", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.PreferencesDialog
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				storageLocationInput       gtk.Button
				verbosityLevelInput        adw.SpinRow
				remoteGatewaySwitchInput   gtk.Switch
				remoteGatewayURLInput      adw.EntryRow
				remoteGatewayUsernameInput adw.EntryRow
				remoteGatewayPasswordInput adw.PasswordEntryRow
				p2pandaRelayInput          adw.EntryRow
				p2pandaNetworkInput        adw.EntryRow
				p2pandaBootstrapInput      adw.EntryRow
			)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "storage_location_input").Cast(&storageLocationInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "verbosity_level_input").Cast(&verbosityLevelInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_remote_gateway_switch").Cast(&remoteGatewaySwitchInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_url_input").Cast(&remoteGatewayURLInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_username_input").Cast(&remoteGatewayUsernameInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_password_input").Cast(&remoteGatewayPasswordInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "p2panda_relay_input").Cast(&p2pandaRelayInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "p2panda_network_input").Cast(&p2pandaNetworkInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "p2panda_bootstrap_input").Cast(&p2pandaBootstrapInput)

			p := &PreferencesDialog{
				PreferencesDialog: parent,

				storageLocationInput:       &storageLocationInput,
				verbosityLevelInput:        &verbosityLevelInput,
				remoteGatewaySwitchInput:   &remoteGatewaySwitchInput,
				remoteGatewayURLInput:      &remoteGatewayURLInput,
				remoteGatewayUsernameInput: &remoteGatewayUsernameInput,
				remoteGatewayPasswordInput: &remoteGatewayPasswordInput,
				p2pandaRelayInput:          &p2pandaRelayInput,
				p2pandaNetworkInput:        &p2pandaNetworkInput,
				p2pandaBootstrapInput:      &p2pandaBootstrapInput,

				preferencesHaveChanged: false,
			}

			var pinner runtime.Pinner
			pinner.Pin(p)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(p)), &onCleanup)
		})
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	var parentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.PreferencesDialogGLibType(), &parentQuery)

	gTypePreferencesDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexPreferencesDialog",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
