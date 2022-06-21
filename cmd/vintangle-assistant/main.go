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

		var (
			calendarPage = gtk.NewBox(gtk.OrientationVertical, 6)
			entryPage    = gtk.NewBox(gtk.OrientationVertical, 6)
			textPage     = gtk.NewBox(gtk.OrientationVertical, 6)
		)

		// Calendar page
		calendarHeader := adw.NewHeaderBar()
		calendarHeader.AddCSSClass("flat")

		forwardToEntryButton := gtk.NewButtonWithLabel("Next")
		forwardToEntryButton.AddCSSClass("suggested-action")
		forwardToEntryButton.ConnectClicked(func() {
			stack.SetVisibleChild(entryPage)
		})

		calendarHeader.PackEnd(forwardToEntryButton)

		calendarPage.Append(calendarHeader)

		clamp := adw.NewClamp()
		clamp.SetMaximumSize(600)
		clamp.SetVExpand(true)
		clamp.SetVAlign(gtk.AlignCenter)
		clamp.SetMarginStart(12)
		clamp.SetMarginEnd(12)
		clamp.SetMarginBottom(12)

		calendar := gtk.NewCalendar()

		clamp.SetChild(calendar)

		calendarPage.Append(clamp)

		stack.AddChild(calendarPage)

		// Entry page
		entryHeader := adw.NewHeaderBar()
		entryHeader.AddCSSClass("flat")

		entryHeaderTitle := gtk.NewLabel("Media")
		entryHeaderTitle.AddCSSClass("title")

		entryHeader.SetTitleWidget(entryHeaderTitle)

		backToCalendarPage := gtk.NewButtonWithLabel("Previous")
		backToCalendarPage.ConnectClicked(func() {
			stack.SetVisibleChild(calendarPage)
		})

		entryHeader.PackStart(backToCalendarPage)

		forwardToTextPage := gtk.NewButtonWithLabel("Next")
		forwardToTextPage.AddCSSClass("suggested-action")
		forwardToTextPage.ConnectClicked(func() {
			stack.SetVisibleChild(textPage)
		})

		entryHeader.PackEnd(forwardToTextPage)

		entryPage.Append(entryHeader)

		stack.AddChild(entryPage)

		// Text page
		stack.AddChild(textPage)

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
