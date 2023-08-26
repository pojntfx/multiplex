package components

import (
	"context"
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/pojntfx/multiplex/internal/ressources"
	"github.com/rs/zerolog/log"
)

const (
	issuesURL = "https://github.com/pojntfx/multiplex/issues"
)

func OpenErrorDialog(ctx context.Context, window *adw.ApplicationWindow, err error) {
	log.Error().
		Err(err).
		Msg("Could not continue due to a fatal error")

	errorBuilder := gtk.NewBuilderFromString(ressources.ErrorUI, len(ressources.ErrorUI))
	errorDialog := errorBuilder.GetObject("error-dialog").Cast().(*gtk.MessageDialog)
	reportErrorButton := errorBuilder.GetObject("report-error-button").Cast().(*gtk.Button)
	closeMultiplexButton := errorBuilder.GetObject("close-multiplex-button").Cast().(*gtk.Button)

	errorDialog.Object.SetObjectProperty("secondary-text", err.Error())

	errorDialog.SetDefaultWidget(reportErrorButton)
	errorDialog.SetTransientFor(&window.Window)
	errorDialog.ConnectCloseRequest(func() (ok bool) {
		errorDialog.Close()
		errorDialog.SetVisible(false)

		return ok
	})

	reportErrorButton.ConnectClicked(func() {
		gtk.ShowURIFull(ctx, &window.Window, issuesURL, gdk.CURRENT_TIME, func(res gio.AsyncResulter) {
			errorDialog.Close()

			os.Exit(1)
		})
	})

	closeMultiplexButton.ConnectClicked(func() {
		errorDialog.Close()

		os.Exit(1)
	})

	errorDialog.Show()
}
