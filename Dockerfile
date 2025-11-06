# syntax=docker/dockerfile:1.6
FROM golang:1.22 AS builder
WORKDIR /app
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY . .
# If vendor/ exists, build offline using vendored modules; otherwise download modules.
RUN /bin/sh -c 'set -euo pipefail; if [ -d vendor ]; then echo "Using vendored modules"; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -o /out/cold-snap ./cmd/runner; else echo "Downloading modules via $${GOPROXY}"; go mod download; CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/cold-snap ./cmd/runner; fi'

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /out/cold-snap /usr/local/bin/cold-snap
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cold-snap"]
