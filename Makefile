.ONESHELL:
SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --all)
BUILDDATE := $(shell date -Iseconds)
VERSION := $(or ${VERSION},devel)

all: test console

.PHONY: test
test:
	go test -v ./...

.PHONY: console
console:
	go build \
		-trimpath \
		-tags netgo \
		-ldflags "-X 'github.com/metal-stack/v.Version=$(VERSION)' \
				  -X 'github.com/metal-stack/v.Revision=$(GITVERSION)' \
				  -X 'github.com/metal-stack/v.GitSHA1=$(SHA)' \
				  -X 'github.com/metal-stack/v.BuildDate=$(BUILDDATE)'" \
		-o bin/metal-console \
		main.go
	strip bin/metal-console
