.PHONY: build generate test lint tidy

build:
	go build ./...

generate:
	buf generate

test:
	go test -race ./...

lint:
	golangci-lint run

tidy:
	go mod tidy
