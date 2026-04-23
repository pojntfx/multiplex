package components

import (
	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"codeberg.org/puregotk/puregotk/v4/adw"
)

// NewShortcutsDialog builds an Adw.ShortcutsDialog with all known application
// shortcuts. Items that use SetActionName automatically pick up their current
// accelerator from the application's action map.
func NewShortcutsDialog() *adw.ShortcutsDialog {
	dlg := adw.NewShortcutsDialog()

	general := adw.NewShortcutsSection(L("General"))

	openMenu := adw.NewShortcutsItem(L("Open Menu"), "F10")
	general.Add(openMenu)

	showShortcuts := adw.NewShortcutsItemFromAction(L("Show Keyboard Shortcuts"), "win.shortcuts")
	general.Add(showShortcuts)

	closeWindow := adw.NewShortcutsItemFromAction(L("Close Window"), "win.closeWindow")
	general.Add(closeWindow)

	quit := adw.NewShortcutsItemFromAction(L("Quit"), "app.quit")
	general.Add(quit)

	dlg.Add(general)

	playback := adw.NewShortcutsSection(L("Playback"))

	togglePlayback := adw.NewShortcutsItemFromAction(L("Toggle Playback"), "win.togglePlayback")
	playback.Add(togglePlayback)

	toggleFullscreen := adw.NewShortcutsItemFromAction(L("Toggle Fullscreen"), "win.toggleFullscreen")
	playback.Add(toggleFullscreen)

	dlg.Add(playback)

	return dlg
}
