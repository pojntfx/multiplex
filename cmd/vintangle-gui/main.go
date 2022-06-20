package main

import (
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintangle", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.ConnectActivate(func() {
		window := adw.NewApplicationWindow(&app.Application)
		window.SetTitle("Vintangle")
		window.SetDefaultSize(600, 500)

		handle := gtk.NewWindowHandle()
		stack := gtk.NewStack()

		settingsPage := gtk.NewBox(gtk.OrientationVertical, 6)

		header := gtk.NewHeaderBar()
		header.AddCSSClass("flat")

		spinner := gtk.NewSpinner()
		spinner.SetMarginEnd(6)

		header.PackEnd(spinner)

		title := gtk.NewBox(gtk.OrientationHorizontal, 0)
		title.SetVisible(false)
		header.SetTitleWidget(title)

		settingsPage.Append(header)

		clamp := adw.NewClamp()
		clamp.SetMaximumSize(450)
		clamp.SetVExpand(true)
		clamp.SetVAlign(gtk.AlignFill)

		status := adw.NewStatusPage()
		status.SetMarginStart(12)
		status.SetMarginEnd(12)
		status.SetIconName("multimedia-player-symbolic")
		status.SetTitle("Vintangle")
		status.SetDescription("Enter a magnet link to start streaming")

		entryBox := gtk.NewBox(gtk.OrientationVertical, 12)

		magnetActions := gtk.NewBox(gtk.OrientationHorizontal, 12)
		magnetActions.SetHAlign(gtk.AlignCenter)
		magnetActions.SetVAlign(gtk.AlignCenter)

		entry := gtk.NewEntry()
		button := gtk.NewButton()

		onSubmit := func() {
			if text := entry.Text(); strings.TrimSpace(text) != "" {
				button.SetSensitive(false)
				entry.SetSensitive(false)

				spinner.SetSpinning(true)

				go func() {
					time.Sleep(2 * time.Second)

					spinner.SetSpinning(false)

					separator := gtk.NewSeparator(gtk.OrientationHorizontal)

					entryBox.Append(separator)

					pathActions := gtk.NewBox(gtk.OrientationHorizontal, 12)
					pathActions.SetHAlign(gtk.AlignCenter)
					pathActions.SetVAlign(gtk.AlignCenter)

					dropdown := gtk.NewDropDownFromStrings([]string{"sadf.jpg", "asdf.mkv"})

					pathActions.Append(dropdown)

					entryBox.Append(pathActions)
				}()
			}
		}

		entry.SetPlaceholderText("Magnet link")
		entry.ConnectActivate(onSubmit)

		magnetActions.Append(entry)

		button.SetIconName("system-search-symbolic")
		button.AddCSSClass("suggested-action")
		button.AddCSSClass("circular")
		button.ConnectClicked(onSubmit)

		magnetActions.Append(button)

		entryBox.Append(magnetActions)

		status.SetChild(entryBox)

		clamp.SetChild(status)

		settingsPage.Append(clamp)

		stack.AddChild(settingsPage)

		handle.SetChild(stack)

		window.SetContent(handle)
		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
