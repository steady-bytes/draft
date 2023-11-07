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

## go install github.com/fluidtruck/protoc-gen-go-timestamppb-sql@v1.0.0

clean:
	rm -rf gen/go

go:
	rm -rf gen/go
	buf mod update
	buf generate --template buf.gen.go.yaml
	buf generate --template buf.gen.gotag.yaml
	$(MAKE) init-go-mod

js:
	rm -rf gen/go
	buf mod update
	buf generate --template buf.gen.local.js.yaml
	$(MAKE) init-go-mod

go-diff:
	rm -f gen/go/go.mod
	rm -f gen/go/go.sum
	buf mod update
	./diff-buf-gen.sh
	$(MAKE) init-go-mod

init-go-mod:
	cd gen/go; go mod init github.com/steady-bytes/draft/api; go mod tidy
