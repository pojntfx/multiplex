package main

import (
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	_ "embed"
)

var (
	//go:embed assistant.ui
	assistantUI string
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglexml", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.ConnectActivate(func() {
		builder := gtk.NewBuilderFromString(assistantUI, len(assistantUI))

		window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)

		app.AddWindow(&window.Window)

		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
