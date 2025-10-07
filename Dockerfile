ARG ARCH="amd64"
ARG OS="linux"
FROM golang:1.23-alpine3.20 AS builderimage
LABEL maintainer="Akamai SRE Observability Team <dl-etg-compute-sre-o11y@akamai.com>"
WORKDIR /go/src/s3_exporter

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build
COPY . .
RUN apk add --no-cache make git ca-certificates && \
    make build

###################################################################

FROM alpine:3.20

# Install ca-certificates and create non-root user
RUN apk --no-cache add ca-certificates tzdata wget && \
    apk --no-cache upgrade && \
    addgroup -g 1000 -S s3exporter && \
    adduser -u 1000 -S s3exporter -G s3exporter && \
    mkdir -p /app && \
    chown s3exporter:s3exporter /app

# Copy binary with correct ownership
COPY --from=builderimage --chown=1000:1000 /go/src/s3_exporter/s3_exporter /app/s3_exporter
WORKDIR /app

# Add healthcheck for production monitoring
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:9340/ || exit 1

EXPOSE      9340
USER        s3exporter:s3exporter
ENTRYPOINT  [ "./s3_exporter" ]
