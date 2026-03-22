# Stage 1: build
FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder

WORKDIR /workspace

# Cache dependencies before copying source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
      -ldflags="-s -w" \
      -a \
      -o gort \
      ./cmd/gort/...

# Stage 2: runtime — UBI9 minimal (no shell, no package manager, smallest attack surface)
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

WORKDIR /

# Copy only the compiled binary.
COPY --from=builder /workspace/gort /gort

# Run as non-root (UID 65532 = "nonroot" convention).
USER 65532:65532

EXPOSE 8080 8081

ENTRYPOINT ["/gort"]
