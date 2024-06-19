ARG GO_VERSION=1.21.3
ARG ALPINE_VERSION=3.18

# This layer builds the binary
FROM golang:${GO_VERSION}-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git upx

# Project setup
WORKDIR /app

# Build arguments
ARG DOMAIN
ARG SERVICE

# We want to populate the module cache based on the go.{mod,sum} files.
COPY ./services/${DOMAIN}/${SERVICE}/go.mod ./go.mod
COPY ./services/${DOMAIN}/${SERVICE}/go.sum ./go.sum

# Fetch go modules
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod verify

# Copy over code
COPY ./services/${DOMAIN}/${SERVICE} .

# Run tests
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go test -race -test.v ./...

# Build the binary
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    GOOS=linux go build -o main ./main.go

# Compress the binary
RUN upx main

# This layer runs the binary
FROM alpine:${ALPINE_VERSION} AS runner

# Install dependencies
RUN apk add -U --no-cache ca-certificates

# Copy the binary from builder
COPY --from=builder /app/main /etc/main

# Set context to run main
WORKDIR /etc

# Run
ENTRYPOINT ["/etc/main"]

# Runtime arguments
ARG HTTP_PORT=8080
ARG GRPC_PORT=8090

# Expose needed ports
EXPOSE ${HTTP_PORT}
EXPOSE ${GRPC_PORT}
