package main

import (
	"log"
	"os"
	"strings"

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

		box := gtk.NewBox(gtk.OrientationVertical, 6)

		header := gtk.NewHeaderBar()
		header.AddCSSClass("flat")

		title := gtk.NewBox(gtk.OrientationHorizontal, 0)
		title.SetVisible(false)
		header.SetTitleWidget(title)

		box.Append(header)

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

		actions := gtk.NewBox(gtk.OrientationHorizontal, 12)
		actions.SetHAlign(gtk.AlignCenter)
		actions.SetVAlign(gtk.AlignCenter)

		entry := gtk.NewEntry()
		entry.SetPlaceholderText("Magnet link")
		entry.ConnectActivate(func() {
			if text := entry.Text(); strings.TrimSpace(text) != "" {
				log.Println("Using link", entry.Text())
			}
		})

		actions.Append(entry)

		button := gtk.NewButton()
		button.SetIconName("media-playback-start-symbolic")
		button.AddCSSClass("suggested-action")
		button.AddCSSClass("circular")
		button.ConnectClicked(func() {
			if text := entry.Text(); strings.TrimSpace(text) != "" {
				log.Println("Using link", entry.Text())
			}
		})

		actions.Append(button)

		status.SetChild(actions)

		clamp.SetChild(status)

		box.Append(clamp)

		stack.AddChild(box)

		handle.SetChild(stack)

		window.SetContent(handle)
		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
