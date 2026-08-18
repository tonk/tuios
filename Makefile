BINDIR := $(CURDIR)
INSTALL_DIR := /usr/local/bin
DIST := $(CURDIR)/dist
PACKAGING := $(CURDIR)/packaging

# ?= so CI can pin an exact released version (e.g. VERSION=0.8.0, no leading
# "v"): rpm's Version field rejects hyphens, which a dirty/commit-suffixed
# git-describe would introduce.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo 0.0.0-dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=v$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE) \
	-X main.builtBy=make

LINUX_ARCHES := amd64 arm64

.PHONY: all build tuios tuios-web install clean dist package checksums

all: build

build: tuios tuios-web

tuios:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINDIR)/tuios ./cmd/tuios

tuios-web:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINDIR)/tuios-web ./cmd/tuios-web

install: build
	sudo install -m 0755 $(BINDIR)/tuios $(INSTALL_DIR)/tuios
	sudo install -m 0755 $(BINDIR)/tuios-web $(INSTALL_DIR)/tuios-web

clean:
	rm -f $(BINDIR)/tuios $(BINDIR)/tuios-web
	rm -rf $(DIST)

# dist cross-compiles standalone Linux binaries for release: one raw binary
# per (package, arch), named so they can sit unpacked next to the .deb/.rpm
# assets on a release page.
dist:
	mkdir -p $(DIST)
	@for arch in $(LINUX_ARCHES); do \
		echo "==> tuios linux/$$arch"; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o $(DIST)/tuios_$(VERSION)_linux_$$arch ./cmd/tuios || exit 1; \
		echo "==> tuios-web linux/$$arch"; \
		GOOS=linux GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" \
			-o $(DIST)/tuios-web_$(VERSION)_linux_$$arch ./cmd/tuios-web || exit 1; \
	done

# package builds .deb and .rpm for each binary/arch out of the dist/ binaries,
# via nfpm (github.com/goreleaser/nfpm) - a pure-Go packager, so no dpkg-deb or
# rpmbuild needs to be installed on the machine that runs this.
package: dist
	@for arch in $(LINUX_ARCHES); do \
		for bin in tuios tuios-web; do \
			for fmt in deb rpm; do \
				echo "==> $$bin linux/$$arch ($$fmt)"; \
				VERSION=$(VERSION) ARCH=$$arch BIN_PATH=$(DIST)/$${bin}_$(VERSION)_linux_$$arch \
					nfpm package --config $(PACKAGING)/$$bin.yaml --packager $$fmt --target $(DIST)/ || exit 1; \
			done; \
		done; \
	done

checksums:
	cd $(DIST) && sha256sum * > checksums.txt
