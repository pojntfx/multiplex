package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/phayes/freeport"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	playIcon  = "media-playback-start-symbolic"
	pauseIcon = "media-playback-pause-symbolic"
)

var (
	letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
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

func formatDuration(duration time.Duration) string {
	hours := math.Floor(duration.Hours())
	minutes := math.Floor(duration.Minutes()) - (hours * 60)
	seconds := math.Floor(duration.Seconds()) - (minutes * 60) - (hours * 3600)

	return fmt.Sprintf("%02d:%02d:%02d", int(hours), int(minutes), int(seconds))
}

// See https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go/22892986#22892986
func randSeq(n int) string {
	b := make([]rune, n)

	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func getStreamURL(base string, magnet, path string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}

	streamSuffix, err := url.Parse("/stream")
	if err != nil {
		return "", err
	}

	stream := baseURL.ResolveReference(streamSuffix)

	q := stream.Query()
	q.Set("magnet", magnet)
	q.Set("path", path)
	stream.RawQuery = q.Encode()

	return stream.String(), nil
}

func makeAssistantWindow(app *adw.Application, manager *client.Manager) (*adw.ApplicationWindow, error) {
	app.StyleManager().SetColorScheme(adw.ColorSchemeDefault)

	assistantWindow := adw.NewApplicationWindow(&app.Application)
	assistantWindow.SetTitle("Vintangle")
	assistantWindow.SetDefaultSize(1024, 680)

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
	confirmationCheckbox := gtk.NewCheckButtonWithLabel(" I have the right to stream the selected media")

	revokePlayConsent := func() {
		playButton.SetSensitive(false)
		playButton.RemoveCSSClass("suggested-action")

		confirmationCheckbox.SetActive(false)
	}

	var onSubmitMagnetLink func(onSuccess func(bool))
	var pages []page

	assistantStack := gtk.NewStack()
	assistantStack.SetTransitionType(gtk.StackTransitionTypeSlideLeftRight)

	onNavigateNext := func() {
		done := make(chan bool)
		if currentPage == 0 {
			onSubmitMagnetLink(func(success bool) {
				done <- success
			})
		} else {
			go func() {
				done <- true
			}()
		}

		go func() {
			if success := <-done; !success {
				return
			}

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
	onSubmitMagnetLink = func(onSuccess func(bool)) {
		if selectedFile == "" {
			nextButton.SetSensitive(false)
		}
		magnetLinkEntry.SetSensitive(false)
		assistantSpinner.SetSpinning(true)

		go func() {
			magnetLink := magnetLinkEntry.Text()

			log.Info().
				Str("magnetLink", magnetLink).
				Msg("Getting info for magnet link")

			files, err := manager.GetInfo(magnetLink)
			if err != nil {
				log.Error().Err(err).Msg("Could not get info for magnet link")

				magnetLinkEntry.SetSensitive(true)
				assistantSpinner.SetSpinning(false)

				nextButton.SetSensitive(true)

				onSuccess(false)

				return
			}

			filePreview := []string{}
			for _, f := range files {
				filePreview = append(filePreview, f.Path)
			}

			log.Info().
				Str("magnetLink", magnetLink).
				Strs("files", filePreview).
				Msg("Got info for magnet link")

			magnetLinkEntry.SetSensitive(true)
			assistantSpinner.SetSpinning(false)

			onSuccess(true)
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

			log.Info().Str("path", selectedFile).Msg("Selected file")
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
	playButton.ConnectClicked(func() {
		assistantWindow.Destroy()

		controlsWindow, err := makeControlsWindow(app, manager, magnetLinkEntry.Text(), selectedFile)
		if err != nil {
			panic(err)
		}

		controlsWindow.Show()
	})

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

	assistantWindow.SetContent(mainStack)

	return assistantWindow, nil
}

func makeControlsWindow(app *adw.Application, manager *client.Manager, magnetLink string, path string) (*adw.ApplicationWindow, error) {
	app.StyleManager().SetColorScheme(adw.ColorSchemePreferDark)

	controlsWindow := adw.NewApplicationWindow(&app.Application)
	controlsWindow.SetTitle(fmt.Sprintf("Vintangle - %v", path))
	controlsWindow.SetDefaultSize(700, 100)
	controlsWindow.SetResizable(false)

	controlsWindow.ConnectCloseRequest(func() (ok bool) {
		log.Info().Msg("Stopping playback")

		controlsWindow.Destroy()

		return true
	})

	handle := gtk.NewWindowHandle()
	stack := gtk.NewStack()

	controlsPage := gtk.NewBox(gtk.OrientationVertical, 6)

	header := adw.NewHeaderBar()
	header.AddCSSClass("flat")

	copyButton := gtk.NewButtonFromIconName("edit-copy-symbolic")
	copyButton.AddCSSClass("flat")
	copyButton.SetTooltipText("Copy magnet link to media")
	copyButton.ConnectClicked(func() {
		log.Info().Msg("Copying magnet link to clipboard")

		controlsWindow.Clipboard().SetText(magnetLink)
	})

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

	playPauseButton := gtk.NewButtonFromIconName(playIcon)
	playPauseButton.AddCSSClass("flat")
	playPauseButton.ConnectClicked(func() {
		if playPauseButton.IconName() == playIcon {
			log.Info().Msg("Starting playback")

			playPauseButton.SetIconName(pauseIcon)
		} else {
			log.Info().Msg("Pausing playback")

			playPauseButton.SetIconName(playIcon)
		}
	})

	controls.Append(playPauseButton)

	stopButton := gtk.NewButtonFromIconName("media-playback-stop-symbolic")
	stopButton.AddCSSClass("flat")
	stopButton.ConnectClicked(func() {
		log.Info().Msg("Stopping playback")

		controlsWindow.Destroy()

		assistantWindow, err := makeAssistantWindow(app, manager)
		if err != nil {
			panic(err)
		}

		assistantWindow.Show()
	})

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

		log.Printf("Seeking to %vs", int(elapsed.Seconds()))

		remaining := total - elapsed

		leftTrack.SetLabel(formatDuration(elapsed))
		rightTrack.SetLabel("-" + formatDuration(remaining))

		return true
	})

	controls.Append(seeker)

	controls.Append(rightTrack)

	volumeButton := gtk.NewVolumeButton()
	volumeButton.AddCSSClass("circular")
	volumeButton.ConnectValueChanged(func(value float64) {
		log.Info().Float64("value", value).Msg("Setting volume")
	})

	controls.Append(volumeButton)

	fullscreenButton := gtk.NewButtonFromIconName("view-fullscreen-symbolic")
	fullscreenButton.AddCSSClass("flat")
	fullscreenButton.ConnectClicked(func() {
		log.Info().Msg("Toggling fullscreen")
	})

	controls.Append(fullscreenButton)

	controlsPage.Append(controls)

	stack.AddChild(controlsPage)

	handle.SetChild(stack)

	controlsWindow.SetContent(handle)

	return controlsWindow, nil
}

const (
	verboseFlag = "verbose"
	storageFlag = "storage"
	mpvFlag     = "mpv"

	verboseFlagDefault = 5
	mpvFlagDefault     = "mpv"
)

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintangle", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	storageFlagDefault := filepath.Join(home, ".local", "share", "htorrent", "var", "lib", "htorrent", "data")

	app.AddMainOption(verboseFlag, byte('v'), glib.OptionFlagInMain, glib.OptionArgInt64, fmt.Sprintf(`Verbosity level (0 is disabled, default is info, 7 is trace) (default %v)`, verboseFlagDefault), "")
	app.AddMainOption(storageFlag, byte('s'), glib.OptionFlagInMain, glib.OptionArgString, fmt.Sprintf(`Path to store downloaded torrents in (default "%v")`, storageFlagDefault), "")
	app.AddMainOption(mpvFlag, byte('m'), glib.OptionFlagInMain, glib.OptionArgString, fmt.Sprintf(`Command to launch mpv with (default "%v")`, mpvFlagDefault), "")

	prov := gtk.NewCSSProvider()
	prov.LoadFromData(`.tabular-nums {
  font-variant-numeric: tabular-nums;
}`)

	verbose := int64(verboseFlagDefault)
	storage := storageFlagDefault
	// mpv := mpvFlagDefault

	app.ConnectHandleLocalOptions(func(options *glib.VariantDict) (gint int) {
		if options.Contains(verboseFlag) {
			verbose = options.LookupValue(verboseFlag, glib.NewVariantInt64(0).Type()).Int64()
		}

		if options.Contains(storageFlag) {
			storage = options.LookupValue(storageFlag, glib.NewVariantString("").Type()).String()
		}

		// if options.Contains(mpvFlag) {
		// 	mpv = options.LookupValue(mpvFlag, glib.NewVariantString("").Type()).String()
		// }

		switch verbose {
		case 0:
			zerolog.SetGlobalLevel(zerolog.Disabled)
		case 1:
			zerolog.SetGlobalLevel(zerolog.PanicLevel)
		case 2:
			zerolog.SetGlobalLevel(zerolog.FatalLevel)
		case 3:
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)
		case 4:
			zerolog.SetGlobalLevel(zerolog.WarnLevel)
		case 5:
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		case 6:
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		default:
			zerolog.SetGlobalLevel(zerolog.TraceLevel)
		}

		return -1
	})

	var gateway *server.Gateway
	ctx, cancel := context.WithCancel(context.Background())

	app.ConnectActivate(func() {
		gtk.StyleContextAddProviderForDisplay(
			gdk.DisplayGetDefault(),
			prov,
			gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
		)

		addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
		if err != nil {
			panic(err)
		}

		port, err := freeport.GetFreePort()
		if err != nil {
			panic(err)
		}
		addr.Port = port

		rand.Seed(time.Now().UnixNano())

		apiUsername := randSeq(20)
		apiPassword := randSeq(20)

		gateway = server.NewGateway(
			addr.String(),
			storage,
			apiUsername,
			apiPassword,
			"",
			"",
			verbose > 5,
			func(peers int, total, completed int64, path string) {
				log.Info().
					Int("peers", peers).
					Int64("total", total).
					Int64("completed", completed).
					Str("path", path).
					Msg("Streaming")
			},
			ctx,
		)

		if err := gateway.Open(); err != nil {
			panic(err)
		}

		go func() {
			log.Info().
				Str("address", addr.String()).
				Msg("Gateway listening")

			if err := gateway.Wait(); err != nil {
				panic(err)
			}
		}()

		manager := client.NewManager(
			"http://"+addr.String(),
			apiUsername,
			apiPassword,
			ctx,
		)

		assistantWindow, err := makeAssistantWindow(app, manager)
		if err != nil {
			panic(err)
		}

		assistantWindow.Show()
	})

	app.ConnectShutdown(func() {
		if err := gateway.Close(); err != nil {
			panic(err)
		}

		cancel()
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
