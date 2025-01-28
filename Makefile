GO ?= go
EXECUTABLE := go-jira
GOFILES := $(shell find . -type f -name "*.go")
TAGS ?=
LDFLAGS ?= -X 'main.Version=$(VERSION)' -X 'main.Commit=$(COMMIT)'

ifneq ($(shell uname), Darwin)
	EXTLDFLAGS = -extldflags "-static" $(null)
else
	EXTLDFLAGS =
endif

ifneq ($(DRONE_TAG),)
	VERSION ?= $(DRONE_TAG)
else
	VERSION ?= $(shell git describe --tags --always || git rev-parse --short HEAD)
endif
COMMIT ?= $(shell git rev-parse --short HEAD)

## build: build the binary
build: $(EXECUTABLE)

$(EXECUTABLE): $(GOFILES)
	$(GO) build -v -tags '$(TAGS)' -ldflags '$(EXTLDFLAGS)-s -w $(LDFLAGS)' -o bin/$@ ./cmd/$(EXECUTABLE)

## install: install the binary
install: $(GOFILES)
	$(GO) install -v -tags '$(TAGS)' -ldflags '$(EXTLDFLAGS)-s -w $(LDFLAGS)'

## test: run tests
test:
	@$(GO) test -v -cover -coverprofile coverage.txt ./... && echo "\n==>\033[32m Ok\033[m\n" || exit 1

## build_linux_amd64: build the binary for linux amd64
build_linux_amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -a -tags '$(TAGS)' -ldflags '$(EXTLDFLAGS)-s -w $(LDFLAGS)' -o release/linux/amd64/$(EXECUTABLE) ./cmd/$(EXECUTABLE)

## build_linux_arm64: build the binary for linux arm64
build_linux_arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -a -tags '$(TAGS)' -ldflags '$(EXTLDFLAGS)-s -w $(LDFLAGS)' -o release/linux/arm64/$(EXECUTABLE) ./cmd/$(EXECUTABLE)

## vendor: tidy and verify module dependencies
.PHONY: vendor
vendor:
	@echo 'Tidying and verifying module dependencies...'
	go mod tidy
	go mod verify

## bench: run benchmarks
.PHONY: bench
bench:
	@$(GO) test -bench=. -benchmem -run=^#  ./...

## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'