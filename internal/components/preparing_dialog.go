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
	gTypePreparingDialog gobject.Type
)

type PreparingDialog struct {
	adw.Dialog

	progressBar  *gtk.ProgressBar
	cancelButton *gtk.Button

	closeRequestCallback func() bool
	cancelCallback       func()
}

func NewPreparingDialog() *PreparingDialog {
	obj := gobject.NewObject(gTypePreparingDialog, "css-name")

	return (*PreparingDialog)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))
}

func (p *PreparingDialog) ProgressBar() *gtk.ProgressBar { return p.progressBar }

func (p *PreparingDialog) SetCloseRequestCallback(callback func() bool) {
	p.closeRequestCallback = callback
}

func (p *PreparingDialog) SetCancelCallback(callback func()) {
	p.cancelCallback = callback
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourcePreparingPath)

		typeClass.BindTemplateChildFull("preparing_progress_bar", false, 0)
		typeClass.BindTemplateChildFull("cancel_preparing_button", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.Dialog
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				progressBar  gtk.ProgressBar
				cancelButton gtk.Button
			)
			parent.Widget.GetTemplateChild(gTypePreparingDialog, "preparing_progress_bar").Cast(&progressBar)
			parent.Widget.GetTemplateChild(gTypePreparingDialog, "cancel_preparing_button").Cast(&cancelButton)

			p := &PreparingDialog{
				Dialog:       parent,
				progressBar:  &progressBar,
				cancelButton: &cancelButton,
			}

			onCloseAttempt := func(adw.Dialog) {
				if p.closeRequestCallback != nil {
					if !p.closeRequestCallback() {
						return
					}
				}
				parent.ForceClose()
			}
			parent.ConnectCloseAttempt(&onCloseAttempt)

			onCancelClicked := func(gtk.Button) {
				if p.cancelCallback != nil {
					p.cancelCallback()
				}
			}
			cancelButton.ConnectClicked(&onCancelClicked)

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
	gobject.NewTypeQuery(adw.DialogGLibType(), &parentQuery)

	gTypePreparingDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexPreparingDialog",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
