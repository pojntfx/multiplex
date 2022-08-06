package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/phayes/freeport"
	v1 "github.com/pojntfx/htorrent/pkg/api/http/v1"
	"github.com/pojntfx/htorrent/pkg/client"
	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	errEmptyMagnetLink         = errors.New("could not work with empty magnet link")
	errEmptyExpression         = errors.New("could not work with empty expression")
	errNoPathMatchesExpression = errors.New("could not find a path that matches the supplied expression")
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

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	verbose := flag.Int("verbose", 5, "Verbosity level (0 is disabled, default is info, 7 is trace)")
	storage := flag.String("storage", filepath.Join(home, ".local", "share", "htorrent", "var", "lib", "htorrent", "data"), "Path to store downloaded torrents in")
	magnet := flag.String("magnet", "", "Magnet link to get info for")
	mpv := flag.String("mpv", "mpv", "Command to launch mpv with")
	expression := flag.String("expression", "(.*).mkv$", "Regex to select the link to output by, i.e. (.*).mp4$ to only return the first .mp4 file; disables all other info")

	flag.Parse()

	if strings.TrimSpace(*magnet) == "" {
		panic(errEmptyMagnetLink)
	}

	if strings.TrimSpace(*expression) == "" {
		panic(errEmptyExpression)
	}

	switch *verbose {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	gateway := server.NewGateway(
		addr.String(),
		*storage,
		apiUsername,
		apiPassword,
		"",
		"",
		*verbose > 5,
		func(torrentMetrics v1.TorrentMetrics, fileMetrics v1.FileMetrics) {
			log.Info().
				Str("magnet", torrentMetrics.Magnet).
				Int("peers", torrentMetrics.Peers).
				Str("path", fileMetrics.Path).
				Int64("length", fileMetrics.Length).
				Int64("completed", fileMetrics.Completed).
				Msg("Streaming")
		},
		ctx,
	)

	if err := gateway.Open(); err != nil {
		panic(err)
	}

	s := make(chan os.Signal)
	signal.Notify(s, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-s

		log.Debug().Msg("Gracefully shutting down")

		go func() {
			<-s

			log.Debug().Msg("Forcing shutdown")

			cancel()

			os.Exit(1)
		}()

		if err := gateway.Close(); err != nil {
			panic(err)
		}

		cancel()
	}()

	go func() {
		log.Debug().
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

	log.Debug().Msg("Getting file list")

	info, err := manager.GetInfo(*magnet)
	if err != nil {
		panic(err)
	}

	filePreview := []string{}
	for _, f := range info.Files {
		filePreview = append(filePreview, f.Path)
	}

	log.Info().
		Strs("files", filePreview).
		Msg("Got file list")

	streamURL := ""

	exp := regexp.MustCompile(*expression)
	for _, f := range info.Files {
		if exp.Match([]byte(f.Path)) {
			u, err := getStreamURL("http://"+addr.String(), *magnet, f.Path)
			if err != nil {
				panic(err)
			}
			streamURL = u

			break
		}
	}

	if streamURL == "" {
		panic(errNoPathMatchesExpression)
	}

	usernameAndPassword := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v:%v", apiUsername, apiPassword)))

	shell := []string{"sh", "-c"}
	if runtime.GOOS == "windows" {
		shell = []string{"cmd", "/c"}
	}
	command := append(shell, fmt.Sprintf("%v '--http-header-fields=Authorization: Basic %v' '%v'", *mpv, usernameAndPassword, streamURL))

	output, err := exec.Command(
		command[0],
		command[1:]...,
	).CombinedOutput()
	if err != nil {
		log.Info().
			Str("output", string(output)).
			Msg("MPV command output")

		panic(err)
	}
}
