package main

import (
	"os"

	"git.f-i-ts.de/cloud-native/maas/metal-console/cmd"
	log "github.com/inconshreveable/log15"
	"github.com/kelseyhightower/envconfig"
)

var (
	version   = "devel"
	revision  string
	gitsha1   string
	builddate string
)

func main() {
	var spec cmd.Specification
	err := envconfig.Process("metal-console", &spec)
	if err != nil {
		log.Error(err.Error())
		os.Exit(1)

	}
	log.Info("metal-console", "version", getVersionString(), "port", spec.Port, "metal-api", spec.MetalAPIUrl)

	console := &cmd.Console{
		Spec: &spec,
	}
	err = console.Run()
	if err != nil {
		log.Error("starting ssh server failed", "error", err)
	}
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
