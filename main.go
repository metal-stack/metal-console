package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	apiclient "github.com/metal-stack/api/go/client"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
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
		BaseURL:                 spec.MetalAPIServerURL,
		TokenFile:               spec.TokenFile,
		TokenFileRereadDuration: spec.TokenFileRereadDuration,
	})
	if err != nil {
		log.Error("failed to create metal client", "error", err)
		panic(err)
	}

	// Ping apiserver every 5min
	client.Ping(context.Background(), &apiclient.PingConfig{
		ComponentType: apiv2.ComponentType_COMPONENT_TYPE_METAL_IMAGE_CACHE_SYNC,
		StartedAt:     time.Now(),
		Version: apiv2.Version{
			Version:   v.Version,
			Revision:  v.Revision,
			GitSha1:   v.GitSHA1,
			BuildDate: v.BuildDate,
		},
	})

	log.Info("metal-console", "version", v.V.String(), "port", spec.Port, "metal-apiserver", spec.MetalAPIServerURL, "devmode", spec.DevMode())
	if err := console.NewServer(log, spec, client).Run(); err != nil {
		log.Error("unable to start console server", "error", err)
		panic(err)
	}
}
