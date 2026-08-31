BINDIR := $(CURDIR)
INSTALL_DIR := /usr/local/bin
DIST := $(CURDIR)/dist
PACKAGING := $(CURDIR)/packaging
PAM_HELPER_DIR := $(CURDIR)/experimental/pam-trainee-auth

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

# tuios-pam-helper is Linux/amd64 only, on purpose: it's a cgo binary
# (github.com/msteinert/pam/v2, needs libpam0g-dev), and cross-compiling cgo
# to arm64 would need a cross toolchain and arm64 PAM headers neither this
# Makefile nor CI sets up. amd64 covers the CI runner and the common case;
# revisit if arm64 is ever actually needed.
PAM_HELPER_ARCH := amd64

.PHONY: all build tuios tuios-web install clean dist package checksums \
	pam-helper install-pam-helper dist-pam-helper package-pam-helper check-pam-headers

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
	rm -f $(BINDIR)/tuios $(BINDIR)/tuios-web $(BINDIR)/tuios-pam-helper
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

# --- tuios-pam-helper ---
#
# Deliberately NOT part of build/install/dist/package above: it lives in its
# own Go module (see PAM_HELPER_DIR) precisely so the ordinary build never
# needs libpam0g-dev, and it's a root-run, security-sensitive tool nobody
# should get bundled into a routine `make install` without asking for it by
# name. CI calls these targets explicitly, alongside (not instead of) the
# ordinary ones - see .forgejo/workflows/release.yml and
# .github/workflows/release.yml.

check-pam-headers:
	@test -f /usr/include/security/pam_appl.h || { \
		echo "error: PAM headers not found (needed to build tuios-pam-helper)." >&2; \
		echo "  Debian/Ubuntu: sudo apt-get install libpam0g-dev" >&2; \
		echo "  Fedora/RHEL:   sudo dnf install pam-devel" >&2; \
		exit 1; \
	}

pam-helper: check-pam-headers
	cd $(PAM_HELPER_DIR) && go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINDIR)/tuios-pam-helper ./helper

install-pam-helper: pam-helper
	sudo install -m 0755 $(BINDIR)/tuios-pam-helper $(INSTALL_DIR)/tuios-pam-helper
	sudo install -m 0644 $(PAM_HELPER_DIR)/pam.d/tuios-web /etc/pam.d/tuios-web

dist-pam-helper: check-pam-headers
	mkdir -p $(DIST)
	@echo "==> tuios-pam-helper linux/$(PAM_HELPER_ARCH)"
	cd $(PAM_HELPER_DIR) && CGO_ENABLED=1 GOOS=linux GOARCH=$(PAM_HELPER_ARCH) go build -trimpath -ldflags "$(LDFLAGS)" \
		-o $(DIST)/tuios-pam-helper_$(VERSION)_linux_$(PAM_HELPER_ARCH) ./helper

package-pam-helper: dist-pam-helper
	@for fmt in deb rpm; do \
		echo "==> tuios-pam-helper linux/$(PAM_HELPER_ARCH) ($$fmt)"; \
		VERSION=$(VERSION) ARCH=$(PAM_HELPER_ARCH) \
			BIN_PATH=$(DIST)/tuios-pam-helper_$(VERSION)_linux_$(PAM_HELPER_ARCH) \
			PAM_D_SRC=$(PAM_HELPER_DIR)/pam.d/tuios-web \
			nfpm package --config $(PACKAGING)/tuios-pam-helper.yaml --packager $$fmt --target $(DIST)/ || exit 1; \
	done
