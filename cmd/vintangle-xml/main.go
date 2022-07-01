package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	_ "embed"
)

type media struct {
	name string
	size int
}

var (
	//go:embed assistant.ui
	assistantUI string

	files = []media{
		{
			name: "movie.mkv",
			size: 2200000000,
		},
		{
			name: "extras.mp4",
			size: 130000000,
		},
	}
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
		mediaSelectionGroup := builder.GetObject("media-selection-group").Cast().(*adw.PreferencesGroup)
		selectedMedia := ""

		stack.ConnectShow(func() {
			stack.SetVisibleChildName(WELCOME_PAGE_NAME)
		})

		magnetLinkEntry.ConnectChanged(func() {
			selectedMedia = ""

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
				if selectedMedia == "" {
					nextButton.SetSensitive(false)
				}

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

		mediaRows := []*adw.ActionRow{}
		mediaSelectionGroup.ConnectRealize(func() {
			for _, row := range mediaRows {
				mediaSelectionGroup.Remove(row)
			}
			mediaRows = []*adw.ActionRow{}

			var lastActivator *gtk.CheckButton
			for _, file := range files {
				row := adw.NewActionRow()

				activator := gtk.NewCheckButton()
				if activator != nil {
					activator.SetGroup(lastActivator)
				}
				lastActivator = activator

				m := file.name
				activator.SetActive(false)
				activator.ConnectActivate(func() {
					selectedMedia = m

					nextButton.SetSensitive(true)
				})

				row.SetTitle(file.name)
				row.SetSubtitle(fmt.Sprintf("%v MB", file.size/1000/1000))
				row.SetActivatable(true)

				row.AddPrefix(activator)
				row.SetActivatableWidget(activator)

				mediaRows = append(mediaRows, row)
				mediaSelectionGroup.Add(row)
			}
		})

		app.AddWindow(&window.Window)

		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
