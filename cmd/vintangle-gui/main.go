package main

//go:generate glib-compile-schemas .

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

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

	//go:embed description.ui
	descriptionUI string

	//go:embed menu.ui
	menuUI string

	//go:embed about.ui
	aboutUI string

	//go:embed preferences.ui
	preferencesUI string

	//go:embed style.css
	styleCSS string

	//go:embed gschemas.compiled
	geschemas []byte

	letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	json = jsoniter.ConfigCompatibleWithStandardLibrary

	errKilled = errors.New("signal: killed")
)

const (
	appID   = "com.pojtinger.felicitas.vintangle"
	stateID = appID + ".state"

	welcomePageName = "welcome-page"
	mediaPageName   = "media-page"
	readyPageName   = "ready-page"

	playIcon  = "media-playback-start-symbolic"
	pauseIcon = "media-playback-pause-symbolic"

	readmePlaceholder = "No README found."

	verboseFlag = "verbose"
	storageFlag = "storage"
	mpvFlag     = "mpv"

	keycodeEscape = 66

	schemaDirEnvVar = "GSETTINGS_SCHEMA_DIR"

	preferencesActionName      = "preferences"
	applyPreferencesActionName = "applypreferences"
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

func formatDuration(duration time.Duration) string {
	hours := math.Floor(duration.Hours())
	minutes := math.Floor(duration.Minutes()) - (hours * 60)
	seconds := math.Floor(duration.Seconds()) - (minutes * 60) - (hours * 3600)

	return fmt.Sprintf("%02d:%02d:%02d", int(hours), int(minutes), int(seconds))
}

func getDisplayPathWithoutRoot(p string) string {
	parts := strings.Split(p, "/") // Incoming paths are always UNIX

	if len(parts) < 2 {
		return p
	}

	return filepath.Join(parts[1:]...) // Outgoing paths are OS-specific (display only)
}

func openAssistantWindow(app *adw.Application, manager *client.Manager, apiAddr, apiUsername, apiPassword, mpv string, settings *gio.Settings, gateway *server.Gateway, cancel func()) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemeDefault)

	builder := gtk.NewBuilderFromString(assistantUI, len(assistantUI))

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	overlay := builder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)
	buttonHeaderbarTitle := builder.GetObject("button-headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarSubtitle := builder.GetObject("button-headerbar-subtitle").Cast().(*gtk.Label)
	previousButton := builder.GetObject("previous-button").Cast().(*gtk.Button)
	nextButton := builder.GetObject("next-button").Cast().(*gtk.Button)
	menuButton := builder.GetObject("menu-button").Cast().(*gtk.MenuButton)
	headerbarSpinner := builder.GetObject("headerbar-spinner").Cast().(*gtk.Spinner)
	stack := builder.GetObject("stack").Cast().(*gtk.Stack)
	magnetLinkEntry := builder.GetObject("magnet-link-entry").Cast().(*gtk.Entry)
	mediaSelectionGroup := builder.GetObject("media-selection-group").Cast().(*adw.PreferencesGroup)
	rightsConfirmationButton := builder.GetObject("rights-confirmation-button").Cast().(*gtk.CheckButton)
	playButton := builder.GetObject("play-button").Cast().(*gtk.Button)
	mediaInfoDisplay := builder.GetObject("media-info-display").Cast().(*gtk.Box)
	mediaInfoButton := builder.GetObject("media-info-button").Cast().(*gtk.Button)

	descriptionBuilder := gtk.NewBuilderFromString(descriptionUI, len(descriptionUI))
	descriptionWindow := descriptionBuilder.GetObject("description-window").Cast().(*adw.Window)
	descriptionText := descriptionBuilder.GetObject("description-text").Cast().(*gtk.TextView)

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

				buttonHeaderbarTitle.SetLabel(torrentTitle)

				mediaInfoDisplay.SetVisible(false)
				mediaInfoButton.SetVisible(true)

				descriptionText.SetWrapMode(gtk.WrapWord)
				if !utf8.Valid([]byte(torrentReadme)) || strings.TrimSpace(torrentReadme) == "" {
					descriptionText.Buffer().SetText(readmePlaceholder)
				} else {
					descriptionText.Buffer().SetText(torrentReadme)
				}

				stack.SetVisibleChildName(mediaPageName)
			}()
		case mediaPageName:
			nextButton.SetVisible(false)

			buttonHeaderbarSubtitle.SetVisible(true)
			buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))

			stack.SetVisibleChildName(readyPageName)
		}
	}

	onPrevious := func() {
		switch stack.VisibleChildName() {
		case mediaPageName:
			previousButton.SetVisible(false)
			nextButton.SetSensitive(true)

			mediaInfoDisplay.SetVisible(true)
			mediaInfoButton.SetVisible(false)

			stack.SetVisibleChildName(welcomePageName)
		case readyPageName:
			nextButton.SetVisible(true)

			buttonHeaderbarSubtitle.SetVisible(false)

			stack.SetVisibleChildName(mediaPageName)
		}
	}

	magnetLinkEntry.ConnectActivate(onNext)
	nextButton.ConnectClicked(onNext)
	previousButton.ConnectClicked(onPrevious)

	addPreferencesWindow(app, window, settings, menuButton, overlay, gateway, cancel)

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

			row.SetTitle(getDisplayPathWithoutRoot(file.name))
			row.SetSubtitle(fmt.Sprintf("%v MB", file.size/1000/1000))
			row.SetActivatable(true)

			row.AddPrefix(activator)
			row.SetActivatableWidget(activator)

			mediaRows = append(mediaRows, row)
			mediaSelectionGroup.Add(row)
		}
	})

	mediaInfoButton.ConnectClicked(func() {
		descriptionWindow.Show()
	})

	ctrl := gtk.NewEventControllerKey()
	descriptionWindow.AddController(ctrl)
	descriptionWindow.SetTransientFor(&window.Window)

	descriptionWindow.ConnectCloseRequest(func() (ok bool) {
		descriptionWindow.Close()
		descriptionWindow.SetVisible(false)

		return ok
	})

	ctrl.ConnectKeyReleased(func(keyval, keycode uint, state gdk.ModifierType) {
		if keycode == keycodeEscape {
			descriptionWindow.Close()
			descriptionWindow.SetVisible(false)
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

		if err := openControlsWindow(app, torrentTitle, selectedTorrentMedia, torrentReadme, manager, apiAddr, apiUsername, apiPassword, mpv, magnetLinkEntry.Text(), settings, gateway, cancel); err != nil {
			panic(err)
		}
	})

	app.AddWindow(&window.Window)

	window.ConnectShow(func() {
		magnetLinkEntry.GrabFocus()
	})

	window.Show()

	return nil
}

