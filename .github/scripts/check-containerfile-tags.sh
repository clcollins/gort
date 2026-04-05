#!/usr/bin/env bash
# Checks Containerfile FROM lines for :latest tags and missing registry prefixes.
# Currently warn-only — exits 0 even on findings. Flip ENFORCE=1 once base images are pinned.
set -euo pipefail

CONTAINERFILE="${1:-Containerfile}"
ENFORCE="${ENFORCE:-1}"

if [ ! -f "${CONTAINERFILE}" ]; then
  echo "No Containerfile found at ${CONTAINERFILE} — skipping."
  exit 0
fi

KNOWN_REGISTRIES=(
  "docker.io"
  "ghcr.io"
  "gcr.io"
  "registry.k8s.io"
  "quay.io"
  "mcr.microsoft.com"
  "public.ecr.aws"
  "lscr.io"
  "registry.access.redhat.com"
  "registry.redhat.io"
  "registry.fedoraproject.org"
)

WARNINGS=0

while IFS= read -r line; do
  # Extract image reference (second word, strip " AS <alias>")
  image=$(echo "${line}" | awk '{print $2}')

  # Check for :latest tag
  if echo "${image}" | grep -qE ':latest$'; then
    echo "WARNING: Containerfile uses :latest tag: ${image}"
    WARNINGS=$((WARNINGS + 1))
  fi

  # Check for missing registry prefix
  has_registry=0
  for registry in "${KNOWN_REGISTRIES[@]}"; do
    if [[ "${image}" == "${registry}/"* ]]; then
      has_registry=1
      break
    fi
  done
  if [ "${has_registry}" -eq 0 ]; then
    echo "WARNING: Image missing known registry prefix: ${image}"
    WARNINGS=$((WARNINGS + 1))
  fi
done < <(grep -i '^FROM ' "${CONTAINERFILE}")

if [ "${WARNINGS}" -gt 0 ]; then
  echo "${WARNINGS} warning(s) found in ${CONTAINERFILE}."
  if [ "${ENFORCE}" -eq 1 ]; then
    echo "ENFORCE=1: treating warnings as errors."
    exit 1
  fi
  echo "ENFORCE=0: warnings only, not failing."
else
  echo "PASSED: All Containerfile base images use pinned tags and known registries."
fi
