package main

import (
	"os"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/metal-console/internal/bmcproxy"
	"github.com/metal-stack/metal-lib/zapup"
	"github.com/metal-stack/v"
)

func main() {
	spec := &bmcproxy.Specification{}
	err := envconfig.Process("BMC_PROXY", spec)
	logger := zapup.MustRootLogger().Sugar()
	if err != nil {
		logger.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	logger.Infow("bmc-proxy", "version", v.V, "port", spec.Port)

	bmcproxy.New(logger, spec).Run()
}
