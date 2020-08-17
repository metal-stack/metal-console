package main

import (
	"github.com/metal-stack/metal-console/internal/console"
	"os"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/metal-lib/zapup"
)

var (
	version   = "devel"
	revision  string
	gitsha1   string
	builddate string
)

func main() {
	spec := &console.Specification{}
	err := envconfig.Process("METAL_CONSOLE", spec)
	logger := zapup.MustRootLogger().Sugar()
	if err != nil {
		logger.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	logger.Infow("metal-console", "version", getVersionString(),
		"port", spec.Port, "metal-api", spec.MetalAPIURL, "devmode", spec.DevMode())

	s, err := console.NewServer(logger, spec)
	if err != nil {
		logger.Errorw("failed to create metal-go driver", "error", err)
		os.Exit(1)
	}
	s.Run()
}

func getVersionString() string {
	var versionString = version
	if gitsha1 != "" {
		versionString += " (" + gitsha1 + ")"
	}
	if revision != "" {
		versionString += ", " + revision
	}
	if builddate != "" {
		versionString += ", " + builddate
	}
	return versionString
}
