package utils

import (
	. "github.com/pojntfx/go-gettext/pkg/i18n"

	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/jwijenbergh/puregotk/v4/adw"
	"github.com/jwijenbergh/puregotk/v4/gtk"
	mpv "github.com/pojntfx/multiplex/pkg/api/sockets/v1"
	mpvClient "github.com/pojntfx/multiplex/pkg/client"
	"github.com/rs/zerolog/log"
)

func SetSubtitles(
	filePath string,
	file io.Reader,
	tmpDir string,
	ipcFile string,

	noneActivator *gtk.CheckButton,
	subtitlesOverlay *adw.ToastOverlay,
) error {
	subtitlesDir, err := os.MkdirTemp(tmpDir, "subtitles")
	if err != nil {
		return err
	}

	subtitlesFile := filepath.Join(subtitlesDir, path.Base(filePath))
	f, err := os.Create(subtitlesFile)
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, file); err != nil {
		return err
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
		return err
	}

	var trackListResponse mpv.ResponseTrackList
	if err := mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		log.Debug().Msg("Getting tracklist")

		if err := encoder.Encode(mpv.Request{[]interface{}{"get_property", "track-list"}}); err != nil {
			return err
		}

		return decoder.Decode(&trackListResponse)
	}); err != nil {
		return err
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
			return err
		}

		toast := adw.NewToast(L("This file does not contain subtitles."))

		subtitlesOverlay.AddToast(toast)

		return nil
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
		return err
	}

	return mpvClient.ExecuteMPVRequest(ipcFile, func(encoder *json.Encoder, decoder *json.Decoder) error {
		if err := encoder.Encode(mpv.Request{[]interface{}{"set_property", "sub-visibility", "yes"}}); err != nil {
			return err
		}

		var successResponse mpv.ResponseSuccess
		return decoder.Decode(&successResponse)
	})
}
