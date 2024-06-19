FROM --platform=$BUILDPLATFORM golang:1.21-alpine

RUN apk add --update --no-cache npm

# install buf
RUN go install github.com/bufbuild/buf/cmd/buf@v1.33.0

# install protoc plugins

# GO
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.34.2
RUN go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.16.2
RUN go install github.com/envoyproxy/protoc-gen-validate@v1.0.4
RUN go install github.com/srikrsna/protoc-gen-gotag@v1.0.1

# WEB
# TODO: should we have versions on these?
RUN npm install typescript tsx
RUN npm install @bufbuild/buf \
    @bufbuild/protobuf \
    @bufbuild/protoc-gen-es \
    @connectrpc/connect \
    @connectrpc/protoc-gen-connect-es \
    @connectrpc/connect-query \
    @connectrpc/protoc-gen-connect-query
