package main

import (
	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/metal-console/internal/bmcproxy"
	"github.com/metal-stack/metal-lib/zapup"
	"os"
)

var (
	version   = "devel"
	revision  string
	gitsha1   string
	builddate string
)

func main() {
	spec := &bmcproxy.Specification{}
	err := envconfig.Process("BMC_PROXY", spec)
	logger := zapup.MustRootLogger().Sugar()
	if err != nil {
		logger.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	logger.Infow("bmc-proxy", "version", getVersionString(), "port", spec.Port)

	bmcproxy.New(logger, spec).Run()
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
