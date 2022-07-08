package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	jsoniter "github.com/json-iterator/go"
	"github.com/phayes/freeport"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	_ "embed"
)

type media struct {
	name string
	size int
}

type mpvCommand struct {
	Command []interface{} `json:"command"`
}

type mpvFloat64Response struct {
	Data float64 `json:"data"`
}

var (
	//go:embed assistant.ui
	assistantUI string

	//go:embed controls.ui
	controlsUI string

	//go:embed style.css
	styleCSS string

	letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

const (
	welcomePageName = "welcome-page"
	mediaPageName   = "media-page"
	readyPageName   = "ready-page"

	playIcon  = "media-playback-start-symbolic"
	pauseIcon = "media-playback-pause-symbolic"

	readmePlaceholder = "No README found."

	verboseFlag = "verbose"
	storageFlag = "storage"
	mpvFlag     = "mpv"

	verboseFlagDefault = 5
	mpvFlagDefault     = "mpv"
)

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

func openAssistantWindow(app *adw.Application, manager *client.Manager, apiAddr, apiUsername, apiPassword, mpv string) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemeDefault)

	builder := gtk.NewBuilderFromString(assistantUI, len(assistantUI))

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	overlay := builder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)
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

	torrentTitle := ""
	torrentMedia := []media{}
	torrentReadme := ""

	selectedTorrentMedia := ""
	activators := []*gtk.CheckButton{}

	stack.SetVisibleChildName(welcomePageName)

	magnetLinkEntry.ConnectChanged(func() {
		selectedTorrentMedia = ""
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
			if selectedTorrentMedia == "" {
				nextButton.SetSensitive(false)
			}

			headerbarSpinner.SetSpinning(true)
			magnetLinkEntry.SetSensitive(false)

			go func() {
				magnetLink := magnetLinkEntry.Text()

				log.Info().
					Str("magnetLink", magnetLink).
					Msg("Getting info for magnet link")

				info, err := manager.GetInfo(magnetLink)
				if err != nil {
					log.Warn().
						Str("magnetLink", magnetLink).
						Err(err).
						Msg("Could not get info for magnet link")

					toast := adw.NewToast("Could not get info for this magnet link.")

					overlay.AddToast(toast)

					headerbarSpinner.SetSpinning(false)
					magnetLinkEntry.SetSensitive(true)

					magnetLinkEntry.GrabFocus()

					return
				}

				torrentTitle = info.Name
				torrentReadme = info.Description
				torrentMedia = []media{}
				for _, file := range info.Files {
					torrentMedia = append(torrentMedia, media{
						name: file.Path,
						size: int(file.Length),
					})
				}

				headerbarSpinner.SetSpinning(false)
				magnetLinkEntry.SetSensitive(true)
				previousButton.SetVisible(true)

				headerbarTitle.SetLabel(torrentTitle)
				buttonHeaderbarTitle.SetLabel(torrentTitle)

				stack.SetVisibleChildName(mediaPageName)
			}()
		case mediaPageName:
			nextButton.SetVisible(false)

			buttonHeaderbarSubtitle.SetVisible(true)
			buttonHeaderbarSubtitle.SetLabel(path.Base(selectedTorrentMedia))

			mediaInfoDisplay.SetVisible(false)
			mediaInfoButton.SetVisible(true)

			headerbarReadme.SetWrapMode(gtk.WrapWord)
			if torrentReadme == "" {
				headerbarReadme.Buffer().SetText(readmePlaceholder)
			} else {
				headerbarReadme.Buffer().SetText(torrentReadme)
			}

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

			headerbarTitle.SetLabel(torrentTitle)
			buttonHeaderbarTitle.SetLabel(torrentTitle)

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
		for i, file := range torrentMedia {
			row := adw.NewActionRow()

			activator := gtk.NewCheckButton()

			if len(activators) > 0 {
				activator.SetGroup(activators[i-1])
			}
			activators = append(activators, activator)

			m := file.name
			activator.SetActive(false)
			activator.ConnectActivate(func() {
				if m != selectedTorrentMedia {
					selectedTorrentMedia = m

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

		if err := openControlsWindow(app, torrentTitle, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, mpv, magnetLinkEntry.Text()); err != nil {
			panic(err)
		}
	})

	app.AddWindow(&window.Window)

	window.Show()

	return nil
}

func openControlsWindow(app *adw.Application, torrentTitle, selectedTorrentMedia, torrentReadme string, manager *client.Manager, apiAddr, apiUsername, apiPassword, mpv, magnetLink string) error {
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
	copyButton := builder.GetObject("copy-button").Cast().(*gtk.Button)

	buttonHeaderbarTitle.SetLabel(torrentTitle)
	buttonHeaderbarSubtitle.SetLabel(path.Base(selectedTorrentMedia))

	copyButton.ConnectClicked(func() {
		window.Clipboard().SetText(magnetLink)
	})

	playButton.ConnectClicked(func() {
		if playButton.IconName() == playIcon {
			playButton.SetIconName(pauseIcon)

			return
		}

		playButton.SetIconName(playIcon)
	})

	stopButton.ConnectClicked(func() {
		window.Close()

		if err := openAssistantWindow(app, manager, apiAddr, apiUsername, apiPassword, mpv); err != nil {
			panic(err)
		}
	})

	headerbarPopover.SetOffset(0, 6)

	mediaInfoButton.ConnectClicked(func() {
		headerbarPopover.SetVisible(!headerbarPopover.Visible())
	})

	headerbarReadme.SetWrapMode(gtk.WrapWord)
	if torrentReadme == "" {
		headerbarReadme.Buffer().SetText(readmePlaceholder)
	} else {
		headerbarReadme.Buffer().SetText(torrentReadme)
	}

	usernameAndPassword := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v:%v", apiUsername, apiPassword)))

	streamURL, err := getStreamURL(apiAddr, magnetLink, selectedTorrentMedia)
	if err != nil {
		panic(err)
	}

	ipcDir, err := ioutil.TempDir(os.TempDir(), "mpv-ipc")
	if err != nil {
		panic(err)
	}
	ipcFile := filepath.Join(ipcDir, "mpv.sock")

	shell := []string{"sh", "-c"}
	if runtime.GOOS == "windows" {
		shell = []string{"cmd", "/c"}
	}
	commandLine := append(shell, fmt.Sprintf("%v '--keep-open=always' '--sub-visibility=no' '--no-osc' '--no-input-default-bindings' '--pause' '--input-ipc-server=%v' '--http-header-fields=Authorization: Basic %v' '%v'", mpv, ipcFile, usernameAndPassword, streamURL))

	command := exec.Command(
		commandLine[0],
		commandLine[1:]...,
	)

	app.AddWindow(&window.Window)

	window.ConnectShow(func() {
		if err := command.Start(); err != nil {
			panic(err)
		}

		window.ConnectCloseRequest(func() (ok bool) {
			if command.Process != nil {
				if err := command.Process.Kill(); err != nil {
					panic(err)
				}
			}

			window.Destroy()

			return true
		})

		var sock net.Conn
		var encoder *jsoniter.Encoder
		// var decoder *jsoniter.Decoder

		for {
			sock, err = net.Dial("unix", ipcFile)
			if err == nil {
				encoder = json.NewEncoder(sock)
				// decoder = json.NewDecoder(sock)

				break
			}

			time.Sleep(time.Millisecond * 100)

			log.Error().
				Str("path", ipcFile).
				Err(err).
				Msg("Could not dial IPC socket, retrying in 100ms")
		}

		if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "volume", 100}}); err != nil {
			panic(err)
		}

		playButton.GrabFocus()
	})

	window.Show()

	return nil
}