func openControlsWindow(app *adw.Application, torrentTitle, selectedTorrentMedia, torrentReadme string, manager *client.Manager, apiAddr, apiUsername, apiPassword, mpv, magnetLink string, settings *gio.Settings, gateway *server.Gateway, cancel func()) error {
	app.StyleManager().SetColorScheme(adw.ColorSchemePreferDark)

	builder := gtk.NewBuilderFromString(controlsUI, len(controlsUI))

	window := builder.GetObject("main-window").Cast().(*adw.ApplicationWindow)
	overlay := builder.GetObject("toast-overlay").Cast().(*adw.ToastOverlay)
	buttonHeaderbarTitle := builder.GetObject("button-headerbar-title").Cast().(*gtk.Label)
	buttonHeaderbarSubtitle := builder.GetObject("button-headerbar-subtitle").Cast().(*gtk.Label)
	playButton := builder.GetObject("play-button").Cast().(*gtk.Button)
	stopButton := builder.GetObject("stop-button").Cast().(*gtk.Button)
	volumeButton := builder.GetObject("volume-button").Cast().(*gtk.VolumeButton)
	fullscreenButton := builder.GetObject("fullscreen-button").Cast().(*gtk.ToggleButton)
	mediaInfoButton := builder.GetObject("media-info-button").Cast().(*gtk.Button)
	menuButton := builder.GetObject("menu-button").Cast().(*gtk.MenuButton)
	copyButton := builder.GetObject("copy-button").Cast().(*gtk.Button)
	elapsedTrackLabel := builder.GetObject("elapsed-track-label").Cast().(*gtk.Label)
	remainingTrackLabel := builder.GetObject("remaining-track-label").Cast().(*gtk.Label)
	seeker := builder.GetObject("seeker").Cast().(*gtk.Scale)

	descriptionBuilder := gtk.NewBuilderFromString(descriptionUI, len(descriptionUI))
	descriptionWindow := descriptionBuilder.GetObject("description-window").Cast().(*adw.Window)
	descriptionText := descriptionBuilder.GetObject("description-text").Cast().(*gtk.TextView)

	buttonHeaderbarTitle.SetLabel(torrentTitle)
	buttonHeaderbarSubtitle.SetLabel(getDisplayPathWithoutRoot(selectedTorrentMedia))

	copyButton.ConnectClicked(func() {
		window.Clipboard().SetText(magnetLink)
	})

	stopButton.ConnectClicked(func() {
		window.Close()

		if err := openAssistantWindow(app, manager, apiAddr, apiUsername, apiPassword, mpv, settings, gateway, cancel); err != nil {
			panic(err)
		}
	})

	mediaInfoButton.ConnectClicked(func() {
		descriptionWindow.Show()
	})

	ctrl := gtk.NewEventControllerKey()
	descriptionWindow.AddController(ctrl)
	descriptionWindow.SetTransientFor(&window.Window)

	descriptionWindow.ConnectCloseRequest(func() (ok bool) {
		descriptionWindow.Close()
		descriptionWindow.SetVisible(false)

		return ok
	})

	ctrl.ConnectKeyReleased(func(keyval, keycode uint, state gdk.ModifierType) {
		if keycode == keycodeEscape {
			descriptionWindow.Close()
			descriptionWindow.SetVisible(false)
		}
	})

	descriptionText.SetWrapMode(gtk.WrapWord)
	if !utf8.Valid([]byte(torrentReadme)) || strings.TrimSpace(torrentReadme) == "" {
		descriptionText.Buffer().SetText(readmePlaceholder)
	} else {
		descriptionText.Buffer().SetText(torrentReadme)
	}

	usernameAndPassword := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v:%v", apiUsername, apiPassword)))

	streamURL, err := getStreamURL(apiAddr, magnetLink, selectedTorrentMedia)
	if err != nil {
		panic(err)
	}

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}

	vintangleCacheDir := filepath.Join(userCacheDir, "vintangle")
	if err := os.MkdirAll(vintangleCacheDir, os.ModePerm); err != nil {
		panic(err)
	}

	ipcDir, err := os.MkdirTemp(vintangleCacheDir, "mpv-ipc")
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

	addPreferencesWindow(app, window, settings, menuButton, overlay, gateway, func() {
		cancel()

		if command.Process != nil {
			if err := command.Process.Kill(); err != nil {
				panic(err)
			}
		}
	})

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

			if err := os.RemoveAll(ipcDir); err != nil {
				panic(err)
			}

			return true
		})

		var sock net.Conn
		for {
			sock, err = net.Dial("unix", ipcFile)
			if err == nil {
				break
			}

			time.Sleep(time.Millisecond * 100)

			log.Error().
				Str("path", ipcFile).
				Err(err).
				Msg("Could not dial IPC socket, retrying in 100ms")
		}

		encoder := json.NewEncoder(sock)
		if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "volume", 100}}); err != nil {
			panic(err)
		}

		seekerIsSeeking := false
		seekerIsUnderPointer := false
		total := time.Duration(0)

		ctrl := gtk.NewEventControllerMotion()
		ctrl.ConnectEnter(func(x, y float64) {
			seekerIsUnderPointer = true
		})
		ctrl.ConnectLeave(func() {
			seekerIsUnderPointer = false
		})
		seeker.AddController(ctrl)

		seeker.ConnectChangeValue(func(scroll gtk.ScrollType, value float64) (ok bool) {
			seekerIsSeeking = true

			seeker.SetValue(value)

			elapsed := time.Duration(int64(value))

			if err := encoder.Encode(mpvCommand{[]interface{}{"seek", int64(elapsed.Seconds()), "absolute"}}); err != nil {
				panic(err)
			}

			log.Info().
				Dur("duration", elapsed).
				Msg("Seeking")

			remaining := total - elapsed

			elapsedTrackLabel.SetLabel(formatDuration(elapsed))
			remainingTrackLabel.SetLabel("-" + formatDuration(remaining))

			var updateScalePosition func(done bool)
			updateScalePosition = func(done bool) {
				if seekerIsUnderPointer {
					if done {
						return
					}

					updateScalePosition(true)
				} else {
					seekerIsSeeking = false
				}
			}

			time.AfterFunc(
				time.Millisecond*200,
				func() {
					updateScalePosition(false)
				},
			)

			return true
		})

		done := make(chan struct{})
		go func() {
			t := time.NewTicker(time.Millisecond * 100)

			updateSeeker := func() {
				encoder := json.NewEncoder(sock)
				decoder := json.NewDecoder(sock)

				if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "duration"}}); err != nil {
					panic(err)
				}

				var durationResponse mpvFloat64Response
				if err := decoder.Decode(&durationResponse); err != nil {
					log.Error().
						Err(err).
						Msg("Could not parse JSON from socket")

					return
				}

				total, err = time.ParseDuration(fmt.Sprintf("%vs", int64(durationResponse.Data)))
				if err != nil {
					panic(err)
				}

				if err := encoder.Encode(mpvCommand{[]interface{}{"get_property", "time-pos"}}); err != nil {
					panic(err)
				}

				var elapsedResponse mpvFloat64Response
				if err := decoder.Decode(&elapsedResponse); err != nil {
					log.Error().Err(err).Msg("Could not parse JSON from socket")

					return
				}

				elapsed, err := time.ParseDuration(fmt.Sprintf("%vs", int64(elapsedResponse.Data)))
				if err != nil {
					panic(err)
				}

				if !seekerIsSeeking {
					seeker.
						SetRange(0, float64(total.Nanoseconds()))
					seeker.
						SetValue(float64(elapsed.Nanoseconds()))

					remaining := total - elapsed

					log.Debug().
						Float64("total", total.Seconds()).
						Float64("elapsed", elapsed.Seconds()).
						Float64("remaining", remaining.Seconds()).
						Msg("Updating scale")

					elapsedTrackLabel.SetLabel(formatDuration(elapsed))
					remainingTrackLabel.SetLabel("-" + formatDuration(remaining))
				}
			}

			for {
				select {
				case <-t.C:
					updateSeeker()
				case <-done:
					return
				}
			}
		}()

		volumeButton.ConnectValueChanged(func(value float64) {
			log.Info().
				Float64("value", value).
				Msg("Setting volume")

			if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "volume", value * 100}}); err != nil {
				panic(err)
			}
		})

		fullscreenButton.ConnectClicked(func() {
			if fullscreenButton.Active() {
				log.Info().Msg("Enabling fullscreen")

				if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "fullscreen", true}}); err != nil {
					panic(err)
				}

				return
			}

			log.Info().Msg("Disabling fullscreen")

			if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "fullscreen", false}}); err != nil {
				panic(err)
			}
		})

		playButton.ConnectClicked(func() {
			if playButton.IconName() == playIcon {
				log.Info().Msg("Starting playback")

				playButton.SetIconName(pauseIcon)

				if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "pause", false}}); err != nil {
					panic(err)
				}

				return
			}

			log.Info().Msg("Pausing playback")

			if err := encoder.Encode(mpvCommand{[]interface{}{"set_property", "pause", true}}); err != nil {
				panic(err)
			}

			playButton.SetIconName(playIcon)
		})

		go func() {
			if err := command.Wait(); err != nil && err.Error() != errKilled.Error() {
				panic(err)
			}

			done <- struct{}{}

			window.Destroy()
		}()

		playButton.GrabFocus()
	})

	window.Show()

	return nil
}

