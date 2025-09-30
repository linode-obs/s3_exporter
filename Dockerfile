ARG ARCH="amd64"
ARG OS="linux"
FROM golang:1.23-alpine3.19 AS build
LABEL maintainer="Linode Observability Team"

WORKDIR /app
COPY . .

RUN apk add --no-cache make git && \
    echo "s3:*:100:s3" > group && \
    echo "s3:*:100:100::/:/s3_exporter" > passwd && \
    make build

###################################################################

FROM alpine:3.19

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

COPY --from=build /app/group /app/passwd /etc/
COPY --from=build /app/s3_exporter /usr/local/bin/s3_exporter

USER s3:s3
EXPOSE 9340/tcp
ENTRYPOINT ["/usr/local/bin/s3_exporter"]
