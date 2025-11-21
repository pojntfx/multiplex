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
	gTypeSubtitlesDialog gobject.Type
)

type SubtitlesDialog struct {
	adw.Window

	cancelButton      *gtk.Button
	spinner           *gtk.Spinner
	okButton          *gtk.Button
	selectionGroup    *adw.PreferencesGroup
	addFromFileButton *gtk.Button
	overlay           *adw.ToastOverlay

	cancelCallback      func()
	okCallback          func()
	addFromFileCallback func()
}

func NewSubtitlesDialog(transientFor *adw.ApplicationWindow) SubtitlesDialog {
	var w gtk.Window
	transientFor.Cast(&w)

	obj := gobject.NewObject(gTypeSubtitlesDialog, "transient-for", w)

	var v SubtitlesDialog
	obj.Cast(&v)

	return v
}

func (s *SubtitlesDialog) CancelButton() *gtk.Button {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	return subD.cancelButton
}

func (s *SubtitlesDialog) Spinner() *gtk.Spinner {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	return subD.spinner
}

func (s *SubtitlesDialog) OKButton() *gtk.Button {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	return subD.okButton
}

func (s *SubtitlesDialog) SelectionGroup() *adw.PreferencesGroup {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	return subD.selectionGroup
}

func (s *SubtitlesDialog) AddFromFileButton() *gtk.Button {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	return subD.addFromFileButton
}

func (s *SubtitlesDialog) Overlay() *adw.ToastOverlay {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	return subD.overlay
}

func (s *SubtitlesDialog) SetCancelCallback(callback func()) {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.cancelCallback = callback
}

func (s *SubtitlesDialog) SetOKCallback(callback func()) {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.okCallback = callback
}

func (s *SubtitlesDialog) SetAddFromFileCallback(callback func()) {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.addFromFileCallback = callback
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceSubtitlesPath)

		typeClass.BindTemplateChildFull("button-cancel", false, 0)
		typeClass.BindTemplateChildFull("headerbar-spinner", false, 0)
		typeClass.BindTemplateChildFull("button-ok", false, 0)
		typeClass.BindTemplateChildFull("subtitle-tracks", false, 0)
		typeClass.BindTemplateChildFull("add-from-file-button", false, 0)
		typeClass.BindTemplateChildFull("toast-overlay", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.Window
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				cancelButton      gtk.Button
				spinner           gtk.Spinner
				okButton          gtk.Button
				selectionGroup    adw.PreferencesGroup
				addFromFileButton gtk.Button
				overlay           adw.ToastOverlay
			)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "button-cancel").Cast(&cancelButton)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "headerbar-spinner").Cast(&spinner)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "button-ok").Cast(&okButton)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "subtitle-tracks").Cast(&selectionGroup)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "add-from-file-button").Cast(&addFromFileButton)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "toast-overlay").Cast(&overlay)

			s := &SubtitlesDialog{
				Window:            parent,
				cancelButton:      &cancelButton,
				spinner:           &spinner,
				okButton:          &okButton,
				selectionGroup:    &selectionGroup,
				addFromFileButton: &addFromFileButton,
				overlay:           &overlay,
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
				if s.cancelCallback != nil {
					s.cancelCallback()
				}
			}
			cancelButton.ConnectClicked(&onCancelClicked)

			onOKClicked := func(gtk.Button) {
				if s.okCallback != nil {
					s.okCallback()
				}
			}
			okButton.ConnectClicked(&onOKClicked)

			onAddFromFileClicked := func(gtk.Button) {
				if s.addFromFileCallback != nil {
					s.addFromFileCallback()
				}
			}
			addFromFileButton.ConnectClicked(&onAddFromFileClicked)

			var pinner runtime.Pinner
			pinner.Pin(s)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(s)), &onCleanup)
		})
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	var parentQuery gobject.TypeQuery
	gobject.NewTypeQuery(adw.WindowGLibType(), &parentQuery)

	gTypeSubtitlesDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"SubtitlesDialog",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize+uint(unsafe.Sizeof(SubtitlesDialog{}))+uint(unsafe.Sizeof(&SubtitlesDialog{})),
		&instanceInit,
		0,
	)
}
