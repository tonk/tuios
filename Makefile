BINDIR := $(CURDIR)
INSTALL_DIR := /usr/local/bin

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE) \
	-X main.builtBy=make

.PHONY: all build tuios tuios-web install clean

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
