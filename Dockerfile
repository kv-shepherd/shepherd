# Multi-stage build for KubeVirt Shepherd
# Stage 1: Build
# Pin the builder to the host platform so multi-arch builds cross-compile
# instead of running the Go toolchain under QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.25.9-bookworm AS builder

WORKDIR /build

ARG TARGETOS
ARG TARGETARCH

# Cache dependencies
COPY go.mod go.sum ./
RUN --mount=type=cache,id=shepherd-go-mod,target=/go/pkg/mod go mod download

# Build
COPY . .
RUN --mount=type=cache,id=shepherd-go-mod,target=/go/pkg/mod --mount=type=cache,id=shepherd-go-build,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=${TARGETOS:-$(go env GOOS)} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -ldflags="-s -w" -o /build/bin/shepherd ./cmd/server/...
RUN --mount=type=cache,id=shepherd-go-mod,target=/go/pkg/mod --mount=type=cache,id=shepherd-go-build,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=${TARGETOS:-$(go env GOOS)} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -ldflags="-s -w" -o /build/bin/seed ./cmd/seed/...

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot AS runtime

COPY --from=builder /build/bin/shepherd /usr/local/bin/shepherd
COPY --from=builder /build/bin/seed /usr/local/bin/seed

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["shepherd"]

# Development runtime image:
# baseline binaries are built on host and copied in directly to reuse host Go
# caches. Extended e2e fixtures are injected into the running container only
# when explicitly requested.
FROM gcr.io/distroless/static-debian12:nonroot AS dev-runtime

COPY build/bin/shepherd /usr/local/bin/shepherd
COPY build/bin/seed /usr/local/bin/seed

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["shepherd"]
