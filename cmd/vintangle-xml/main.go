package main

import (
	"fmt"
	"os"
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
	READY_PAGE_NAME   = "ready-page"
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglexml", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.ConnectActivate(func() {
		builder := gtk.NewBuilderFromString(assistantUI, len(assistantUI))

		window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
		previousButton := builder.GetObject("previous-button").Cast().(*gtk.Button)
		nextButton := builder.GetObject("next-button").Cast().(*gtk.Button)
		headerbarSpinner := builder.GetObject("headerbar-spinner").Cast().(*gtk.Spinner)
		stack := builder.GetObject("stack").Cast().(*gtk.Stack)
		magnetLinkEntry := builder.GetObject("magnet-link-entry").Cast().(*gtk.Entry)
		mediaSelectionGroup := builder.GetObject("media-selection-group").Cast().(*adw.PreferencesGroup)
		rightsConfirmationButton := builder.GetObject("rights-confirmation-button").Cast().(*gtk.CheckButton)
		playButton := builder.GetObject("play-button").Cast().(*gtk.Button)

		selectedMedia := ""

		stack.ConnectShow(func() {
			stack.SetVisibleChildName(WELCOME_PAGE_NAME)
		})

		magnetLinkEntry.ConnectChanged(func() {
			selectedMedia = ""

			if magnetLinkEntry.Text() == "" {
				nextButton.SetSensitive(false)

				return
			}

			nextButton.SetSensitive(true)
		})

		onNext := func() {
			switch stack.VisibleChildName() {
			case WELCOME_PAGE_NAME:
				if selectedMedia == "" {
					nextButton.SetSensitive(false)
				}

				headerbarSpinner.SetSpinning(true)

				go func() {
					time.AfterFunc(time.Second, func() {
						headerbarSpinner.SetSpinning(false)

						previousButton.SetVisible(true)
						window.SetTitle("Media")

						stack.SetVisibleChildName(MEDIA_PAGE_NAME)
					})
				}()
			case MEDIA_PAGE_NAME:
				nextButton.SetVisible(false)
				window.SetTitle("Ready to Go")

				stack.SetVisibleChildName(READY_PAGE_NAME)
			}
		}

		onPrevious := func() {
			switch stack.VisibleChildName() {
			case MEDIA_PAGE_NAME:
				previousButton.SetVisible(false)
				window.SetTitle("Welcome")
				nextButton.SetSensitive(true)

				stack.SetVisibleChildName(WELCOME_PAGE_NAME)
			case READY_PAGE_NAME:
				nextButton.SetVisible(true)
				window.SetTitle("Media")

				stack.SetVisibleChildName(MEDIA_PAGE_NAME)
			}
		}

		magnetLinkEntry.ConnectActivate(onNext)
		nextButton.ConnectClicked(onNext)
		previousButton.ConnectClicked(onPrevious)

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
					if m != selectedMedia {
						selectedMedia = m

						rightsConfirmationButton.SetActive(false)
					}

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

		rightsConfirmationButton.ConnectToggled(func() {
			if rightsConfirmationButton.Active() {
				playButton.AddCSSClass("suggested-action")
				playButton.SetSensitive(true)

				return
			}

			playButton.RemoveCSSClass("suggested-action")
			playButton.SetSensitive(false)
		})

		playButton.ConnectClicked(func() {
			window.Close()
		})

		app.AddWindow(&window.Window)

		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
