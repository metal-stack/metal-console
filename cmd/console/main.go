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
	if err != nil {
		zapup.MustRootLogger().Error(err.Error())
		os.Exit(1)
	}

	zapup.MustRootLogger().Sugar().Info("metal-console", "version", getVersionString(),
		"port", spec.Port, "metal-api", spec.MetalAPIURL, "devmode", spec.DevMode())

	s, err := console.NewServer(zapup.MustRootLogger(), spec)
	if err != nil {
		zapup.MustRootLogger().Error(err.Error())
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
