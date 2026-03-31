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

echo "=== Downloading promtool ${PROM_VERSION} ==="
PROM_DIR="prometheus-${PROM_VERSION#v}.linux-amd64"
PROM_TGZ="${PROM_DIR}.tar.gz"
PROM_BASE="https://github.com/prometheus/prometheus/releases/download/${PROM_VERSION}"
curl -sL "${PROM_BASE}/${PROM_TGZ}" -o "/tmp/${PROM_TGZ}"
curl -sL "${PROM_BASE}/sha256sums.txt" -o "/tmp/prometheus-sha256sums.txt"

echo "=== Verifying promtool tarball checksum ==="
(cd /tmp && grep " ${PROM_TGZ}$" prometheus-sha256sums.txt | sha256sum -c -)

tar xzf "/tmp/${PROM_TGZ}" -C /tmp/ "${PROM_DIR}/promtool" --strip-components=1

echo "=== Installing yq ==="
YQ_VERSION="v4.44.1"
curl -sL "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/yq_linux_amd64" -o /tmp/yq
chmod +x /tmp/yq

echo "=== Extracting spec.groups from PrometheusRule ==="
EXTRACTED="/tmp/gort-rules-extracted.yaml"
/tmp/yq eval '{"groups": .spec.groups}' "${RULES_FILE}" > "${EXTRACTED}"

echo "=== Checking Prometheus rule syntax ==="
/tmp/promtool check rules "${EXTRACTED}"

echo "PASSED: All Prometheus alerting rules are valid"
