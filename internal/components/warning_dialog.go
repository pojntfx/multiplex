package components

import (
	"runtime"
	"unsafe"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/pojntfx/multiplex/assets/resources"
)

var (
	gTypeWarningDialog gobject.Type
)

type WarningDialog struct {
	adw.AlertDialog

	responseCallback func(response string)
}

func NewWarningDialog() WarningDialog {
	obj := gobject.NewObject(gTypeWarningDialog, "css-name")

	var v WarningDialog
	obj.Cast(&v)

	return v
}

func (w *WarningDialog) SetResponseCallback(callback func(response string)) {
	warnW := (*WarningDialog)(unsafe.Pointer(w.GetData(dataKeyGoInstance)))
	warnW.responseCallback = callback
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceWarningPath)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.AlertDialog
			o.Cast(&parent)

			parent.InitTemplate()

			w := &WarningDialog{
				AlertDialog: parent,
			}

			onResponse := func(dialog adw.AlertDialog, response string) {
				if w.responseCallback != nil {
					w.responseCallback(response)
				}
			}
			parent.ConnectResponse(&onResponse)

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
	gobject.NewTypeQuery(adw.AlertDialogGLibType(), &parentQuery)

	gTypeWarningDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexWarningDialog",
		parentQuery.ClassSize,
		&classInit,
		parentQuery.InstanceSize,
		&instanceInit,
		0,
	)
}
