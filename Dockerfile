FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk --no-cache add git ca-certificates && update-ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Always run go mod tidy to ensure dependencies are correct
RUN go mod tidy
RUN mkdir -p /out && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/cold-snap ./cmd/runner

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata && update-ca-certificates
WORKDIR /app
COPY --from=builder /out/cold-snap /usr/local/bin/cold-snap
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cold-snap"]
