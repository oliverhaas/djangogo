GO  ?= go
PKG := ./...

.PHONY: all build run test test-race cover vet fmt tidy lint clean

all: fmt vet test

build:
	$(GO) build $(PKG)

run:
	$(GO) run ./cmd/djangogo

test:
	$(GO) test $(PKG)

test-race:
	$(GO) test -race $(PKG)

cover:
	$(GO) test -race -coverprofile=coverage.txt -covermode=atomic $(PKG)
	$(GO) tool cover -func=coverage.txt | tail -1

vet:
	$(GO) vet $(PKG)

fmt:
	$(GO) fmt $(PKG)

tidy:
	$(GO) mod tidy

lint:
	golangci-lint run

clean:
	rm -f coverage.txt coverage.html
	rm -rf bin dist
