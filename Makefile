.ONESHELL:
SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --all)
BUILDDATE := $(shell date -Iseconds)
VERSION := $(or ${VERSION},devel)

.PHONY: default
default: release;

COMMONDIR := $(or ${COMMONDIR},../builder)

include $(COMMONDIR)/Makefile.inc

release:: gofmt test bmcproxy console;

bmcproxy:
	$(GO) build \
		-trimpath \
		-tags netgo \
		-ldflags "-X 'github.com/metal-stack/v.Version=$(VERSION)' \
				  -X 'github.com/metal-stack/v.Revision=$(GITVERSION)' \
				  -X 'github.com/metal-stack/v.GitSHA1=$(SHA)' \
				  -X 'github.com/metal-stack/v.BuildDate=$(BUILDDATE)'" \
		-o bin/bmc-proxy \
		./cmd/bmcproxy
	strip bin/bmc-proxy

console:
	$(GO) build \
		-trimpath \
		-tags netgo \
		-ldflags "-X 'github.com/metal-stack/v.Version=$(VERSION)' \
				  -X 'github.com/metal-stack/v.Revision=$(GITVERSION)' \
				  -X 'github.com/metal-stack/v.GitSHA1=$(SHA)' \
				  -X 'github.com/metal-stack/v.BuildDate=$(BUILDDATE)'" \
		-o bin/metal-console \
		./cmd/console
	strip bin/metal-console