func main() {
	app := adw.NewApplication("com.pojtinger.felicitas.vintanglexml", gio.ApplicationFlags(gio.ApplicationFlagsNone))

	prov := gtk.NewCSSProvider()
	prov.LoadFromData(styleCSS)

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	storageFlagDefault := filepath.Join(home, ".local", "share", "htorrent", "var", "lib", "htorrent", "data")

	app.AddMainOption(verboseFlag, byte('v'), glib.OptionFlagInMain, glib.OptionArgInt64, fmt.Sprintf(`Verbosity level (0 is disabled, default is info, 7 is trace) (default %v)`, verboseFlagDefault), "")
	app.AddMainOption(storageFlag, byte('s'), glib.OptionFlagInMain, glib.OptionArgString, fmt.Sprintf(`Path to store downloaded torrents in (default "%v")`, storageFlagDefault), "")
	app.AddMainOption(mpvFlag, byte('m'), glib.OptionFlagInMain, glib.OptionArgString, fmt.Sprintf(`Command to launch mpv with (default "%v")`, mpvFlagDefault), "")

	verbose := int64(verboseFlagDefault)
	storage := storageFlagDefault
	mpv := mpvFlagDefault

	app.ConnectHandleLocalOptions(func(options *glib.VariantDict) (gint int) {
		if options.Contains(verboseFlag) {
			verbose = options.LookupValue(verboseFlag, glib.NewVariantInt64(0).Type()).Int64()
		}

		if options.Contains(storageFlag) {
			storage = options.LookupValue(storageFlag, glib.NewVariantString("").Type()).String()
		}

		if options.Contains(mpvFlag) {
			mpv = options.LookupValue(mpvFlag, glib.NewVariantString("").Type()).String()
		}

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

		apiAddr := "http://" + addr.String()
		manager := client.NewManager(
			apiAddr,
			apiUsername,
			apiPassword,
			ctx,
		)

		if err := openAssistantWindow(app, manager, apiAddr, apiUsername, apiPassword, mpv); err != nil {
			panic(err)
		}
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
