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

		calendar := gtk.NewCalendar()
		entry := gtk.NewEntry()
		text := gtk.NewTextView()
		text.SetEditable(false)
		buf := text.Buffer()
		buf.SetText(`You chose to:
    * Frobnicate the foo.
    * Reverse the glop.
    * Enable future auto-frobnication.`)

		assistant := gtk.NewAssistant()

		assistant.AppendPage(calendar)
		assistant.AppendPage(entry)
		assistant.AppendPage(text)

		assistant.SetPageType(calendar, gtk.AssistantPageIntro)
		assistant.SetPageTitle(calendar, "This is an assistant.")
		assistant.SetPageComplete(calendar, true)

		assistant.SetPageType(entry, gtk.AssistantPageContent)
		assistant.SetPageTitle(entry, "Enter some information on this page.")
		assistant.SetPageComplete(entry, true)

		assistant.SetPageType(text, gtk.AssistantPageSummary)
		assistant.SetPageTitle(entry, "Congratulations, you're done.")

		assistant.Show()

		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
