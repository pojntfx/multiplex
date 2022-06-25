package main

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func formatDuration(duration time.Duration) string {
	hours := math.Floor(duration.Hours())
	minutes := math.Floor(duration.Minutes()) - (hours * 60)
	seconds := math.Floor(duration.Seconds()) - (minutes * 60) - (hours * 3600)

	return fmt.Sprintf("%02d:%02d:%02d", int(hours), int(minutes), int(seconds))
}

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglecontrols", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.StyleManager().SetColorScheme(adw.ColorSchemePreferDark)

	prov := gtk.NewCSSProvider()
	prov.ConnectParsingError(func(section *gtk.CSSSection, err error) {
		panic(err)
	})
	prov.LoadFromData(`.tabular-nums {
  font-variant-numeric: tabular-nums;
}`)

	app.ConnectActivate(func() {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
		)

		window := adw.NewApplicationWindow(&app.Application)
		window.SetTitle("Vintangle - movie.mkv")
		window.SetDefaultSize(700, 100)
		window.SetResizable(false)

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

		controls := gtk.NewBox(gtk.OrientationHorizontal, 6)
		controls.SetHAlign(gtk.AlignFill)
		controls.SetVAlign(gtk.AlignCenter)
		controls.SetVExpand(true)
		controls.SetMarginTop(0)
		controls.SetMarginStart(18)
		controls.SetMarginEnd(18)
		controls.SetMarginBottom(24)

		playPauseButton := gtk.NewButtonFromIconName("media-playback-start-symbolic")
		playPauseButton.AddCSSClass("flat")

		controls.Append(playPauseButton)

		stopButton := gtk.NewButtonFromIconName("media-playback-stop-symbolic")
		stopButton.AddCSSClass("flat")

		controls.Append(stopButton)

		total, err := time.ParseDuration("2h")
		if err != nil {
			panic(err)
		}

		leftTrack := gtk.NewLabel(formatDuration(time.Duration(0)))
		leftTrack.SetMarginStart(12)
		leftTrack.AddCSSClass("tabular-nums")

		controls.Append(leftTrack)

		rightTrack := gtk.NewLabel(formatDuration(total))
		rightTrack.SetMarginEnd(12)
		rightTrack.AddCSSClass("tabular-nums")

		seeker := gtk.NewScale(gtk.OrientationHorizontal, nil)
		seeker.SetRange(0, float64(total.Nanoseconds()))
		seeker.SetHExpand(true)
		seeker.ConnectChangeValue(func(scroll gtk.ScrollType, value float64) (ok bool) {
			seeker.SetValue(value)

			elapsed := time.Duration(int64(value))
			remaining := total - elapsed

			leftTrack.SetLabel(formatDuration(elapsed))
			rightTrack.SetLabel("-" + formatDuration(remaining))

			return true
		})

		controls.Append(seeker)

		controls.Append(rightTrack)

		volumeButton := gtk.NewVolumeButton()
		volumeButton.AddCSSClass("circular")

		controls.Append(volumeButton)

		fullscreenButton := gtk.NewButtonFromIconName("view-fullscreen-symbolic")
		fullscreenButton.AddCSSClass("flat")

		controls.Append(fullscreenButton)

		controlsPage.Append(controls)

		stack.AddChild(controlsPage)

		handle.SetChild(stack)

		window.SetContent(handle)
		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
