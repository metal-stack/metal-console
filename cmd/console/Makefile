BINARY := metal-console
COMMONDIR := $(or ${COMMONDIR},../../../common)

in-docker: clean-client generate-client test all;

include $(COMMONDIR)/Makefile.inc

release:: generate-client test all ;

.PHONY: clean-local-dirs
clean-local-dirs:
	rm -rf metal-api
	mkdir metal-api

.PHONY: clean-client
clean-client: clean-local-dirs
	cp ../../../metal-api/spec/metal-api.json metal-api.json

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: generate-client
generate-client: clean-local-dirs
	GO111MODULE=off swagger generate client --target=metal-api -f metal-api.json --skip-validation
