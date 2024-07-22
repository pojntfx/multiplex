package components

import (
	"context"
	"os"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/pojntfx/multiplex/internal/resources"
	"github.com/rs/zerolog/log"
	"github.com/rymdport/portal/openuri"
)

const (
	issuesURL = "https://github.com/pojntfx/multiplex/issues"
)

func OpenErrorDialog(ctx context.Context, window *adw.ApplicationWindow, err error) {
	log.Error().
		Err(err).
		Msg("Could not continue due to a fatal error")

	errorBuilder := gtk.NewBuilderFromResource(resources.GResourceErrorPath)
	errorDialog := errorBuilder.GetObject("error-dialog").Cast().(*adw.AlertDialog)

	errorDialog.SetBody(err.Error())

	errorDialog.ConnectResponse(func(response string) {
		switch response {
		case "report":
			_ = openuri.OpenURI("", issuesURL, nil)

			errorDialog.Close()

			os.Exit(1)

		default:
			errorDialog.Close()

			os.Exit(1)
		}
	})

	errorDialog.Present(window)
}
