#!/usr/bin/env bash
set -euo pipefail

REPO="dorky-robot/yelo"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

main() {
    local os arch target
    os="$(uname -s)"
    arch="$(uname -m)"

    case "$os" in
        Linux)  os="unknown-linux-gnu" ;;
        Darwin) os="apple-darwin" ;;
        *)      echo "Unsupported OS: $os" >&2; exit 1 ;;
    esac

    case "$arch" in
        x86_64|amd64)   arch="x86_64" ;;
        aarch64|arm64)  arch="aarch64" ;;
        *)              echo "Unsupported architecture: $arch" >&2; exit 1 ;;
    esac

    target="${arch}-${os}"

    local version
    version="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed 's/.*"v\(.*\)".*/\1/')"

    if [ -z "$version" ]; then
        echo "Error: could not determine latest version" >&2
        exit 1
    fi

    local url="https://github.com/${REPO}/releases/download/v${version}/yelo-${target}.tar.gz"
    local tmpdir
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    echo "Downloading yelo v${version} for ${target}..."
    curl -fsSL "$url" | tar -xz -C "$tmpdir"

    echo "Installing to ${INSTALL_DIR}/yelo..."
    install -d "$INSTALL_DIR"
    install -m 755 "$tmpdir/yelo" "$INSTALL_DIR/yelo"

    if [ "$(uname -s)" = "Darwin" ]; then
        codesign --sign - --force "$INSTALL_DIR/yelo" 2>/dev/null || true
    fi

    echo "yelo v${version} installed successfully"
    echo "Run 'yelo --help' to get started"
}

main "$@"
