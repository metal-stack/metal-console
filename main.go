package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	apiclient "github.com/metal-stack/api/go/client"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-console/internal/console"
	metalgo "github.com/metal-stack/metal-go"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/v"
)

func main() {
	spec := &console.Specification{}

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	log := slog.New(jsonHandler)

	err := envconfig.Process("METAL_CONSOLE", spec)
	if err != nil {
		log.Error("failed to read env config", "error", err)
		panic(err)
	}

	apiv1client, err := metalgo.NewDriver(spec.MetalAPIURL, "", spec.HMACKey)
	if err != nil {
		log.Error("failed to create metal-apiv1 client", "error", err)
		panic(err)
	}

	apiv2client, err := apiclient.New(&apiclient.DialConfig{
		BaseURL:   spec.MetalAPIServerURL,
		TokenFile: spec.TokenFile,
		// TokenFileRereadDuration: spec.TokenFileRereadDuration,
		Log: log,
	})
	if err != nil {
		log.Error("failed to create metal-apiserver v2 client", "error", err)
		panic(err)
	}

	// Ping apiserver every 5min
	apiv2client.Ping(context.Background(), &apiclient.PingConfig{
		ComponentType: apiv2.ComponentType_COMPONENT_TYPE_METAL_CONSOLE,
		StartedAt:     time.Now(),
		Version: apiv2.Version{
			Version:   v.Version,
			Revision:  v.Revision,
			GitSha1:   v.GitSHA1,
			BuildDate: v.BuildDate,
		},
	})

	log.Info("metal-console", "version", v.V.String(), "port", spec.Port, "metal-apiserver", spec.MetalAPIServerURL, "devmode", spec.DevMode())
	if err := console.NewServer(log, spec, apiv2client, apiv1client).Run(); err != nil {
		log.Error("unable to start console server", "error", err)
		panic(err)
	}
}
