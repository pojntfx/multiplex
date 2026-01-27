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
	spinner           *adw.Spinner
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

func (s *SubtitlesDialog) EnableSpinner() {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.spinner.SetVisible(true)
}

func (s *SubtitlesDialog) DisableSpinner() {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.spinner.SetVisible(false)
}

func (s *SubtitlesDialog) EnableOKButton() {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.okButton.SetSensitive(true)
}

func (s *SubtitlesDialog) DisableOKButton() {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.okButton.SetSensitive(false)
}

func (s *SubtitlesDialog) AddSubtitleTrack(row *adw.ActionRow) {
	subD := (*SubtitlesDialog)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))
	subD.selectionGroup.Add(&row.PreferencesRow.Widget)
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

		typeClass.BindTemplateChildFull("button_cancel", false, 0)
		typeClass.BindTemplateChildFull("headerbar_spinner", false, 0)
		typeClass.BindTemplateChildFull("button_ok", false, 0)
		typeClass.BindTemplateChildFull("subtitle_tracks", false, 0)
		typeClass.BindTemplateChildFull("add_from_file_button", false, 0)
		typeClass.BindTemplateChildFull("toast_overlay", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.Window
			o.Cast(&parent)

			parent.InitTemplate()

			var (
				cancelButton      gtk.Button
				spinner           adw.Spinner
				okButton          gtk.Button
				selectionGroup    adw.PreferencesGroup
				addFromFileButton gtk.Button
				overlay           adw.ToastOverlay
			)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "button_cancel").Cast(&cancelButton)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "headerbar_spinner").Cast(&spinner)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "button_ok").Cast(&okButton)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "subtitle_tracks").Cast(&selectionGroup)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "add_from_file_button").Cast(&addFromFileButton)
			parent.Widget.GetTemplateChild(gTypeSubtitlesDialog, "toast_overlay").Cast(&overlay)

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

	parentQuery := newTypeQuery(adw.WindowGLibType())

	gTypeSubtitlesDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexSubtitlesDialog",
		uint(parentQuery.ClassSize),
		&classInit,
		uint(parentQuery.InstanceSize)+uint(unsafe.Sizeof(SubtitlesDialog{}))+uint(unsafe.Sizeof(&SubtitlesDialog{})),
		&instanceInit,
		0,
	)
}
