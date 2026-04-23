package utils

import (
	"io"
	"os"
	"path"
	"path/filepath"
)

// SaveSubtitles writes the given subtitle file contents to a temporary file
// under tmpDir and returns its filesystem path. The caller is expected to
// hand the resulting path (wrapped as file://...) to the player.
func SaveSubtitles(filePath string, r io.Reader, tmpDir string) (string, error) {
	subtitlesDir, err := os.MkdirTemp(tmpDir, "subtitles")
	if err != nil {
		return "", err
	}

	subtitlesFile := filepath.Join(subtitlesDir, path.Base(filePath))
	f, err := os.Create(subtitlesFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}

	return subtitlesFile, nil
}
