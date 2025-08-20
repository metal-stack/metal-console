package main

import (
	"log/slog"
	"os"

	apiclient "github.com/metal-stack/api/go/client"
	"github.com/metal-stack/metal-console/internal/console"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/v"
)

func main() {
	spec := &console.Specification{}

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})
	log := slog.New(jsonHandler)

	err := envconfig.Process("METAL_CONSOLE", spec)
	if err != nil {
		log.Error("failed to read env config", "error", err)
		panic(err)
	}

	client, err := apiclient.New(&apiclient.DialConfig{
		BaseURL: spec.MetalAPIServerURL,
		Token:   spec.Token,
		// TODO enable token refresh
	})
	if err != nil {
		log.Error("failed to create metal client", "error", err)
		panic(err)
	}

	log.Info("metal-console", "version", v.V.String(), "port", spec.Port, "metal-apiserver", spec.MetalAPIServerURL, "devmode", spec.DevMode())
	if err := console.NewServer(log, spec, client).Run(); err != nil {
		log.Error("unable to start console server", "error", err)
		panic(err)
	}
}
