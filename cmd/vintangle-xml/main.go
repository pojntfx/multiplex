package main

import (
	"fmt"
	"os"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
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

	//go:embed controls.ui
	controlsUI string

	//go:embed style.css
	styleCSS string

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
	welcomePageName = "welcome-page"
	mediaPageName   = "media-page"
	readyPageName   = "ready-page"

	playIcon  = "media-playback-start-symbolic"
	pauseIcon = "media-playback-pause-symbolic"
)

func openAssistantWindow(app *adw.Application) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemeDefault)

	builder := gtk.NewBuilderFromString(assistantUI, len(assistantUI))

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	headerbarPopover := builder.GetObject("headerbar-popover").Cast().(*gtk.Popover)
	headerbarTitle := builder.GetObject("headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarTitle := builder.GetObject("button-headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarSubtitle := builder.GetObject("button-headerbar-subtitle").Cast().(*gtk.Label)
	headerbarReadme := builder.GetObject("headerbar-readme").Cast().(*gtk.TextView)
	previousButton := builder.GetObject("previous-button").Cast().(*gtk.Button)
	nextButton := builder.GetObject("next-button").Cast().(*gtk.Button)
	headerbarSpinner := builder.GetObject("headerbar-spinner").Cast().(*gtk.Spinner)
	stack := builder.GetObject("stack").Cast().(*gtk.Stack)
	magnetLinkEntry := builder.GetObject("magnet-link-entry").Cast().(*gtk.Entry)
	mediaSelectionGroup := builder.GetObject("media-selection-group").Cast().(*adw.PreferencesGroup)
	rightsConfirmationButton := builder.GetObject("rights-confirmation-button").Cast().(*gtk.CheckButton)
	playButton := builder.GetObject("play-button").Cast().(*gtk.Button)
	mediaInfoDisplay := builder.GetObject("media-info-display").Cast().(*gtk.Box)
	mediaInfoButton := builder.GetObject("media-info-button").Cast().(*gtk.Button)

	selectedTorrent := "Sintel (2010)"
	selectedMedia := ""
	selectedReadme := `A lonely young woman, Sintel, helps and befriends a dragon, whom she calls Scales. But when he is kidnapped by an adult dragon, Sintel decides to embark on a dangerous quest to find her lost friend Scales.`

	activators := []*gtk.CheckButton{}

	stack.SetVisibleChildName(welcomePageName)

	magnetLinkEntry.ConnectChanged(func() {
		selectedMedia = ""
		for _, activator := range activators {
			activator.SetActive(false)
		}

		if magnetLinkEntry.Text() == "" {
			nextButton.SetSensitive(false)

			return
		}

		nextButton.SetSensitive(true)
	})

	onNext := func() {
		switch stack.VisibleChildName() {
		case welcomePageName:
			if selectedMedia == "" {
				nextButton.SetSensitive(false)
			}

			headerbarSpinner.SetSpinning(true)

			go func() {
				time.AfterFunc(time.Millisecond*100, func() {
					headerbarSpinner.SetSpinning(false)

					previousButton.SetVisible(true)

					headerbarTitle.SetLabel(selectedTorrent)
					buttonHeaderbarTitle.SetLabel(selectedTorrent)

					stack.SetVisibleChildName(mediaPageName)
				})
			}()
		case mediaPageName:
			nextButton.SetVisible(false)

			buttonHeaderbarSubtitle.SetVisible(true)
			buttonHeaderbarSubtitle.SetLabel(selectedMedia)

			mediaInfoDisplay.SetVisible(false)
			mediaInfoButton.SetVisible(true)

			headerbarReadme.SetWrapMode(gtk.WrapWord)
			headerbarReadme.Buffer().SetText(selectedReadme)

			stack.SetVisibleChildName(readyPageName)
		}
	}

	onPrevious := func() {
		switch stack.VisibleChildName() {
		case mediaPageName:
			previousButton.SetVisible(false)
			nextButton.SetSensitive(true)

			headerbarTitle.SetLabel("Welcome")

			stack.SetVisibleChildName(welcomePageName)
		case readyPageName:
			nextButton.SetVisible(true)

			headerbarTitle.SetLabel(selectedTorrent)
			buttonHeaderbarTitle.SetLabel(selectedTorrent)

			mediaInfoDisplay.SetVisible(true)
			mediaInfoButton.SetVisible(false)

			stack.SetVisibleChildName(mediaPageName)
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

		activators = []*gtk.CheckButton{}
		for i, file := range files {
			row := adw.NewActionRow()

			activator := gtk.NewCheckButton()

			if len(activators) > 0 {
				activator.SetGroup(activators[i-1])
			}
			activators = append(activators, activator)

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

	headerbarPopover.SetOffset(0, 6)

	mediaInfoButton.ConnectClicked(func() {
		headerbarPopover.SetVisible(!headerbarPopover.Visible())
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

		if err := openControlsWindow(app, selectedTorrent, selectedMedia, selectedReadme); err != nil {
			panic(err)
		}
	})

	app.AddWindow(&window.Window)

	window.Show()

	return nil
}

func openControlsWindow(app *adw.Application, selectedTorrent, selectedMedia, selectedReadme string) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemePreferDark)

	builder := gtk.NewBuilderFromString(controlsUI, len(controlsUI))

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	headerbarPopover := builder.GetObject("headerbar-popover").Cast().(*gtk.Popover)
	buttonHeaderbarTitle := builder.GetObject("button-headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarSubtitle := builder.GetObject("button-headerbar-subtitle").Cast().(*gtk.Label)
	headerbarReadme := builder.GetObject("headerbar-readme").Cast().(*gtk.TextView)
	playButton := builder.GetObject("play-button").Cast().(*gtk.Button)
	stopButton := builder.GetObject("stop-button").Cast().(*gtk.Button)
	mediaInfoButton := builder.GetObject("media-info-button").Cast().(*gtk.Button)

	buttonHeaderbarTitle.SetLabel(selectedTorrent)
	buttonHeaderbarSubtitle.SetLabel(selectedMedia)

	playButton.ConnectClicked(func() {
		if playButton.IconName() == playIcon {
			playButton.SetIconName(pauseIcon)

			return
		}

		playButton.SetIconName(playIcon)
	})

	stopButton.ConnectClicked(func() {
		window.Close()

		if err := openAssistantWindow(app); err != nil {
			panic(err)
		}
	})

	headerbarPopover.SetOffset(0, 6)

	mediaInfoButton.ConnectClicked(func() {
		headerbarPopover.SetVisible(!headerbarPopover.Visible())
	})

	headerbarReadme.SetWrapMode(gtk.WrapWord)
	headerbarReadme.Buffer().SetText(selectedReadme)

	app.AddWindow(&window.Window)

	window.ConnectShow(func() {
		playButton.GrabFocus()
	})

	window.Show()

	return nil
}

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglexml", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	prov := gtk.NewCSSProvider()
	prov.LoadFromData(styleCSS)

	app.ConnectActivate(func() {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
		)

		if err := openAssistantWindow(app); err != nil {
			panic(err)
		}
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
