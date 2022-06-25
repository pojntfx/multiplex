package main

import (
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglecontrols", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.ConnectActivate(func() {
		window := adw.NewApplicationWindow(&app.Application)
		window.SetTitle("Vintangle - movie.mkv")
		window.SetDefaultSize(650, 100)

		handle := gtk.NewWindowHandle()
		stack := gtk.NewStack()

		controlsPage := gtk.NewBox(gtk.OrientationVertical, 6)

		header := adw.NewHeaderBar()
		header.AddCSSClass("flat")

		copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
		copyButton.AddCSSClass("flat")
		copyButton.SetTooltipText("Copy magnet link to media")

		header.PackEnd(copyButton)

		controlsPage.Append(header)

		clamp := adw.NewClamp()
		clamp.SetMaximumSize(1500)
		clamp.SetVExpand(true)
		clamp.SetVAlign(gtk.AlignCenter)
		clamp.SetMarginTop(0)
		clamp.SetMarginStart(24)
		clamp.SetMarginEnd(24)
		clamp.SetMarginBottom(24)

		controls := gtk.NewBox(gtk.OrientationHorizontal, 6)
		controls.SetHAlign(gtk.AlignFill)
		controls.SetVAlign(gtk.AlignCenter)

		playPauseButton := gtk.NewButtonFromIconName("media-playback-start-symbolic")
		playPauseButton.AddCSSClass("flat")

		controls.Append(playPauseButton)

		stopButton := gtk.NewButtonFromIconName("media-playback-stop-symbolic")
		stopButton.AddCSSClass("flat")

		controls.Append(stopButton)

		leftTrack := gtk.NewLabel("0:28:15")
		leftTrack.SetMarginStart(12)

		controls.Append(leftTrack)

		seeker := gtk.NewScale(gtk.OrientationHorizontal, nil)
		seeker.SetRange(0, 100)
		seeker.SetHExpand(true)

		controls.Append(seeker)

		rightTrack := gtk.NewLabel("-0:53:54")
		rightTrack.SetMarginEnd(12)

		controls.Append(rightTrack)

		volumeButton := gtk.NewVolumeButton()
		volumeButton.AddCSSClass("circular")

		controls.Append(volumeButton)

		fullscreenButton := gtk.NewButtonFromIconName("view-fullscreen-symbolic")
		fullscreenButton.AddCSSClass("flat")

		controls.Append(fullscreenButton)

		clamp.SetChild(controls)

		controlsPage.Append(clamp)

		stack.AddChild(controlsPage)

		handle.SetChild(stack)

		window.SetContent(handle)
		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
