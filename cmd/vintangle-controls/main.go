package main

import (
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	_ "embed"
)

var (
	//go:embed controls.ui
	controlsUI string

	//go:embed style.css
	styleCSS string
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglecontrols", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.StyleManager().SetColorScheme(adw.ColorSchemePreferDark)

	prov := gtk.NewCSSProvider()
	prov.LoadFromData(styleCSS)

	app.ConnectActivate(func() {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
		)

		builder := gtk.NewBuilderFromString(controlsUI, len(controlsUI))

		window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)

		app.AddWindow(&window.Window)

		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
