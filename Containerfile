# CI build image for tuios's Forgejo Actions release workflow
# (.forgejo/workflows/release.yml). Cross-compiles tuios/tuios-web for Linux
# and packages them into .deb/.rpm via nfpm - a pure-Go packager, so no
# dpkg-deb or rpmbuild is needed here. Also builds tuios-pam-helper (see
# experimental/pam-trainee-auth), a cgo binary needing libpam0g-dev - a
# dependency of that build alone, not of tuios/tuios-web.
#
# This is a build-time tool image only, not something tuios itself runs in;
# see Dockerfile for the runtime image.
FROM golang:1.26-bookworm

RUN apt-get update \
	&& apt-get install -y --no-install-recommends make libpam0g-dev \
	&& rm -rf /var/lib/apt/lists/*

# Pin this once you've picked a version, for reproducible builds.
RUN go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

WORKDIR /workspace
