FROM golang:1.19

# Install buf
RUN go install github.com/bufbuild/buf/cmd/buf@v1.13.1

# Install protoc (needed for C# compilation)
RUN apt update && \
    apt install -y protobuf-compiler && \
    rm -rf /var/lib/apt/lists/*

# Install public dependencies
RUN go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.13.0
RUN go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.13.0
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
RUN go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0
RUN go install github.com/sorcererxw/protoc-gen-go-grpc-mock@v1.1.0
RUN go install github.com/srikrsna/protoc-gen-gotag@v0.6.2
RUN go install github.com/envoyproxy/protoc-gen-validate@v0.9.0

# Install private dependencies
ARG GITHUB_TOKEN
RUN git config --global url."https://$GITHUB_TOKEN:x-oauth-basic@github.com/fluidtruck/".insteadOf "https://github.com/fluidtruck/"
ENV GOPRIVATE=github.com/fluidtruck
RUN go install github.com/fluidtruck/protoc-gen-go-timestamppb-sql@v1.0.0

# clear git config
RUN : > ~/.gitconfig

# Set working directory
WORKDIR /work
