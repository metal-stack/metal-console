package main

import (
	"os"

	"github.com/kelseyhightower/envconfig"
	"github.com/metal-stack/metal-console/internal/bmcproxy"
	"github.com/metal-stack/v"
	"go.uber.org/zap"
)

func main() {
	spec := &bmcproxy.Specification{}
	err := envconfig.Process("BMC_PROXY", spec)
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()
	if err != nil {
		sugar.Errorw("failed to read env config", "error", err)
		os.Exit(1)
	}

	sugar.Infow("bmc-proxy", "version", v.V, "port", spec.Port)

	bmcproxy.New(sugar, spec).Run()
}
