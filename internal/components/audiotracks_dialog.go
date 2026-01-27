package components

import (
	"runtime"
	"unsafe"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/pojntfx/multiplex/assets/resources"
)

var (
	gTypeAudioTracksDialog gobject.Type
)

type AudioTracksDialog struct {
	adw.Window

	cancelButton   *gtk.Button
	okButton       *gtk.Button
	selectionGroup *adw.PreferencesGroup

	cancelCallback func()
	okCallback     func()
}

func NewAudioTracksDialog(transientFor *adw.ApplicationWindow) AudioTracksDialog {
	var w gtk.Window
	transientFor.Cast(&w)

	obj := gobject.NewObject(gTypeAudioTracksDialog, "transient-for", w)

	var v AudioTracksDialog
	obj.Cast(&v)

	return v
}

func (a *AudioTracksDialog) AddAudioTrack(row *adw.ActionRow) {
	audioD := (*AudioTracksDialog)(unsafe.Pointer(a.Widget.GetData(dataKeyGoInstance)))
	audioD.selectionGroup.Add(&row.PreferencesRow.Widget)
}

func (a *AudioTracksDialog) SetCancelCallback(callback func()) {
	audioD := (*AudioTracksDialog)(unsafe.Pointer(a.Widget.GetData(dataKeyGoInstance)))
	audioD.cancelCallback = callback
}

func (a *AudioTracksDialog) SetOKCallback(callback func()) {
	audioD := (*AudioTracksDialog)(unsafe.Pointer(a.Widget.GetData(dataKeyGoInstance)))
	audioD.okCallback = callback
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceAudiotracksPath)

		typeClass.BindTemplateChildFull("button_cancel", false, 0)
		typeClass.BindTemplateChildFull("button_ok", false, 0)
		typeClass.BindTemplateChildFull("audiotracks", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.Window
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				cancelButton   gtk.Button
				okButton       gtk.Button
				selectionGroup adw.PreferencesGroup
			)
			parent.Widget.GetTemplateChild(gTypeAudioTracksDialog, "button_cancel").Cast(&cancelButton)
			parent.Widget.GetTemplateChild(gTypeAudioTracksDialog, "button_ok").Cast(&okButton)
			parent.Widget.GetTemplateChild(gTypeAudioTracksDialog, "audiotracks").Cast(&selectionGroup)

			a := &AudioTracksDialog{
				Window:         parent,
				cancelButton:   &cancelButton,
				okButton:       &okButton,
				selectionGroup: &selectionGroup,
			}

			ctrl := gtk.NewEventControllerKey()
			parent.AddController(&ctrl.EventController)

			onCloseRequest := func(gtk.Window) bool {
				parent.Close()
				parent.SetVisible(false)
				return true
			}
			parent.ConnectCloseRequest(&onCloseRequest)

			onKeyReleased := func(ctrl gtk.EventControllerKey, keyval, keycode uint, state gdk.ModifierType) {
				if keycode == keycodeEscape {
					parent.Close()
					parent.SetVisible(false)
				}
			}
			ctrl.ConnectKeyReleased(&onKeyReleased)

			onCancelClicked := func(gtk.Button) {
				if a.cancelCallback != nil {
					a.cancelCallback()
				}
			}
			cancelButton.ConnectClicked(&onCancelClicked)

			onOKClicked := func(gtk.Button) {
				if a.okCallback != nil {
					a.okCallback()
				}
			}
			okButton.ConnectClicked(&onOKClicked)

			var pinner runtime.Pinner
			pinner.Pin(a)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(a)), &onCleanup)
		})
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	parentQuery := newTypeQuery(adw.WindowGLibType())

	gTypeAudioTracksDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexAudioTracksDialog",
		uint(parentQuery.ClassSize),
		&classInit,
		uint(parentQuery.InstanceSize)+uint(unsafe.Sizeof(AudioTracksDialog{}))+uint(unsafe.Sizeof(&AudioTracksDialog{})),
		&instanceInit,
		0,
	)
}
