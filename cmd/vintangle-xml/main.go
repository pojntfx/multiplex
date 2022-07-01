package main

import (
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	_ "embed"
)

var (
	//go:embed assistant.ui
	assistantUI string
)

const (
	WELCOME_PAGE_NAME = "welcome-page"
	MEDIA_PAGE_NAME   = "media-page"
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglexml", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.ConnectActivate(func() {
		builder := gtk.NewBuilderFromString(assistantUI, len(assistantUI))

		window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
		previousButton := builder.GetObject("previous-button").Cast().(*gtk.Button)
		nextButton := builder.GetObject("next-button").Cast().(*gtk.Button)
		headerbarTitle := builder.GetObject("headerbar-title").Cast().(*gtk.Label)
		headerbarSpinner := builder.GetObject("headerbar-spinner").Cast().(*gtk.Spinner)
		stack := builder.GetObject("stack").Cast().(*gtk.Stack)
		magnetLinkEntry := builder.GetObject("magnet-link-entry").Cast().(*gtk.Entry)

		stack.ConnectShow(func() {
			stack.SetVisibleChildName(WELCOME_PAGE_NAME)
		})

		magnetLinkEntry.ConnectChanged(func() {
			if magnetLinkEntry.Text() == "" {
				nextButton.SetSensitive(false)

				return
			} else {
				nextButton.SetSensitive(true)

				return
			}
		})

		onNavigateToMediaPage := func() {
			if text := magnetLinkEntry.Text(); strings.TrimSpace(text) != "" {
				nextButton.SetSensitive(false)
				headerbarSpinner.SetSpinning(true)

				go func() {
					time.AfterFunc(time.Second, func() {
						headerbarSpinner.SetSpinning(false)

						previousButton.SetVisible(true)
						headerbarTitle.SetText("Media")

						stack.SetVisibleChildName(MEDIA_PAGE_NAME)
					})
				}()
			}
		}

		onNavigateToWelcomePage := func() {
			previousButton.SetVisible(false)
			headerbarTitle.SetText("Welcome")
			nextButton.SetSensitive(true)

			stack.SetVisibleChildName(WELCOME_PAGE_NAME)
		}

		magnetLinkEntry.ConnectActivate(onNavigateToMediaPage)
		nextButton.ConnectClicked(onNavigateToMediaPage)
		previousButton.ConnectClicked(onNavigateToWelcomePage)

		app.AddWindow(&window.Window)

		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
