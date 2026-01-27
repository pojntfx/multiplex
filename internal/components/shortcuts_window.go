package components

import (
	"runtime"
	"unsafe"

	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gdk"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/pojntfx/multiplex/assets/resources"
)

var (
	gTypeShortcutsWindow gobject.Type
)

type shortcutInfo struct {
	title      string
	actionName string
}

type shortcutSection struct {
	title     string
	shortcuts []shortcutInfo
}

type ShortcutsWindow struct {
	adw.Window

	container *gtk.Box
	app       *adw.Application
}

func NewShortcutsWindow(transientFor *adw.ApplicationWindow, app *adw.Application) *ShortcutsWindow {
	var w gtk.Window
	transientFor.Cast(&w)

	obj := gobject.NewObject(gTypeShortcutsWindow, "transient-for", w)

	v := (*ShortcutsWindow)(unsafe.Pointer(obj.GetData(dataKeyGoInstance)))
	v.app = app

	onShow := func(gtk.Widget) {
		v.populateShortcuts()
	}
	v.Window.ConnectShow(&onShow)

	return v
}

func (s *ShortcutsWindow) populateShortcuts() {
	w := (*ShortcutsWindow)(unsafe.Pointer(s.Widget.GetData(dataKeyGoInstance)))

	// Remove all the old shortcuts
	for {
		child := w.container.GetFirstChild()
		if child == nil {
			break
		}

		w.container.Remove(child)
	}

	var a gtk.Application
	w.app.Cast(&a)

	for _, section := range []shortcutSection{
		{
			title: L("General"),
			shortcuts: []shortcutInfo{
				{title: L("Show Keyboard Shortcuts"), actionName: "win.shortcuts"},
				{title: L("Close Window"), actionName: "win.closeWindow"},
				{title: L("Quit"), actionName: "app.quit"},
			},
		},
		{
			title: L("Playback"),
			shortcuts: []shortcutInfo{
				{title: L("Toggle Playback"), actionName: "win.togglePlayback"},
				{title: L("Toggle Fullscreen"), actionName: "win.toggleFullscreen"},
			},
		},
	} {
		listBox := gtk.NewListBox()
		listBox.SetSelectionMode(gtk.SelectionNoneValue)
		listBox.AddCssClass("boxed-list")

		sectionHasActiveShortcuts := false
		for _, shortcut := range section.shortcuts {
			accels := a.GetAccelsForAction(shortcut.actionName)
			if len(accels) == 0 {
				// The accelerators for some shortcuts (e.g. for the control window) don't get
				// registered until the window they are attached to has been opened, so don't render their label

				continue
			}

			row := adw.NewActionRow()
			row.SetTitle(shortcut.title)

			shortcutLabel := adw.NewShortcutLabel(accels[0])
			shortcutLabel.SetValign(gtk.AlignCenterValue)
			row.AddSuffix(&shortcutLabel.Widget)

			listBox.Append(&row.Widget)
			sectionHasActiveShortcuts = true
		}

		// The accelerators for some shortcuts (e.g. for the control window) don't get
		// registered until the window they are attached to has been opened, so don't render their section
		if !sectionHasActiveShortcuts {
			continue
		}

		sectionBox := gtk.NewBox(gtk.OrientationVerticalValue, 12)

		titleLabel := gtk.NewLabel(section.title)
		titleLabel.SetHalign(gtk.AlignStartValue)
		titleLabel.AddCssClass("heading")
		sectionBox.Append(&titleLabel.Widget)

		sectionBox.Append(&listBox.Widget)
		w.container.Append(&sectionBox.Widget)
	}
}

func init() {
	var shortcutsClassInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceShortcutsWindowPath)

		typeClass.BindTemplateChildFull("shortcuts_container", false, 0)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.Window
			o.Cast(&parent)

			parent.InitTemplate()

			var container gtk.Box
			parent.Widget.GetTemplateChild(gTypeShortcutsWindow, "shortcuts_container").Cast(&container)

			w := &ShortcutsWindow{
				Window:    parent,
				container: &container,
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

			var pinner runtime.Pinner
			pinner.Pin(w)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(w)), &onCleanup)
		})
	}

	var shortcutsInstanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	shortcutsParentQuery := newTypeQuery(adw.WindowGLibType())

	gTypeShortcutsWindow = gobject.TypeRegisterStaticSimple(
		shortcutsParentQuery.Type,
		"MultiplexShortcutsWindow",
		uint(shortcutsParentQuery.ClassSize),
		&shortcutsClassInit,
		uint(shortcutsParentQuery.InstanceSize)+uint(unsafe.Sizeof(ShortcutsWindow{}))+uint(unsafe.Sizeof(&ShortcutsWindow{})),
		&shortcutsInstanceInit,
		0,
	)
}
