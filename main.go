package main

import (
	"git.f-i-ts.de/cloud-native/maas/metal-console/cmd"
	log "github.com/inconshreveable/log15"
)

var (
	version   = "devel"
	revision  string
	gitsha1   string
	builddate string
)

const (
	port = 2222
)

func main() {
	log.Info("metal-console", "version", getVersionString())

	err := cmd.Run(port)
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
