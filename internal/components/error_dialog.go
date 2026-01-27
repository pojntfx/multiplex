package components

import (
	"context"
	"os"
	"runtime"
	"unsafe"

	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/glib"
	"github.com/jwijenbergh/puregotk/v4/gobject"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	"github.com/pojntfx/multiplex/assets/resources"
	"github.com/rs/zerolog/log"
	"github.com/rymdport/portal/openuri"
)

var (
	gTypeErrorDialog gobject.Type
)

const (
	issuesURL = "https://github.com/pojntfx/multiplex/issues"
)

type ErrorDialog struct {
	adw.AlertDialog

	responseCallback func(response string)
}

func NewErrorDialog() ErrorDialog {
	obj := gobject.NewObject(gTypeErrorDialog, "css-name")

	var v ErrorDialog
	obj.Cast(&v)

	return v
}

func (e *ErrorDialog) SetResponseCallback(callback func(response string)) {
	errD := (*ErrorDialog)(unsafe.Pointer(e.GetData(dataKeyGoInstance)))
	errD.responseCallback = callback
}

func OpenErrorDialog(ctx context.Context, window *adw.ApplicationWindow, err error) {
	log.Error().
		Err(err).
		Msg(L("Could not continue due to a fatal error"))

	errorDialog := NewErrorDialog()
	errorDialog.SetBody(err.Error())

	onResponse := func(response string) {
		switch response {
		case "report":
			_ = openuri.OpenURI("", issuesURL, nil)

			errorDialog.Close()

			os.Exit(1)

		default:
			errorDialog.Close()

			os.Exit(1)
		}
	}
	errorDialog.SetResponseCallback(onResponse)

	errorDialog.Present(&window.Widget)
}

func init() {
	var classInit gobject.ClassInitFunc = func(tc *gobject.TypeClass, u uintptr) {
		typeClass := (*gtk.WidgetClass)(unsafe.Pointer(tc))
		typeClass.SetTemplateFromResource(resources.ResourceErrorPath)

		objClass := (*gobject.ObjectClass)(unsafe.Pointer(tc))

		objClass.OverrideConstructed(func(o *gobject.Object) {
			parentObjClass := (*gobject.ObjectClass)(unsafe.Pointer(tc.PeekParent()))
			parentObjClass.GetConstructed()(o)

			var parent adw.AlertDialog
			o.Cast(&parent)

			parent.InitTemplate()

			e := &ErrorDialog{
				AlertDialog: parent,
			}

			onResponse := func(dialog adw.AlertDialog, response string) {
				if e.responseCallback != nil {
					e.responseCallback(response)
				}
			}
			parent.ConnectResponse(&onResponse)

			var pinner runtime.Pinner
			pinner.Pin(e)

			onCleanup := glib.DestroyNotify(func(data uintptr) {
				pinner.Unpin()
			})
			o.SetDataFull(dataKeyGoInstance, uintptr(unsafe.Pointer(e)), &onCleanup)
		})
	}

	var instanceInit gobject.InstanceInitFunc = func(ti *gobject.TypeInstance, tc *gobject.TypeClass) {}

	parentQuery := newTypeQuery(adw.AlertDialogGLibType())

	gTypeErrorDialog = gobject.TypeRegisterStaticSimple(
		parentQuery.Type,
		"MultiplexErrorDialog",
		uint(parentQuery.ClassSize),
		&classInit,
		uint(parentQuery.InstanceSize)+uint(unsafe.Sizeof(ErrorDialog{}))+uint(unsafe.Sizeof(&ErrorDialog{}))+uint(unsafe.Sizeof(adw.AlertDialog{})), // TODO: Figure out why we need the extra `adw.AlertDialog` here
		&instanceInit,
		0,
	)
}
