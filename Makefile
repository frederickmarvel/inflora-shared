.PHONY: build generate test lint tidy

build:
	go build ./...

generate:
	cd proto && protoc -I . \
		--go_out=../gen/go --go_opt=paths=source_relative \
		--go-grpc_out=../gen/go --go-grpc_opt=paths=source_relative \
		$$(find . -name '*.proto' -type f | sort)
	rm -rf gen/go/proto

test:
	go test -race ./...

lint:
	golangci-lint run

tidy:
	go mod tidy
