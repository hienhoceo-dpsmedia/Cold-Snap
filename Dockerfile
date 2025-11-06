FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk --no-cache add git ca-certificates && update-ca-certificates
COPY go.mod ./
RUN go mod download
COPY . .
# If vendor is present (prepared by CI), skip go mod tidy/network access
RUN if [ -d vendor ]; then echo "vendor present; skipping go mod tidy"; else go mod tidy; fi
RUN mkdir -p /out && \
    if [ -d vendor ]; then \
      echo "Building with vendor/"; \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/cold-snap ./cmd/runner; \
    else \
      echo "Building with modules"; \
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/cold-snap ./cmd/runner; \
    fi

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata && update-ca-certificates
WORKDIR /app
COPY --from=builder /out/cold-snap /usr/local/bin/cold-snap
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cold-snap"]
