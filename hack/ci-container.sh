#!/usr/bin/env bash
# Run all CI checks inside ubuntu:latest, mirroring the GitHub Actions environment.
set -euo pipefail

apt-get update -q
apt-get install -y -q --no-install-recommends wget curl tar git make nodejs npm docker.io python3-pip
GO_VERSION=$(grep '^go ' go.mod | awk '{print $2}')
wget -q "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -O /tmp/go.tar.gz
tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH="${PATH}:/usr/local/go/bin:$(go env GOPATH)/bin"
pip install --break-system-packages yamllint
make ci-all
