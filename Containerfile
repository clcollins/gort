# Stage 1: build
FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder

ARG VERSION=dev

WORKDIR /workspace

# Cache dependencies before copying source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -buildvcs=false \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -a \
      -o gort \
      ./cmd/gort
RUN ./gort --help

# Stage 2: runtime — UBI9 minimal (no shell, no package manager, smallest attack surface)
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ARG BUILD_DATE=1970-01-01T00:00:00Z
ARG VCS_REF=unknown
ARG VERSION=dev

LABEL org.opencontainers.image.title="gort" \
      org.opencontainers.image.description="GORT - GitOps Reconciliation Tool" \
      org.opencontainers.image.url="https://github.com/clcollins/gort" \
      org.opencontainers.image.source="https://github.com/clcollins/gort" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.vendor="clcollins" \
      org.opencontainers.image.licenses="MIT" \
      io.k8s.display-name="gort" \
      io.k8s.description="GORT - GitOps Reconciliation Tool" \
      is.collins.cluster.image.revision="${VCS_REF}" \
      is.collins.cluster.image.version="${VERSION}" \
      is.collins.cluster.image.created="${BUILD_DATE}" \
      is.collins.cluster.build.commit.id="${VCS_REF}" \
      is.collins.cluster.build.date="${BUILD_DATE}"

WORKDIR /

# Copy only the compiled binary.
COPY --from=builder /workspace/gort /gort

# Run as non-root (UID 65532 = "nonroot" convention).
USER 65532:65532

EXPOSE 8080 8081

ENTRYPOINT ["/gort"]
