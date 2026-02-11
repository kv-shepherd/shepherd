# Multi-stage build for KubeVirt Shepherd
# Stage 1: Build
FROM golang:1.25-bookworm AS builder

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/bin/shepherd ./cmd/server/...
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /build/bin/seed ./cmd/seed/...

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /build/bin/shepherd /usr/local/bin/shepherd
COPY --from=builder /build/bin/seed /usr/local/bin/seed

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["shepherd"]