func addPreferencesWindow(app *adw.Application, window *adw.ApplicationWindow, settings *gio.Settings, menuButton *gtk.MenuButton, overlay *adw.ToastOverlay, gateway *server.Gateway, cancel func()) {
	menuBuilder := gtk.NewBuilderFromString(menuUI, len(menuUI))
	menu := menuBuilder.GetObject("main-menu").Cast().(*gio.Menu)

	aboutBuilder := gtk.NewBuilderFromString(aboutUI, len(aboutUI))
	aboutDialog := aboutBuilder.GetObject("about-dialog").Cast().(*gtk.AboutDialog)

	preferencesBuilder := gtk.NewBuilderFromString(preferencesUI, len(preferencesUI))
	preferencesWindow := preferencesBuilder.GetObject("preferences-window").Cast().(*adw.PreferencesWindow)
	storageLocationInput := preferencesBuilder.GetObject("storage-location-input").Cast().(*gtk.Button)
	mpvCommandInput := preferencesBuilder.GetObject("mpv-command-input").Cast().(*gtk.Entry)
	verbosityLevelInput := preferencesBuilder.GetObject("verbosity-level-input").Cast().(*gtk.SpinButton)

	preferencesHaveChanged := false

	preferencesAction := gio.NewSimpleAction(preferencesActionName, nil)
	preferencesAction.ConnectActivate(func(parameter *glib.Variant) {
		preferencesWindow.Show()
	})
	app.SetAccelsForAction(preferencesActionName, []string{`<Primary>comma`})
	window.AddAction(preferencesAction)

	preferencesWindow.SetTransientFor(&window.Window)
	preferencesWindow.ConnectCloseRequest(func() (ok bool) {
		preferencesWindow.Close()
		preferencesWindow.SetVisible(false)

		if preferencesHaveChanged {
			toast := adw.NewToast("Reopen to apply the changes.")
			toast.SetButtonLabel("Reopen")
			toast.SetActionName("win." + applyPreferencesActionName)

			overlay.AddToast(toast)
		}

		preferencesHaveChanged = false

		return ok
	})

	applyPreferencesAction := gio.NewSimpleAction(applyPreferencesActionName, nil)
	applyPreferencesAction.ConnectActivate(func(parameter *glib.Variant) {
		cancel()

		if err := gateway.Close(); err != nil {
			panic(err)
		}

		ex, err := os.Executable()
		if err != nil {
			panic(err)
		}

		if _, err := syscall.ForkExec(
			ex,
			os.Args,
			&syscall.ProcAttr{
				Env:   os.Environ(),
				Files: []uintptr{os.Stdin.Fd(), os.Stdout.Fd(), os.Stderr.Fd()},
			},
		); err != nil {
			panic(err)
		}

		os.Exit(0)
	})
	window.AddAction(applyPreferencesAction)

	storageLocationInput.ConnectClicked(func() {
		filePicker := gtk.NewFileChooserNative(
			"Select storage location",
			&preferencesWindow.Window.Window,
			gtk.FileChooserActionSelectFolder,
			"",
			"")
		filePicker.SetModal(true)
		filePicker.ConnectResponse(func(responseId int) {
			if responseId == int(gtk.ResponseAccept) {
				settings.SetString(storageFlag, filePicker.File().Path())

				preferencesHaveChanged = true
			}

			filePicker.Destroy()
		})

		filePicker.Show()
	})

	settings.Bind(mpvFlag, mpvCommandInput.Object, "text", gio.SettingsBindDefault)

	verbosityLevelInput.SetAdjustment(gtk.NewAdjustment(0, 0, 8, 1, 1, 1))
	settings.Bind(verboseFlag, verbosityLevelInput.Object, "value", gio.SettingsBindDefault)

	mpvCommandInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})
	verbosityLevelInput.ConnectChanged(func() {
		preferencesHaveChanged = true
	})

	aboutAction := gio.NewSimpleAction("about", nil)
	aboutAction.ConnectActivate(func(parameter *glib.Variant) {
		aboutDialog.Show()
	})
	window.AddAction(aboutAction)

	aboutDialog.SetTransientFor(&window.Window)
	aboutDialog.ConnectCloseRequest(func() (ok bool) {
		aboutDialog.Close()
		aboutDialog.SetVisible(false)

		return ok
	})

	menuButton.SetMenuModel(menu)
}

