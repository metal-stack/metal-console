.ONESHELL:
SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --all)
BUILDDATE := $(shell date -Iseconds)
VERSION := $(or ${VERSION},devel)

BINARY := bin/metal-console

all: $(BINARY)

test:
	GO111MODULE=on \
	go test -v -race -cover $(shell go list ./...)

${BINARY}: clean test
	CGO_ENABLED=0 \
	GO111MODULE=on \
	go build \
		-tags netgo \
		-ldflags "-X 'main.version=$(VERSION)' \
				  -X 'main.revision=$(GITVERSION)' \
				  -X 'main.gitsha1=$(SHA)' \
				  -X 'main.builddate=$(BUILDDATE)'" \
	-o $@

clean:
	rm -f ${BINARY}
