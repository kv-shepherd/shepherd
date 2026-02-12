# Multi-stage build for KubeVirt Shepherd
# Stage 1: Build
FROM golang:1.25.7-bookworm AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,id=shepherd-go-mod,target=/go/pkg/mod go mod download

# Build
COPY . .
RUN --mount=type=cache,id=shepherd-go-mod,target=/go/pkg/mod --mount=type=cache,id=shepherd-go-build,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/bin/shepherd ./cmd/server/...
RUN --mount=type=cache,id=shepherd-go-mod,target=/go/pkg/mod --mount=type=cache,id=shepherd-go-build,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/bin/seed ./cmd/seed/...

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/bin/shepherd /usr/local/bin/shepherd
COPY --from=builder /build/bin/seed /usr/local/bin/seed

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["shepherd"]

# Development runtime image:
# binaries are built on host and copied in directly to reuse host Go caches.
FROM gcr.io/distroless/static-debian12:nonroot AS dev-runtime

COPY build/bin/shepherd /usr/local/bin/shepherd
COPY build/bin/seed /usr/local/bin/seed

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["shepherd"]
