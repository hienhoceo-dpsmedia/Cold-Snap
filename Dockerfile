FROM golang:1.22 AS builder
WORKDIR /app
ARG GOPROXY=https://proxy.golang.org,direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}
COPY go.mod ./
# Copy go.sum if present to improve reproducibility
COPY go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/cold-snap ./cmd/runner

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=builder /out/cold-snap /usr/local/bin/cold-snap
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cold-snap"]
