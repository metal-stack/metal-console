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
	sugar := l.Sugar()

	err = envconfig.Process("METAL_CONSOLE", spec)
	if err != nil {
		sugar.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	client, _, err := metalgo.NewDriver(spec.MetalAPIURL, spec.HMACKey, "")
	if err != nil {
		sugar.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	sugar.Infow("metal-console", "version", v.V, "port", spec.Port, "metal-api", spec.MetalAPIURL, "devmode", spec.DevMode())
	s, err := console.NewServer(sugar, spec, client)
	if err != nil {
		sugar.Errorw("failed to create metal-go driver", "error", err)
		os.Exit(1)
	}
	s.Run()
}
