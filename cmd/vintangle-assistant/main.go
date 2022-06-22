package main

import (
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type page struct {
	title  string
	widget *gtk.Widget
}

func createClamp(withMargins bool) *adw.Clamp {
	clamp := adw.NewClamp()
	clamp.SetMaximumSize(600)
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

		// Welcome page
		welcomePage := gtk.NewBox(gtk.OrientationVertical, 6)

		welcomePageClamp := createClamp(false)

		welcomeStatus := adw.NewStatusPage()
		welcomeStatus.SetMarginStart(12)
		welcomeStatus.SetMarginEnd(12)
		welcomeStatus.SetIconName("multimedia-player-symbolic")
		welcomeStatus.SetTitle("Vintangle")
		welcomeStatus.SetDescription("Enter a magnet link to start streaming")

		welcomePageClamp.SetChild(welcomeStatus)

		welcomePage.Append(welcomePageClamp)

		// Media page
		mediaPage := gtk.NewBox(gtk.OrientationVertical, 6)

		mediaPageClamp := createClamp(true)

		calendar := gtk.NewCalendar()

		mediaPageClamp.SetChild(calendar)

		mediaPage.Append(mediaPageClamp)

		// Ready page
		readyPage := gtk.NewBox(gtk.OrientationVertical, 6)

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

		nextButton := gtk.NewButtonWithLabel("Next")
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
