# Multi-stage build for KubeVirt Shepherd
# Stage 1: Build
# Pin the builder to the host platform so multi-arch builds cross-compile
# instead of running the Go toolchain under QEMU emulation.
ARG GO_BUILDER_IMAGE=golang:1.25.12-bookworm
ARG ATLAS_IMAGE=arigaio/atlas:1.2.0

FROM --platform=$BUILDPLATFORM ${GO_BUILDER_IMAGE} AS builder

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

FROM ${ATLAS_IMAGE} AS atlas-cli

# Runtime asset layer shared by release images and downstream enterprise images.
FROM gcr.io/distroless/static-debian12:nonroot AS runtime-base

COPY --from=atlas-cli /atlas /usr/local/bin/atlas
COPY migrations/atlas /usr/local/share/shepherd/migrations/atlas

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["shepherd"]

# Stage 2: Runtime
FROM runtime-base AS runtime

COPY --from=builder /build/bin/shepherd /usr/local/bin/shepherd
COPY --from=builder /build/bin/seed /usr/local/bin/seed

# Development runtime image:
# baseline binaries are built on host and copied in directly to reuse host Go
# caches. Extended e2e fixtures are injected into the running container only
# when explicitly requested.
FROM runtime-base AS dev-runtime

COPY build/bin/shepherd /usr/local/bin/shepherd
COPY build/bin/seed /usr/local/bin/seed
