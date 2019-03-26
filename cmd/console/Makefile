.ONESHELL:
SHA := $(shell git rev-parse --short=8 HEAD)
GITVERSION := $(shell git describe --long --all)
BUILDDATE := $(shell date -Iseconds)
VERSION := $(or ${VERSION},devel)
BIN_DIR := ../../bin
BINARY := ${BIN_DIR}/metal-console

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

fmt:
	go fmt ./...

swagger:
	rm -rf metal-api; \
	mkdir -p ${BIN_DIR} metal-api; \
	if [ ! -f ${BIN_DIR}/swagger ]; then \
		curl -fLSs https://github.com/go-swagger/go-swagger/releases/download/v0.17.0/swagger_linux_amd64 -o ${BIN_DIR}/swagger; \
		chmod +x ${BIN_DIR}/swagger; \
	fi; \
	GO111MODULE=off \
	${BIN_DIR}/swagger generate client --target=metal-api -f metal-api.json --skip-validation
