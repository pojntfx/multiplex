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
	gTypeDescriptionWindow gobject.Type
)

type DescriptionWindow struct {
	adw.Window

	text                 *gtk.TextView
	headerbarTitle       *gtk.Label
	headerbarSubtitle    *gtk.Label
	preparingProgressBar *gtk.ProgressBar
}

func NewDescriptionWindow(transientFor *adw.ApplicationWindow) DescriptionWindow {
	var w gtk.Window
	transientFor.Cast(&w)

	obj := gobject.NewObject(gTypeDescriptionWindow, "transient-for", w)

	var v DescriptionWindow
	obj.Cast(&v)

	return v
}

func (d *DescriptionWindow) Text() *gtk.TextView {
	descW := (*DescriptionWindow)(unsafe.Pointer(d.Widget.GetData(dataKeyGoInstance)))
	return descW.text
}

func (d *DescriptionWindow) HeaderbarTitle() *gtk.Label {
	descW := (*DescriptionWindow)(unsafe.Pointer(d.Widget.GetData(dataKeyGoInstance)))
	return descW.headerbarTitle
}

func (d *DescriptionWindow) HeaderbarSubtitle() *gtk.Label {
	descW := (*DescriptionWindow)(unsafe.Pointer(d.Widget.GetData(dataKeyGoInstance)))
	return descW.headerbarSubtitle
}

func (d *DescriptionWindow) PreparingProgressBar() *gtk.ProgressBar {
	descW := (*DescriptionWindow)(unsafe.Pointer(d.Widget.GetData(dataKeyGoInstance)))
	return descW.preparingProgressBar
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceDescriptionPath)

		typeClass.BindTemplateChildFull("description_text", false, 0)
		typeClass.BindTemplateChildFull("headerbar_title", false, 0)
		typeClass.BindTemplateChildFull("headerbar_subtitle", false, 0)
		typeClass.BindTemplateChildFull("preparing_progress_bar", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.Window
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				descriptionText                 gtk.TextView
				descriptionHeaderbarTitle       gtk.Label
				descriptionHeaderbarSubtitle    gtk.Label
				descriptionPreparingProgressBar gtk.ProgressBar
			)
			parent.Widget.GetTemplateChild(gTypeDescriptionWindow, "description_text").Cast(&descriptionText)
			parent.Widget.GetTemplateChild(gTypeDescriptionWindow, "headerbar_title").Cast(&descriptionHeaderbarTitle)
			parent.Widget.GetTemplateChild(gTypeDescriptionWindow, "headerbar_subtitle").Cast(&descriptionHeaderbarSubtitle)
			parent.Widget.GetTemplateChild(gTypeDescriptionWindow, "preparing_progress_bar").Cast(&descriptionPreparingProgressBar)

			w := &DescriptionWindow{
				Window:               parent,
				text:                 &descriptionText,
				headerbarTitle:       &descriptionHeaderbarTitle,
				headerbarSubtitle:    &descriptionHeaderbarSubtitle,
				preparingProgressBar: &descriptionPreparingProgressBar,
			}

			ctrl := gtk.NewEventControllerKey()
			parent.AddController(&ctrl.EventController)

			onCloseRequest := func(gtk.Window) bool {
				parent.Close()
				parent.SetVisible(false)
				return true
			}
			parent.ConnectCloseRequest(&onCloseRequest)

			onKeyReleased := func(ctrl gtk.EventControllerKey, keyval, keycode uint32, state gdk.ModifierType) {
				if keycode == keycodeEscape {
					parent.Close()
					parent.SetVisible(false)
				}
			}
			ctrl.ConnectKeyReleased(&onKeyReleased)

			var pinner runtime.Pinner
			pinner.Pin(w)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(w)), &onCleanup)
		})
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	var parentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.WindowGLibType(), &parentQuery)

	gTypeDescriptionWindow = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexDescriptionWindow",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
