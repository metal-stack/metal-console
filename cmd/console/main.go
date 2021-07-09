package main

import (
	"os"

	"github.com/metal-stack/metal-console/internal/console"
	"go.uber.org/zap"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/v"
)

func main() {
	spec := &console.Specification{}
	err := envconfig.Process("METAL_CONSOLE", spec)
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()
	if err != nil {
		sugar.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	sugar.Infow("metal-console", "version", v.V, "port", spec.Port, "metal-api", spec.MetalAPIURL, "devmode", spec.DevMode())

	s, err := console.NewServer(sugar, spec)
	if err != nil {
		sugar.Errorw("failed to create metal-go driver", "error", err)
		os.Exit(1)
	}
	s.Run()
}
