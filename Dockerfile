# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /src

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Cache dependencies
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source code
COPY . .

# Build single static binary
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X 'github.com/liteploy/liteploy/internal/system.Version=${VERSION}' -X 'github.com/liteploy/liteploy/internal/system.CommitSHA=${COMMIT}' -X 'github.com/liteploy/liteploy/internal/system.BuildDate=${BUILD_DATE}'" \
    -o /bin/liteploy \
    ./cmd/liteploy

# Minimal runtime stage
FROM alpine:3.20

# Install runtime prerequisites: git for cloning repositories, ca-certificates for TLS
RUN apk add --no-cache git ca-certificates tzdata

# Create data directory
RUN mkdir -p /var/lib/liteploy && chmod 750 /var/lib/liteploy

COPY --from=builder /bin/liteploy /usr/local/bin/liteploy

ENV LITEPLOY_HTTP_ADDR=":8080" \
    LITEPLOY_DATA_DIR="/var/lib/liteploy" \
    LITEPLOY_LOG_LEVEL="info" \
    LITEPLOY_LOG_JSON="true"

EXPOSE 8080

VOLUME ["/var/lib/liteploy"]

ENTRYPOINT ["/usr/local/bin/liteploy"]
