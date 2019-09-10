.PHONY: default
default: release;

COMMONDIR := $(or ${COMMONDIR},../common)

include $(COMMONDIR)/Makefile.inc

release:: gofmt test bmcproxy console;

bmcproxy:
	$(GO) build \
		-tags netgo \
		-o bin/bmc-proxy \
		./cmd/bmcproxy

console:
	$(GO) build \
		-tags netgo \
		-o bin/metal-console \
		./cmd/console
