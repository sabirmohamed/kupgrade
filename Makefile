.PHONY: build run test lint clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/sabirmohamed/kupgrade/internal/cli.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/kupgrade ./cmd/kupgrade

run: build
	./bin/kupgrade watch

test:
	go test -race ./...

lint:
	golangci-lint run

clean:
	rm -rf bin/

deps:
	go mod tidy
