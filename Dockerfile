FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/cold-snap ./cmd/runner

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/cold-snap /usr/local/bin/cold-snap
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cold-snap"]
