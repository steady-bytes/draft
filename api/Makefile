.PHONY: build

install:
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.13.0
	go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@v2.13.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0
	go install github.com/sorcererxw/protoc-gen-go-grpc-mock@v1.1.0
	go install github.com/srikrsna/protoc-gen-gotag@v0.6.2
	go install github.com/envoyproxy/protoc-gen-validate@v0.9.0
	go install github.com/sorcererxw/protoc-gen-go-grpc-mock@latest
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

## go install github.com/fluidtruck/protoc-gen-go-timestamppb-sql@v1.0.0

clean:
	rm -rf gen/go
	rm -rf gen/go-connect
	rm -rf gen/web

go:
	rm -rf gen/go
	buf mod update
	buf generate --template buf.gen.go.yaml
	buf generate --template buf.gen.gotag.yaml
	$(MAKE) init-go-mod
	$(MAKE) init-go-connect-mod


# todo -> run this before compiling the web code
# this adds all the generators to node_modules
# PATH=$PATH:$(pwd)/node_modules/.bin
web:
	rm -rf gen/web
	buf mod update
	buf generate --template buf.gen.web.yaml

rust:
	rm -rf gen/rust/registry
	mkdir gen/rust/registry
	cargo build

go-diff:
	rm -f gen/go/go.mod
	rm -f gen/go/go.sum
	buf mod update
	./diff-buf-gen.sh
	$(MAKE) init-go-mod

init-go-mod:
	cd gen/go; go mod init github.com/steady-bytes/draft/api/gen/go; go mod tidy

# init-go-connect-mod:
# 	cd gen/go-connect; go mod init github.com/steady-bytes/draft/api/go-connect; go mod tidy;
