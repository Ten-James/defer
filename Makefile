BINARY  := defer
MODULE  := github.com/Ten-James/defer
VERSION := 1.0.0
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS := -s -w

.PHONY: build install uninstall clean fmt vet test

build:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) .

install:
	go build -ldflags '$(LDFLAGS)' -o $(BINARY) .
	mv $(BINARY) /usr/local/bin/

uninstall:
	rm -f /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...