func main() {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "vintangle-gschemas")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(filepath.Join(tmpDir, "gschemas.compiled"), geschemas, os.ModePerm); err != nil {
		panic(err)
	}

	if err := os.Setenv(schemaDirEnvVar, tmpDir); err != nil {
		panic(err)
	}

	settings := gio.NewSettings(stateID)

	if storage := settings.String(storageFlag); strings.TrimSpace(storage) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}

		settings.SetString(storageFlag, filepath.Join(home, ".local", "share", "htorrent", "var", "lib", "htorrent", "data"))
	}

	settings.ConnectChanged(func(key string) {
		if key == verboseFlag {
			verbose := settings.Int64(verboseFlag)

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
		}
	})

	app := adw.NewApplication(appID, gio.ApplicationFlags(gio.ApplicationFlagsNone))

	prov := gtk.NewCSSProvider()
	prov.LoadFromData(styleCSS)

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
			settings.String(storageFlag),
			apiUsername,
			apiPassword,
			"",
			"",
			settings.Int64(verboseFlag) > 5,
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

		if err := openAssistantWindow(app, manager, apiAddr, apiUsername, apiPassword, settings.String(mpvFlag), settings, gateway, cancel); err != nil {
			panic(err)
		}
	})

	app.ConnectShutdown(func() {
		cancel()

		if err := gateway.Close(); err != nil {
			panic(err)
		}
	})

	if code := app.Run(os.Args); code > 0 {
		os.Exit(code)
	}
}
