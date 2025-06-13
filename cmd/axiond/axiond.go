package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"peertech.de/axion/pkg/api"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	api := api.New(
		api.WithListenAddr("0.0.0.0:8080"),
	)
	if err := api.Initialize(); err != nil {
		log.Error().Err(err).Msg("Failed to initialize api")
		return
	}

	log.Info().Msg("Serving api...")
	go func() {
		if err := api.Serve(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			log.Error().Err(err).Msg("Failed to serve api")
			cancel()
		}
	}()

	<-ctx.Done()
	log.Info().Msg("Stopping api...")
	if err := api.Stop(); err != nil {
		log.Error().Err(err).Msg("Failed to stop api")
	}

	log.Info().Msg("Done")
}
