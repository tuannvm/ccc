# Multi-stage Dockerfile for CCC (Claude Command Center)
# Standard build produces binary without voice features (CGO)

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build standard binary (no voice features, pure Go)
ARG VERSION=dev
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG TARGETARM

RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    GOARM=$TARGETARM \
    go build \
    -ldflags "-s -w" \
    -a -installsuffix cgo \
    -o ccc .

# Final stage - minimal runtime image
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/ccc /app/ccc

# Create non-root user
RUN addgroup -g 1000 ccc && \
    adduser -D -u 1000 -G ccc ccc && \
    chown -R ccc:ccc /app

USER ccc

ENTRYPOINT ["/app/ccc"]
CMD ["--help"]
