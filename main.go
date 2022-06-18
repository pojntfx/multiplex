package main

import (
	"context"
	"flag"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/pojntfx/htorrent/pkg/server"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// See https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go/22892986#22892986
func randSeq(n int) string {
	b := make([]rune, n)

	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	verbose := flag.Int("verbose", 5, "Verbosity level (0 is disabled, default is info, 7 is trace)")
	storage := flag.String("storage", filepath.Join(home, ".local", "share", "htorrent", "var", "lib", "htorrent", "data"), "Path to store downloaded torrents in")

	flag.Parse()

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
		func(peers int, total, completed int64, path string) {
			log.Debug().
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

	log.Info().
		Str("address", addr.String()).
		Msg("Listening")

	if err := gateway.Wait(); err != nil {
		panic(err)
	}
}
