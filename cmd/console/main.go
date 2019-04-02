package main

import (
	"git.f-i-ts.de/cloud-native/metal/metal-console/internal/server"
	"git.f-i-ts.de/cloud-native/metallib/zapup"
	"github.com/kelseyhightower/envconfig"
	"os"
)

var (
	version   = "devel"
	revision  string
	gitsha1   string
	builddate string
)

func main() {
	spec := &server.Specification{}
	err := envconfig.Process("METAL_CONSOLE", spec)
	if err != nil {
		zapup.MustRootLogger().Error(err.Error())
		os.Exit(1)
	}

	zapup.MustRootLogger().Sugar().Info("metal-console", "version", getVersionString(),
		"port", spec.Port, "metal-api", spec.MetalAPIAddress, "bmc reverse proxy address", spec.BMCReverseProxyAddress, "devmode", spec.DevMode())

	server.New(zapup.MustRootLogger(), spec).Run()
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
