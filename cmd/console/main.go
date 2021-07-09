package main

import (
	"os"

	"github.com/metal-stack/metal-console/internal/console"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/metal-lib/zapup"
	"github.com/metal-stack/v"
)

func main() {
	spec := &console.Specification{}
	err := envconfig.Process("METAL_CONSOLE", spec)
	logger := zapup.MustRootLogger().Sugar()
	if err != nil {
		logger.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	logger.Infow("metal-console", "version", v.V, "port", spec.Port, "metal-api", spec.MetalAPIURL, "devmode", spec.DevMode())

	s, err := console.NewServer(logger, spec)
	if err != nil {
		logger.Errorw("failed to create metal-go driver", "error", err)
		os.Exit(1)
	}
	s.Run()
}
