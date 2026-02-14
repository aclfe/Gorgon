#!/bin/bash

set -e

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
else
    echo "Unsupported architecture: $ARCH"
    exit 1
fi

if [ "$OS" = "linux" ]; then
    OS="linux"
elif [ "$OS" = "darwin" ]; then
    OS="darwin"
else
    echo "Unsupported OS: $OS (only Linux and macOS supported)"
    exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
    echo "curl not found. Attempting to install on Linux..."
    if [ "$OS" != "linux" ]; then
        echo "Please install curl manually on $OS."
        exit 1
    fi
    if command -v apt >/dev/null 2>&1; then
        sudo apt update && sudo apt install -y curl
    elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y curl
    elif command -v yum >/dev/null 2>&1; then
        sudo yum install -y curl
    else
        echo "Unable to install curl automatically. Please install curl and try again."
        exit 1
    fi
    if ! command -v curl >/dev/null 2>&1; then
        echo "Failed to install curl."
        exit 1
    fi
    echo "curl installed successfully."
fi

echo "Fetching latest stable Go version..."
GO_JSON=$(curl -s https://go.dev/dl/?mode=json)
GO_VERSION=$(echo "$GO_JSON" | grep -m1 -oP '(?<="version": "go)\d+\.\d+(\.\d+)?')
if [ -z "$GO_VERSION" ]; then
    echo "Failed to fetch latest Go version."
    exit 1
fi
echo "Latest Go version: $GO_VERSION"

echo "Fetching latest golangci-lint version..."
LINT_VERSION=$(curl -s https://api.github.com/repos/golangci/golangci-lint/releases/latest | grep '"tag_name":' | cut -d '"' -f4)
if [ -z "$LINT_VERSION" ]; then
    echo "Failed to fetch latest golangci-lint version."
    exit 1
fi
echo "Latest golangci-lint version: $LINT_VERSION"

FILE="go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
URL="https://go.dev/dl/${FILE}"
echo "Downloading Go ${GO_VERSION} for ${OS}-${ARCH}..."
curl -L "$URL" -o "/tmp/${FILE}"

echo "Installing Go to /usr/local/go..."
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "/tmp/${FILE}"
rm "/tmp/${FILE}"

echo "Go upgraded. Add '/usr/local/go/bin' to your PATH if not already (e.g., in ~/.bashrc or ~/.zshrc)."
echo "You may need to restart your terminal or run 'source ~/.bashrc'."

echo "Upgrading golangci-lint to ${LINT_VERSION}..."
export PATH="/usr/local/go/bin:$PATH"
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(/usr/local/go/bin/go env GOPATH)/bin" "${LINT_VERSION}"

echo "Upgrade complete. Verify with:"
echo "go version  # Should show go${GO_VERSION}"
echo "golangci-lint --version  # Should show ${LINT_VERSION}"