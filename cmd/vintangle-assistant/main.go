package main

import (
	"os"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type page struct {
	title  string
	widget *gtk.Widget
}

func createClamp(maxWidth int, withMargins bool) *adw.Clamp {
	clamp := adw.NewClamp()
	clamp.SetMaximumSize(maxWidth)
	clamp.SetVExpand(true)
	clamp.SetVAlign(gtk.AlignCenter)

	if withMargins {
		clamp.SetMarginStart(12)
		clamp.SetMarginEnd(12)
		clamp.SetMarginBottom(12)
	}

	return clamp
}

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintangle.assistant", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	app.ConnectActivate(func() {
		window := adw.NewApplicationWindow(&app.Application)
		window.SetTitle("Vintangle")
		window.SetDefaultSize(700, 500)

		mainStack := gtk.NewStack()
		mainStack.SetTransitionType(gtk.StackTransitionTypeCrossfade)

		// CSD
		assistantSpinner := gtk.NewSpinner()
		assistantSpinner.SetMarginEnd(6)

		nextButton := gtk.NewButtonWithLabel("Next")

		readyPage := gtk.NewBox(gtk.OrientationVertical, 6)
		welcomePage := gtk.NewBox(gtk.OrientationVertical, 6)
		mediaPage := gtk.NewBox(gtk.OrientationVertical, 6)

		// Welcome page
		welcomePageClamp := createClamp(295, false)

		welcomeStatus := adw.NewStatusPage()
		welcomeStatus.SetMarginStart(12)
		welcomeStatus.SetMarginEnd(12)
		welcomeStatus.SetIconName("multimedia-player-symbolic")
		welcomeStatus.SetTitle("Vintangle")
		welcomeStatus.SetDescription("Enter a magnet link to start streaming")

		magnetLinkEntry := gtk.NewEntry()
		onSubmitMagnetLink := func() {
			nextButton.SetSensitive(false)
			magnetLinkEntry.SetSensitive(false)
			assistantSpinner.SetSpinning(true)

			go func() {
				time.Sleep(2 * time.Second)

				nextButton.SetSensitive(true)
				magnetLinkEntry.SetSensitive(true)
				assistantSpinner.SetSpinning(false)
			}()
		}

		magnetLinkEntry.SetPlaceholderText("Magnet link")
		magnetLinkEntry.ConnectChanged(func() {
			if text := magnetLinkEntry.Text(); strings.TrimSpace(text) != "" {
				nextButton.SetSensitive(true)
			} else {
				nextButton.SetSensitive(false)
			}
		})
		magnetLinkEntry.ConnectActivate(onSubmitMagnetLink)

		welcomeStatus.SetChild(magnetLinkEntry)

		welcomePageClamp.SetChild(welcomeStatus)

		welcomePage.Append(welcomePageClamp)

		// Media page
		mediaPageClamp := createClamp(600, true)

		calendar := gtk.NewCalendar()

		mediaPageClamp.SetChild(calendar)

		mediaPage.Append(mediaPageClamp)

		// Ready page
		assistantStack := gtk.NewStack()
		assistantStack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRight)

		pages := []page{
			{
				title:  "Welcome",
				widget: &welcomePage.Widget,
			},
			{
				title:  "Media",
				widget: &mediaPage.Widget,
			},
			{
				title:  "Ready to Go",
				widget: &readyPage.Widget,
			},
		}

		for _, page := range pages {
			assistantStack.AddChild(page.widget)
		}

		currentPage := 0

		// Assistant layout
		assistantHeader := adw.NewHeaderBar()
		assistantHeader.AddCSSClass("flat")
		nextButton.SetSensitive(false)

		previousButton := gtk.NewButtonWithLabel("Previous")

		nextButton.AddCSSClass("suggested-action")
		nextButton.ConnectClicked(func() {
			currentPage++

			assistantStack.SetVisibleChild(pages[currentPage].widget)

			entryHeaderTitle := gtk.NewLabel(pages[currentPage].title)
			entryHeaderTitle.AddCSSClass("title")

			assistantHeader.SetTitleWidget(entryHeaderTitle)

			if currentPage >= len(pages)-1 {
				nextButton.Hide()
			} else {
				previousButton.Show()
			}
		})

		previousButton.ConnectClicked(func() {
			currentPage--

			assistantStack.SetVisibleChild(pages[currentPage].widget)

			entryHeaderTitle := gtk.NewLabel(pages[currentPage].title)
			entryHeaderTitle.AddCSSClass("title")

			assistantHeader.SetTitleWidget(entryHeaderTitle)

			if currentPage <= 0 {
				previousButton.Hide()
			} else {
				nextButton.Show()
			}
		})

		assistantHeader.PackStart(previousButton)

		assistantHeader.PackEnd(nextButton)
		assistantHeader.PackEnd(assistantSpinner)

		assistantPage := gtk.NewBox(gtk.OrientationVertical, 6)

		assistantPage.Append(assistantHeader)
		assistantPage.Append(assistantStack)

		previousButton.Hide()

		assistantStack.SetVisibleChild(pages[currentPage].widget)

		entryHeaderTitle := gtk.NewLabel(pages[currentPage].title)
		entryHeaderTitle.AddCSSClass("title")

		assistantHeader.SetTitleWidget(entryHeaderTitle)

		mainStack.AddChild(assistantPage)

		window.SetContent(mainStack)
		window.Show()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
