.PHONY: build test lint tidy

build:
	go build ./...

test:
	go test -race ./...

lint:
	golangci-lint run

tidy:
	go mod tidy
