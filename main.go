package main

import (
	"log/slog"
	"os"

	"github.com/metal-stack/metal-console/internal/console"
	metalgo "github.com/metal-stack/metal-go"

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

	// FIXME metal-view is enough
	client, err := metalgo.NewDriver(spec.MetalAPIURL, "", spec.HMACKey)
	if err != nil {
		log.Error("failed to create metal client", "error", err)
		panic(err)
	}

	log.Info("metal-console", "version", v.V.String(), "port", spec.Port, "metal-api", spec.MetalAPIURL, "devmode", spec.DevMode())
	if err := console.NewServer(log, spec, client).Run(); err != nil {
		log.Error("unable to start console server", "error", err)
		panic(err)
	}
}
