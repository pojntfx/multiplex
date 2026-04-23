package components

import (
	"runtime"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/adw"
	"codeberg.org/puregotk/puregotk/v4/glib"
	"codeberg.org/puregotk/puregotk/v4/gobject"
	"codeberg.org/puregotk/puregotk/v4/gtk"
	"github.com/pojntfx/multiplex/assets/resources"
)

var (
	gTypeAudioTracksDialog gobject.Type
)

type AudioTracksDialog struct {
	adw.Dialog

	cancelButton   *gtk.Button
	okButton       *gtk.Button
	selectionGroup *adw.PreferencesGroup

	cancelCallback func()
	okCallback     func()
}

func NewAudioTracksDialog() *AudioTracksDialog {
	obj := gobject.NewObject(gTypeAudioTracksDialog, "css-name")

	return (*AudioTracksDialog)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))
}

func (a *AudioTracksDialog) AddAudioTrack(row *adw.ActionRow) {
	a.selectionGroup.Add(&row.PreferencesRow.Widget)
}

func (a *AudioTracksDialog) SetCancelCallback(callback func()) { a.cancelCallback = callback }
func (a *AudioTracksDialog) SetOKCallback(callback func())     { a.okCallback = callback }

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

			var parent adw.Dialog
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
				Dialog:         parent,
				cancelButton:   &cancelButton,
				okButton:       &okButton,
				selectionGroup: &selectionGroup,
			}

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

	var parentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.DialogGLibType(), &parentQuery)

	gTypeAudioTracksDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexAudioTracksDialog",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
