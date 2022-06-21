package main

import (
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintangle.assistant", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.ConnectActivate(func() {
		window := adw.NewApplicationWindow(&app.Application)
		window.SetTitle("Vintangle")
		window.SetDefaultSize(700, 500)

		stack := gtk.NewStack()

		calendarPage := gtk.NewBox(gtk.OrientationVertical, 6)

		header := gtk.NewHeaderBar()
		header.AddCSSClass("flat")

		clamp := adw.NewClamp()
		clamp.SetMaximumSize(600)
		clamp.SetVExpand(true)
		clamp.SetVAlign(gtk.AlignFill)

		calendar := gtk.NewCalendar()

		calendarPage.Append(header)
		clamp.SetChild(calendar)

		calendarPage.Append(clamp)

		stack.AddChild(calendarPage)
		// 	entry := gtk.NewEntry()
		// 	text := gtk.NewTextView()
		// 	text.SetEditable(false)
		// 	buf := text.Buffer()
		// 	buf.SetText(`You chose to:
		// * Frobnicate the foo.
		// * Reverse the glop.
		// * Enable future auto-frobnication.`)

		window.SetContent(stack)
		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
