#!/usr/bin/env bash
# Validates Prometheus alert rule syntax from the PrometheusRule manifest.
# Extracts spec.groups from the K8s resource and checks with promtool.
set -euo pipefail

PROM_VERSION="v3.2.1"
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
  PROM_DIR="prometheus-${PROM_VERSION#v}.linux-amd64"
  PROM_TGZ="${PROM_DIR}.tar.gz"
  PROM_BASE="https://github.com/prometheus/prometheus/releases/download/${PROM_VERSION}"
  curl -sL "${PROM_BASE}/${PROM_TGZ}" -o "/tmp/${PROM_TGZ}"
  curl -sL "${PROM_BASE}/sha256sums.txt" -o "/tmp/prometheus-sha256sums.txt"

  echo "=== Verifying promtool tarball checksum ==="
  (cd /tmp && grep " ${PROM_TGZ}$" prometheus-sha256sums.txt | sha256sum -c -)

  tar xzf "/tmp/${PROM_TGZ}" -C /tmp/ "${PROM_DIR}/promtool" --strip-components=1
  PROMTOOL="/tmp/promtool"
fi

echo "=== Installing yq ==="
YQ_VERSION="v4.44.1"
YQ_SHA256="6dc2d0cd4e0caca5aeffd0d784a48263591080e4a0895abe69f3a76eb50d1ba3"
curl -sL "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64" -o /tmp/yq

echo "=== Verifying yq checksum ==="
echo "${YQ_SHA256}  /tmp/yq" | sha256sum -c -
chmod +x /tmp/yq

echo "=== Extracting spec.groups from PrometheusRule ==="
EXTRACTED="/tmp/gort-rules-extracted.yaml"
/tmp/yq eval '{"groups": .spec.groups}' "${RULES_FILE}" > "${EXTRACTED}"

echo "=== Checking Prometheus rule syntax ==="
${PROMTOOL} check rules "${EXTRACTED}"

echo "PASSED: All Prometheus alerting rules are valid"
