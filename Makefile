COMMONDIR := $(or ${COMMONDIR},../common)

include $(COMMONDIR)/Makefile.inc

SWAGGERSPEC := metal-api.json
SWAGGERTARGET := metal-api

release:: generate-client gofmt test bmcproxy console;

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

.PHONY: clean-target
clean-target:
	rm -rf ${SWAGGERTARGET}
	mkdir ${SWAGGERTARGET}

.PHONY: generate-client
generate-client: clean-target swaggergenerate;
