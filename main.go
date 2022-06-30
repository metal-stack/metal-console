package main

import (
	"fmt"
	"os"

	"github.com/metal-stack/metal-console/internal/console"
	metalgo "github.com/metal-stack/metal-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/v"
)

func main() {
	spec := &console.Specification{}

	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "timestamp"
	cfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	l, err := cfg.Build()
	if err != nil {
		fmt.Printf("can't initialize zap logger: %s", err)
		os.Exit(1)
	}
	log := l.Sugar()

	err = envconfig.Process("METAL_CONSOLE", spec)
	if err != nil {
		log.Fatalw("failed to read env config", "error", err)
	}

	// FIXME metal-view is enough
	client, _, err := metalgo.NewDriver(spec.MetalAPIURL, "", spec.HMACKey)
	if err != nil {
		log.Fatalw("failed to create metal client", "error", err)
	}

	log.Infow("metal-console", "version", v.V, "port", spec.Port, "metal-api", spec.MetalAPIURL, "devmode", spec.DevMode())
	s, err := console.NewServer(log, spec, client)
	if err != nil {
		log.Fatalw("failed to create console server", "error", err)
	}
	s.Run()
}
