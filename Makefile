BINARY  := exp112-pl-patch
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all test build clean

all: test build

test:
	go vet ./...
	GOOS=windows GOARCH=386 go vet ./...
	go test ./...

build:
	GOOS=windows GOARCH=386 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY).exe ./cmd/$(BINARY)

clean:
	rm -rf dist
