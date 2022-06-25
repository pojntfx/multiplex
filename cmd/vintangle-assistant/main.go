package main

import (
	"log"
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
		window.SetDefaultSize(1024, 680)

		mainStack := gtk.NewStack()
		mainStack.SetTransitionType(gtk.StackTransitionTypeCrossfade)

		// Header
		currentPage := 0
		selectedFile := ""

		assistantHeader := adw.NewHeaderBar()
		assistantHeader.AddCSSClass("flat")

		assistantSpinner := gtk.NewSpinner()
		assistantSpinner.SetMarginEnd(6)

		nextButton := gtk.NewButtonWithLabel("Next")
		nextButton.SetSensitive(false)
		nextButton.AddCSSClass("suggested-action")

		previousButton := gtk.NewButtonWithLabel("Previous")

		playButton := gtk.NewButtonWithLabel("Play")
		confirmationCheckbox := gtk.NewCheckButtonWithLabel("I have the right to stream the selected media")

		revokePlayConsent := func() {
			playButton.SetSensitive(false)
			playButton.RemoveCSSClass("suggested-action")

			confirmationCheckbox.SetActive(false)
		}

		var onSubmitMagnetLink func(onSuccess func())
		var pages []page

		assistantStack := gtk.NewStack()
		assistantStack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRight)

		onNavigateNext := func() {
			done := make(chan bool)
			if currentPage == 0 {
				onSubmitMagnetLink(func() {
					done <- true
				})
			} else {
				go func() {
					done <- true
				}()
			}

			currentPage++

			go func() {
				if success := <-done; !success {
					return
				}

				assistantStack.SetVisibleChild(pages[currentPage].widget)

				entryHeaderTitle := gtk.NewLabel(pages[currentPage].title)
				entryHeaderTitle.AddCSSClass("title")

				assistantHeader.SetTitleWidget(entryHeaderTitle)

				if currentPage >= len(pages)-1 {
					nextButton.Hide()
				} else {
					previousButton.Show()
				}
			}()
		}

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
		onSubmitMagnetLink = func(onSuccess func()) {
			if selectedFile == "" {
				nextButton.SetSensitive(false)
			}
			magnetLinkEntry.SetSensitive(false)
			assistantSpinner.SetSpinning(true)

			go func() {
				time.Sleep(100 * time.Millisecond)

				magnetLinkEntry.SetSensitive(true)

				assistantSpinner.SetSpinning(false)

				onSuccess()
			}()
		}

		activators := []*gtk.CheckButton{}
		magnetLinkEntry.SetPlaceholderText("Magnet link")
		magnetLinkEntry.ConnectChanged(func() {
			if text := magnetLinkEntry.Text(); strings.TrimSpace(text) != "" {
				nextButton.SetSensitive(true)
			} else {
				nextButton.SetSensitive(false)
			}

			selectedFile = ""
			for _, activator := range activators {
				activator.SetActive(false)
			}

			revokePlayConsent()
		})
		magnetLinkEntry.ConnectActivate(func() {
			if text := magnetLinkEntry.Text(); strings.TrimSpace(text) != "" {
				onNavigateNext()
			}
		})

		welcomeStatus.SetChild(magnetLinkEntry)

		welcomePageClamp.SetChild(welcomeStatus)

		welcomePage.Append(welcomePageClamp)

		// Media page
		mediaPageClamp := createClamp(600, false)

		mediaStatus := adw.NewStatusPage()
		mediaStatus.SetMarginStart(12)
		mediaStatus.SetMarginEnd(12)
		mediaStatus.SetIconName("applications-multimedia-symbolic")
		mediaStatus.SetTitle("Media")
		mediaStatus.SetDescription("Select the file you want to play")

		mediaPreferencesGroup := adw.NewPreferencesGroup()
		for i, file := range []string{"poster.png", "description.txt", "movie.mkv"} {
			row := adw.NewActionRow()

			activator := gtk.NewCheckButton()
			if i > 0 {
				activator.SetGroup(activators[i-1])
			}
			activators = append(activators, activator)

			activator.SetActive(false)

			row.SetTitle(file)
			row.SetActivatable(true)

			row.AddPrefix(activator)
			row.SetActivatableWidget(activator)

			f := file
			activator.ConnectActivate(func() {
				nextButton.SetSensitive(true)
				revokePlayConsent()

				selectedFile = f

				log.Println("Selected file", selectedFile)
			})

			mediaPreferencesGroup.Add(row)
		}

		mediaStatus.SetChild(mediaPreferencesGroup)

		mediaPageClamp.SetChild(mediaStatus)

		mediaPage.Append(mediaPageClamp)

		// Ready page
		readyPageClamp := createClamp(295, false)

		readyStatus := adw.NewStatusPage()
		readyStatus.SetMarginStart(12)
		readyStatus.SetMarginEnd(12)
		readyStatus.SetIconName("emblem-ok-symbolic")
		readyStatus.SetTitle("You're all set!")

		readyActions := gtk.NewBox(gtk.OrientationVertical, 12)
		readyActions.SetHAlign(gtk.AlignCenter)
		readyActions.SetVAlign(gtk.AlignCenter)

		confirmationCheckbox.ConnectToggled(func() {
			if confirmationCheckbox.Active() {
				playButton.SetSensitive(true)
				playButton.AddCSSClass("suggested-action")
			} else {
				revokePlayConsent()
			}
		})

		playButton.SetSensitive(false)
		playButton.AddCSSClass("pill")
		playButton.SetHAlign(gtk.AlignCenter)
		playButton.SetMarginTop(24)

		readyActions.Append(confirmationCheckbox)
		readyActions.Append(playButton)

		readyStatus.SetChild(readyActions)

		readyPageClamp.SetChild(readyStatus)

		readyPage.Append(readyPageClamp)

		// Stack
		pages = []page{
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

		// Assistant layout
		nextButton.ConnectClicked(onNavigateNext)

		previousButton.ConnectClicked(func() {
			currentPage--

			assistantStack.SetVisibleChild(pages[currentPage].widget)

			entryHeaderTitle := gtk.NewLabel(pages[currentPage].title)
			entryHeaderTitle.AddCSSClass("title")

			assistantHeader.SetTitleWidget(entryHeaderTitle)

			nextButton.SetSensitive(true)

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
