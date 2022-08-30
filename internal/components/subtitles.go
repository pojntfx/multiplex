package components

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	mpv "github.com/pojntfx/vintangle/pkg/api/sockets/v1"
	mpvClient "github.com/pojntfx/vintangle/pkg/client"
	"github.com/rs/zerolog/log"
)

func SetSubtitles(
	ctx context.Context,
	window *adw.ApplicationWindow,

	filePath string,
	file io.Reader,
	tmpDir string,
	ipcFile string,

	noneActivator *gtk.CheckButton,
	subtitlesOverlay *adw.ToastOverlay,
) {
	subtitlesDir, err := os.MkdirTemp(tmpDir, "subtitles")
	if err != nil {
		OpenErrorDialog(ctx, window, err)

		return
	}

	subtitlesFile := filepath.Join(subtitlesDir, path.Base(filePath))
	f, err := os.Create(subtitlesFile)
	if err != nil {
		OpenErrorDialog(ctx, window, err)

		return
	}

	if _, err := io.Copy(f, file); err != nil {
		OpenErrorDialog(ctx, window, err)

		return
	}

	if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().
			Str("path", subtitlesFile).
			Msg("Adding subtitles path")

		if err := encoder.Encode(mpv.Request{[]interface{}{"sub-add", subtitlesFile}}); err != nil {
			return err
		}

		var successResponse mpv.ResponseSuccess
		return decoder.Decode(&successResponse)
	}); err != nil {
		OpenErrorDialog(ctx, window, err)

		return
	}

	var trackListResponse mpv.ResponseTrackList
	if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().Msg("Getting tracklist")

		if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "track-list"}}); err != nil {
			return err
		}

		return decoder.Decode(&trackListResponse)
	}); err != nil {
		OpenErrorDialog(ctx, window, err)

		return
	}

	sid := -1
	for _, track := range trackListResponse.Data {
		if track.Type == mpv.TypeSub && track.ExternalFilename == subtitlesFile {
			sid = track.ID

			break
		}
	}

	if sid == -1 {
		log.Info().
			Msg("Disabling subtitles")

		time.AfterFunc(time.Millisecond*100, func() {
			noneActivator.SetActive(true)
		})

		if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
			if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", "no"}}); err != nil {
				return err
			}

			var successResponse mpv.ResponseSuccess
			return decoder.Decode(&successResponse)
		}); err != nil {
			OpenErrorDialog(ctx, window, err)

			return
		}

		toast := adw.NewToast("This file does not contain subtitles.")

		subtitlesOverlay.AddToast(toast)

		return
	}

	if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().
			Str("path", subtitlesFile).
			Int("sid", sid).
			Msg("Setting subtitle ID")

		if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sid", sid}}); err != nil {
			return err
		}

		var successResponse mpv.ResponseSuccess
		return decoder.Decode(&successResponse)
	}); err != nil {
		OpenErrorDialog(ctx, window, err)

		return
	}

	if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sub-visibility", "yes"}}); err != nil {
			return err
		}

		var successResponse mpv.ResponseSuccess
		return decoder.Decode(&successResponse)
	}); err != nil {
		OpenErrorDialog(ctx, window, err)

		return
	}
}
