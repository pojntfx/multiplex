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
	gTypePreparingWindow gobject.Type
)

type PreparingWindow struct {
	adw.Window

	progressBar  *gtk.ProgressBar
	cancelButton *gtk.Button

	closeRequestCallback func() bool
	cancelCallback       func()
}

func NewPreparingWindow(transientFor *adw.ApplicationWindow) PreparingWindow {
	var w gtk.Window
	transientFor.Cast(&w)

	obj := gobject.NewObject(gTypePreparingWindow, "transient-for", w)

	var v PreparingWindow
	obj.Cast(&v)

	return v
}

func (p *PreparingWindow) ProgressBar() *gtk.ProgressBar {
	prepW := (*PreparingWindow)(unsafe.Pointer(p.Widget.GetData(dataKeyGoInstance)))
	return prepW.progressBar
}

func (p *PreparingWindow) SetCloseRequestCallback(callback func() bool) {
	prepW := (*PreparingWindow)(unsafe.Pointer(p.Widget.GetData(dataKeyGoInstance)))
	prepW.closeRequestCallback = callback
}

func (p *PreparingWindow) SetCancelCallback(callback func()) {
	prepW := (*PreparingWindow)(unsafe.Pointer(p.Widget.GetData(dataKeyGoInstance)))
	prepW.cancelCallback = callback
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

			var parent adw.Window
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				progressBar  gtk.ProgressBar
				cancelButton gtk.Button
			)
			parent.Widget.GetTemplateChild(gTypePreparingWindow, "preparing_progress_bar").Cast(&progressBar)
			parent.Widget.GetTemplateChild(gTypePreparingWindow, "cancel_preparing_button").Cast(&cancelButton)

			p := &PreparingWindow{
				Window:       parent,
				progressBar:  &progressBar,
				cancelButton: &cancelButton,
			}

			ctrl := gtk.NewEventControllerKey()
			parent.AddController(&ctrl.EventController)

			onCloseRequest := func(gtk.Window) bool {
				if p.closeRequestCallback != nil {
					return p.closeRequestCallback()
				}
				parent.Close()
				parent.SetVisible(false)
				return true
			}
			parent.ConnectCloseRequest(&onCloseRequest)

			onKeyReleased := func(ctrl gtk.EventControllerKey, keyval, keycode uint32, state gdk.ModifierType) {
				if keycode == keycodeEscape {
					if p.closeRequestCallback != nil {
						if p.closeRequestCallback() {
							return
						}
					}
					parent.Close()
					parent.SetVisible(false)
				}
			}
			ctrl.ConnectKeyReleased(&onKeyReleased)

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
	gobject.NewTypeQuery(adw.WindowGLibType(), &parentQuery)

	gTypePreparingWindow = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexPreparingWindow",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
