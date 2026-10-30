# git-inktrace build rules. Workspaces are disabled so the module builds
# exactly as declared in go.mod.

export GOWORK := off

BINARY  := git-inktrace
PKG     := github.com/ZhiWei-Ou/git-inktrace
BIN_DIR := bin

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION)
GOFLAGS_BUILD := -trimpath -buildvcs=true -ldflags "$(LDFLAGS)"

SHA256  := $(shell command -v sha256sum 2>/dev/null || echo shasum -a 256)

PLATFORMS := \
	darwin/amd64 darwin/arm64 \
	linux/amd64 linux/arm64 linux/arm \
	windows/amd64 windows/arm64

RELEASE_TARGETS := $(addprefix release-,$(subst /,-,$(PLATFORMS)))

.PHONY: all build install run test vet fmt tidy check release checksums clean help $(RELEASE_TARGETS)

all: check build

## build: build for the host platform into bin/
build:
	go build $(GOFLAGS_BUILD) -o $(BIN_DIR)/$(BINARY)$(shell go env GOEXE) .

## install: install into $(go env GOBIN), or $(go env GOPATH)/bin if unset
install:
	go install $(GOFLAGS_BUILD) .

## run: build and run in the current repository
run: build
	./$(BIN_DIR)/$(BINARY)$(shell go env GOEXE)

## test: run unit tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format sources
fmt:
	gofmt -s -w .

## tidy: tidy go.mod and go.sum
tidy:
	go mod tidy

## check: run vet and tests
check: vet test

## release: build release binaries for all platforms into bin/
release: $(RELEASE_TARGETS)
	@$(MAKE) --no-print-directory checksums

# release-<os>-<arch>: static release binary, e.g. make release-linux-arm64
# Recursive variables so $* expands per target, also under make -j.
REL_OS   = $(word 1,$(subst -, ,$*))
REL_ARCH = $(word 2,$(subst -, ,$*))
REL_EXT  = $(if $(filter windows,$(REL_OS)),.exe,)

$(RELEASE_TARGETS): release-%:
	CGO_ENABLED=0 GOOS=$(REL_OS) GOARCH=$(REL_ARCH) $(if $(filter arm,$(REL_ARCH)),GOARM=7,) \
		go build $(GOFLAGS_BUILD) -o $(BIN_DIR)/$(BINARY)-$(VERSION)-$(REL_OS)-$(REL_ARCH)$(REL_EXT) .

## checksums: write SHA256 sums of release binaries
checksums:
	cd $(BIN_DIR) && $(SHA256) $(BINARY)-$(VERSION)-* > $(BINARY)-$(VERSION)-SHA256SUMS

## clean: remove build output
clean:
	rm -rf $(BIN_DIR)

## help: list targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## //'
