package components

import (
	"context"
	"os"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
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
	var errorDialog adw.AlertDialog
	errorBuilder.GetObject("error-dialog").Cast(&errorDialog)

	errorDialog.SetBody(err.Error())

	responseCallback := func(dialog adw.AlertDialog, response string) {
		switch response {
		case "report":
			_ = openuri.OpenURI("", issuesURL, nil)

			errorDialog.Close()

			os.Exit(1)

		default:
			errorDialog.Close()

			os.Exit(1)
		}
	}
	errorDialog.ConnectResponse(&responseCallback)

	errorDialog.Present(&window.Widget)
}
