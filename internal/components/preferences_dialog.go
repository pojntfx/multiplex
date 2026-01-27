package components

import (
	"context"
	"math"
	"runtime"
	"unsafe"

	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gio"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/pojntfx/multiplex/assets/resources"
)

var (
	gTypePreferencesDialog gobject.Type
)

type PreferencesDialog struct {
	adw.PreferencesWindow

	storageLocationInput       *gtk.Button
	mpvCommandInput            *adw.EntryRow
	verbosityLevelInput        *adw.SpinRow
	remoteGatewaySwitchInput   *gtk.Switch
	remoteGatewayURLInput      *adw.EntryRow
	remoteGatewayUsernameInput *adw.EntryRow
	remoteGatewayPasswordInput *adw.PasswordEntryRow
	weronURLInput              *adw.EntryRow
	weronICEInput              *adw.EntryRow
	weronTimeoutInput          *adw.SpinRow
	weronForceRelayInput       *gtk.Switch

	preferencesHaveChanged bool
	closeRequestCallback   func() bool

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

func (p *PreferencesDialog) MpvCommandInput() *adw.EntryRow {
	return p.mpvCommandInput
}

func (p *PreferencesDialog) setCloseRequestCallback(callback func() bool) {
	prefD := (*PreferencesDialog)(unsafe.Pointer(p.GetData(dataKeyGoInstance)))
	prefD.closeRequestCallback = callback
}

func (p *PreferencesDialog) markPreferencesChanged() {
	prefD := (*PreferencesDialog)(unsafe.Pointer(p.GetData(dataKeyGoInstance)))
	prefD.preferencesHaveChanged = true
}

func (p *PreferencesDialog) resetPreferencesChanged() {
	prefD := (*PreferencesDialog)(unsafe.Pointer(p.GetData(dataKeyGoInstance)))
	prefD.preferencesHaveChanged = false
}

func (p *PreferencesDialog) havePreferencesChanged() bool {
	prefD := (*PreferencesDialog)(unsafe.Pointer(p.GetData(dataKeyGoInstance)))
	return prefD.preferencesHaveChanged
}

func (p *PreferencesDialog) setupBindings() {
	p.settings.Bind(resources.SchemaMPVKey, &p.mpvCommandInput.Object, "text", gio.GSettingsBindDefaultValue)

	p.verbosityLevelInput.SetAdjustment(gtk.NewAdjustment(0, 0, 8, 1, 1, 1))
	p.settings.Bind(resources.SchemaVerboseKey, &p.verbosityLevelInput.Object, "value", gio.GSettingsBindDefaultValue)

	p.settings.Bind(resources.SchemaGatewayRemoteKey, &p.remoteGatewaySwitchInput.Object, "active", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaGatewayURLKey, &p.remoteGatewayURLInput.Object, "text", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaGatewayUsernameKey, &p.remoteGatewayUsernameInput.Object, "text", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaGatewayPasswordKey, &p.remoteGatewayPasswordInput.Object, "text", gio.GSettingsBindDefaultValue)

	p.settings.Bind(resources.SchemaWeronURLKey, &p.weronURLInput.Object, "text", gio.GSettingsBindDefaultValue)

	p.weronTimeoutInput.SetAdjustment(gtk.NewAdjustment(0, 0, math.MaxFloat64, 1, 1, 1))
	p.settings.Bind(resources.SchemaWeronTimeoutKey, &p.weronTimeoutInput.Object, "value", gio.GSettingsBindDefaultValue)

	p.settings.Bind(resources.SchemaWeronICEKey, &p.weronICEInput.Object, "text", gio.GSettingsBindDefaultValue)
	p.settings.Bind(resources.SchemaWeronForceRelayKey, &p.weronForceRelayInput.Object, "active", gio.GSettingsBindDefaultValue)
}

func (p *PreferencesDialog) setupCallbacks() {
	p.SetTransientFor(&p.window.Window)

	syncSensitivityState := func() {
		if p.remoteGatewaySwitchInput.GetActive() {
			p.remoteGatewayURLInput.SetEditable(true)
			p.remoteGatewayUsernameInput.SetEditable(true)
			p.remoteGatewayPasswordInput.SetEditable(true)
		} else {
			p.remoteGatewayURLInput.SetEditable(false)
			p.remoteGatewayUsernameInput.SetEditable(false)
			p.remoteGatewayPasswordInput.SetEditable(false)
		}
	}

	onCloseRequest := func() bool {
		p.Close()
		p.SetVisible(false)

		if p.havePreferencesChanged() {
			p.settings.Apply()

			toast := adw.NewToast(L("Reopen to apply the changes."))
			toast.SetButtonLabel(L("Reopen"))
			toast.SetActionName("win." + applyPreferencesActionName)

			p.overlay.AddToast(toast)
		}

		p.resetPreferencesChanged()

		return true
	}
	p.setCloseRequestCallback(onCloseRequest)

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
		onFilePickerResponse := func(dialog gtk.NativeDialog, responseId int) {
			if responseId == int(gtk.ResponseAcceptValue) {
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

	onWeronForceRelayStateSet := func(gtk.Switch, bool) bool {
		p.markPreferencesChanged()

		return false
	}
	p.weronForceRelayInput.ConnectStateSet(&onWeronForceRelayStateSet)
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourcePreferencesPath)

		typeClass.BindTemplateChildFull("storage_location_input", false, 0)
		typeClass.BindTemplateChildFull("mpv_command_input", false, 0)
		typeClass.BindTemplateChildFull("verbosity_level_input", false, 0)
		typeClass.BindTemplateChildFull("htorrent_remote_gateway_switch", false, 0)
		typeClass.BindTemplateChildFull("htorrent_url_input", false, 0)
		typeClass.BindTemplateChildFull("htorrent_username_input", false, 0)
		typeClass.BindTemplateChildFull("htorrent_password_input", false, 0)
		typeClass.BindTemplateChildFull("weron_url_input", false, 0)
		typeClass.BindTemplateChildFull("weron_ice_input", false, 0)
		typeClass.BindTemplateChildFull("weron_timeout_input", false, 0)
		typeClass.BindTemplateChildFull("weron_force_relay_input", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.PreferencesWindow
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				storageLocationInput       gtk.Button
				mpvCommandInput            adw.EntryRow
				verbosityLevelInput        adw.SpinRow
				remoteGatewaySwitchInput   gtk.Switch
				remoteGatewayURLInput      adw.EntryRow
				remoteGatewayUsernameInput adw.EntryRow
				remoteGatewayPasswordInput adw.PasswordEntryRow
				weronURLInput              adw.EntryRow
				weronICEInput              adw.EntryRow
				weronTimeoutInput          adw.SpinRow
				weronForceRelayInput       gtk.Switch
			)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "storage_location_input").Cast(&storageLocationInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "mpv_command_input").Cast(&mpvCommandInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "verbosity_level_input").Cast(&verbosityLevelInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_remote_gateway_switch").Cast(&remoteGatewaySwitchInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_url_input").Cast(&remoteGatewayURLInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_username_input").Cast(&remoteGatewayUsernameInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "htorrent_password_input").Cast(&remoteGatewayPasswordInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "weron_url_input").Cast(&weronURLInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "weron_ice_input").Cast(&weronICEInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "weron_timeout_input").Cast(&weronTimeoutInput)
			parent.Widget.GetTemplateChild(gTypePreferencesDialog, "weron_force_relay_input").Cast(&weronForceRelayInput)

			p := &PreferencesDialog{
				PreferencesWindow: parent,

				storageLocationInput:       &storageLocationInput,
				mpvCommandInput:            &mpvCommandInput,
				verbosityLevelInput:        &verbosityLevelInput,
				remoteGatewaySwitchInput:   &remoteGatewaySwitchInput,
				remoteGatewayURLInput:      &remoteGatewayURLInput,
				remoteGatewayUsernameInput: &remoteGatewayUsernameInput,
				remoteGatewayPasswordInput: &remoteGatewayPasswordInput,
				weronURLInput:              &weronURLInput,
				weronICEInput:              &weronICEInput,
				weronTimeoutInput:          &weronTimeoutInput,
				weronForceRelayInput:       &weronForceRelayInput,

				preferencesHaveChanged: false,
			}

			onCloseRequest := func(gtk.Window) bool {
				if p.closeRequestCallback != nil {
					return p.closeRequestCallback()
				}
				return false
			}
			parent.ConnectCloseRequest(&onCloseRequest)

			var pinner runtime.Pinner
			pinner.Pin(p)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(p)), &onCleanup)
		})
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	parentQuery := newTypeQuery(adw.PreferencesWindowGLibType())

	gTypePreferencesDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexPreferencesDialog",
		uint(parentQuery.ClassSize),
		&classInit,
		uint(parentQuery.InstanceSize)+uint(unsafe.Sizeof(PreferencesDialog{}))+uint(unsafe.Sizeof(&PreferencesDialog{})),
		&instanceInit,
		0,
	)
}
