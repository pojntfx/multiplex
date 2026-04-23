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
	gTypeSubtitlesDialog gobject.Type
)

type SubtitlesDialog struct {
	adw.Dialog

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

func NewSubtitlesDialog() *SubtitlesDialog {
	obj := gobject.NewObject(gTypeSubtitlesDialog, "css-name")

	return (*SubtitlesDialog)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))
}

func (s *SubtitlesDialog) EnableSpinner()   { s.spinner.SetVisible(true) }
func (s *SubtitlesDialog) DisableSpinner()  { s.spinner.SetVisible(false) }
func (s *SubtitlesDialog) EnableOKButton()  { s.okButton.SetSensitive(true) }
func (s *SubtitlesDialog) DisableOKButton() { s.okButton.SetSensitive(false) }

func (s *SubtitlesDialog) AddSubtitleTrack(row *adw.ActionRow) {
	s.selectionGroup.Add(&row.PreferencesRow.Widget)
}

func (s *SubtitlesDialog) Overlay() *adw.ToastOverlay { return s.overlay }

func (s *SubtitlesDialog) SetCancelCallback(callback func())      { s.cancelCallback = callback }
func (s *SubtitlesDialog) SetOKCallback(callback func())          { s.okCallback = callback }
func (s *SubtitlesDialog) SetAddFromFileCallback(callback func()) { s.addFromFileCallback = callback }

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

			var parent adw.Dialog
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
				Dialog:            parent,
				cancelButton:      &cancelButton,
				spinner:           &spinner,
				okButton:          &okButton,
				selectionGroup:    &selectionGroup,
				addFromFileButton: &addFromFileButton,
				overlay:           &overlay,
			}

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
	gobject.NewTypeQuery(adw.DialogGLibType(), &parentQuery)

	gTypeSubtitlesDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexSubtitlesDialog",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
