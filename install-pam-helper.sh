#!/usr/bin/env bash

# tuios-pam-helper Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/tonk/tuios/main/install-pam-helper.sh | bash
#
# tuios-pam-helper is the privileged half of tuios-web's optional
# --pam-auth mode: it runs as root, authenticates trainees against PAM, and
# spawns their shells as their own Unix accounts. This script installs the
# binary and a starter /etc/pam.d/tuios-web, and explains what you still
# need to do (run it as root, pass --pam-auth to tuios-web) - it does not
# start anything or set up a service for you.
#
# Unlike install.sh/install-web.sh, this only ever targets Linux/amd64: the
# binary is a cgo build against your distro's PAM headers, and only amd64 is
# currently published (see PAM_HELPER_ARCH in the root Makefile).

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# tonk/tuios, not the upstream Gaurav-Gosain/tuios this was originally
# forked from: pam-helper (and its release asset, and this starter PAM
# service file) only exist on this fork, never upstream.
REPO="tonk/tuios"
BINARY_NAME="tuios-pam-helper"
PAM_SERVICE_PATH="/etc/pam.d/tuios-web"
PAM_SERVICE_REPO_PATH="pam-helper/pam.d/tuios-web"

print_info()    { echo -e "${BLUE}i${NC} $1"; }
print_success() { echo -e "${GREEN}+${NC} $1"; }
print_error()   { echo -e "${RED}x${NC} $1"; }
print_warning() { echo -e "${YELLOW}!${NC} $1"; }

get_latest_version() {
    if command -v curl &> /dev/null; then
        VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    elif command -v wget &> /dev/null; then
        VERSION=$(wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    else
        print_error "Neither curl nor wget found. Please install one of them."
        exit 1
    fi
    if [ -z "$VERSION" ]; then
        print_error "Failed to get latest version from GitHub"
        exit 1
    fi
    echo "$VERSION"
}

download_file() {
    URL=$1
    OUTPUT=$2
    if command -v curl &> /dev/null; then
        curl -fsSL "$URL" -o "$OUTPUT"
    elif command -v wget &> /dev/null; then
        wget -qO "$OUTPUT" "$URL"
    else
        print_error "Neither curl nor wget found"
        exit 1
    fi
}

main() {
    print_info "Installing tuios-pam-helper..."
    print_warning "This is a privileged tool: it runs as root and authenticates other users."
    echo ""

    OS="$(uname -s)"
    if [ "$OS" != "Linux" ]; then
        print_error "tuios-pam-helper only runs on Linux (it needs your distro's PAM stack). Detected: $OS"
        exit 1
    fi

    ARCH="$(uname -m)"
    if [ "$ARCH" != "x86_64" ] && [ "$ARCH" != "amd64" ]; then
        print_error "Only amd64 is currently published for tuios-pam-helper. Detected: $ARCH"
        print_info "Build it yourself instead: see pam-helper/README.md in the repo"
        print_info "(needs libpam0g-dev / pam-devel, then \`make pam-helper\`)."
        exit 1
    fi

    print_info "Fetching latest release..."
    VERSION=$(get_latest_version)
    print_success "Latest version: $VERSION"
    VERSION_NO_V="${VERSION#v}"

    # Matches what the release pipeline actually produces (see Makefile's
    # dist-pam-helper target): a raw binary, not an archive.
    ASSET_NAME="${BINARY_NAME}_${VERSION_NO_V}_linux_amd64"
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"

    TMP_DIR=$(mktemp -d)
    trap 'rm -rf -- "$TMP_DIR"' EXIT

    print_info "Downloading ${ASSET_NAME}..."
    if ! download_file "$DOWNLOAD_URL" "$TMP_DIR/$BINARY_NAME"; then
        print_error "Failed to download release"
        print_info "URL: $DOWNLOAD_URL"
        exit 1
    fi
    chmod +x "$TMP_DIR/$BINARY_NAME"
    print_success "Downloaded successfully"

    INSTALL_DIR="/usr/local/bin"
    print_info "Installing to $INSTALL_DIR (needs sudo)..."
    sudo install -m 0755 "$TMP_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME"
    print_success "Installed $BINARY_NAME to $INSTALL_DIR"

    if [ -f "$PAM_SERVICE_PATH" ]; then
        print_info "$PAM_SERVICE_PATH already exists, leaving it alone"
    else
        print_info "Fetching the starter PAM service file..."
        PAM_SERVICE_URL="https://raw.githubusercontent.com/${REPO}/${VERSION}/${PAM_SERVICE_REPO_PATH}"
        if download_file "$PAM_SERVICE_URL" "$TMP_DIR/tuios-web.pam"; then
            sudo install -m 0644 "$TMP_DIR/tuios-web.pam" "$PAM_SERVICE_PATH"
            print_success "Installed $PAM_SERVICE_PATH"
        else
            print_warning "Could not fetch the PAM service file automatically"
            print_info "Copy it yourself from ${PAM_SERVICE_REPO_PATH} in the repo to $PAM_SERVICE_PATH"
        fi
    fi

    echo ""
    print_success "Installation complete!"
    print_info "Run 'tuios-pam-helper --version' to verify"
    echo ""
    print_info "This does nothing until you run it as root: sudo tuios-pam-helper"
    print_info "Then start tuios-web with --pam-auth to actually use it."
}

main
