#!/usr/bin/env bash
# Validates Prometheus alert rule syntax from the PrometheusRule manifest.
# Extracts spec.groups from the K8s resource and checks with promtool.
set -euo pipefail

PROM_VERSION="v3.3.1"
RULES_FILE="config/alerting/alerts.yaml"

if [ ! -f "${RULES_FILE}" ]; then
  echo "No rules file found at ${RULES_FILE} — skipping."
  exit 0
fi

# Use preinstalled promtool if available (e.g., inside CI container).
if command -v promtool >/dev/null 2>&1; then
  PROMTOOL="promtool"
  echo "=== Using preinstalled promtool ($(promtool --version 2>&1 | head -1)) ==="
else
  echo "=== Downloading promtool ${PROM_VERSION} ==="
  ARCH=$(case $(uname -m) in x86_64) echo amd64;; aarch64) echo arm64;; *) echo "$(uname -m)";; esac)
  PROM_DIR="prometheus-${PROM_VERSION#v}.linux-${ARCH}"
  PROM_TGZ="${PROM_DIR}.tar.gz"
  PROM_BASE="https://github.com/prometheus/prometheus/releases/download/${PROM_VERSION}"
  curl -sL "${PROM_BASE}/${PROM_TGZ}" -o "/tmp/${PROM_TGZ}"
  curl -sL "${PROM_BASE}/sha256sums.txt" -o "/tmp/prometheus-sha256sums.txt"

  echo "=== Verifying promtool tarball checksum ==="
  (cd /tmp && grep -F " ${PROM_TGZ}" prometheus-sha256sums.txt | sha256sum -c -)

  tar xzf "/tmp/${PROM_TGZ}" -C /tmp/ "${PROM_DIR}/promtool" --strip-components=1
  PROMTOOL="/tmp/promtool"
fi

echo "=== Installing yq ==="
YQ_VERSION="v4.44.1"
YQ_ARCH=$(case $(uname -m) in x86_64) echo amd64;; aarch64) echo arm64;; *) echo "$(uname -m)";; esac)
# SHA256 checksums for yq binaries (without .tar.gz suffix).
case "${YQ_ARCH}" in
  amd64) YQ_SHA256="6dc2d0cd4e0caca5aeffd0d784a48263591080e4a0895abe69f3a76eb50d1ba3" ;;
  arm64) YQ_SHA256="8c12fcc10e14774ca6624cc282f092a526568b036fe1192258c3aecbad56d063" ;;
  *) echo "ERROR: unsupported architecture for yq: ${YQ_ARCH}"; exit 1 ;;
esac
curl -sL "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_${YQ_ARCH}" -o /tmp/yq

echo "=== Verifying yq checksum ==="
echo "${YQ_SHA256}  /tmp/yq" | sha256sum -c -
chmod +x /tmp/yq

echo "=== Extracting spec.groups from PrometheusRule ==="
EXTRACTED="/tmp/gort-rules-extracted.yaml"
/tmp/yq eval '{"groups": .spec.groups}' "${RULES_FILE}" > "${EXTRACTED}"

echo "=== Checking Prometheus rule syntax ==="
${PROMTOOL} check rules "${EXTRACTED}"

echo "PASSED: All Prometheus alerting rules are valid"
