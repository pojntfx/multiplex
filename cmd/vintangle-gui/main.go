package main

import (
	"log"
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

		magnetLinkEntry := gtk.NewEntry()
		searchButton := gtk.NewButton()

		onSubmit := func() {
			if text := magnetLinkEntry.Text(); strings.TrimSpace(text) != "" {
				searchButton.SetSensitive(false)
				magnetLinkEntry.SetSensitive(false)

				spinner.SetSpinning(true)

				go func() {
					time.Sleep(2 * time.Second)

					searchButton.RemoveCSSClass("suggested-action")
					spinner.SetSpinning(false)
					status.SetDescription("Select the media you want to stream")

					pathActions := gtk.NewBox(gtk.OrientationHorizontal, 12)
					pathActions.SetHAlign(gtk.AlignCenter)
					pathActions.SetVAlign(gtk.AlignCenter)

					media := []string{"sadf.jpg", "asdf.mkv"}
					dropdown := gtk.NewDropDownFromStrings(media)

					pathActions.Append(dropdown)

					playButton := gtk.NewButton()
					playButton.SetIconName("media-playback-start-symbolic")
					playButton.AddCSSClass("suggested-action")
					playButton.AddCSSClass("circular")
					playButton.ConnectClicked(func() {
						log.Println("Playing", media[dropdown.Selected()])
					})

					pathActions.Append(playButton)

					entryBox.Append(pathActions)
				}()
			}
		}

		magnetLinkEntry.SetPlaceholderText("Magnet link")
		magnetLinkEntry.ConnectActivate(onSubmit)

		magnetActions.Append(magnetLinkEntry)

		searchButton.SetIconName("system-search-symbolic")
		searchButton.AddCSSClass("suggested-action")
		searchButton.AddCSSClass("circular")
		searchButton.ConnectClicked(onSubmit)

		magnetActions.Append(searchButton)

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
