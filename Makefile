GO  ?= go
PKG := ./...

.PHONY: all build run test test-race test-postgres cover vet fmt fmt-check tidy lint gate clean

all: fmt vet test

build:
	$(GO) build $(PKG)

run:
	$(GO) run ./cmd/djangogo

test:
	$(GO) test -race $(PKG)

test-race: test

test-postgres:
	@if [ -z "$$DJANGOGO_TEST_POSTGRES_DSN" ]; then \
		echo "DJANGOGO_TEST_POSTGRES_DSN is not set; export it to run the PostgreSQL tests" >&2; \
		exit 1; \
	fi
	$(GO) test -race $(PKG)

cover:
	$(GO) test -race -coverprofile=coverage.txt -covermode=atomic $(PKG)
	$(GO) tool cover -func=coverage.txt | tail -1

vet:
	$(GO) vet $(PKG)

fmt:
	gofmt -w .

fmt-check:
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		echo "gofmt needed for:" >&2; \
		echo "$$files" >&2; \
		exit 1; \
	fi

tidy:
	$(GO) mod tidy

lint:
	golangci-lint run --allow-parallel-runners --max-same-issues 0 --max-issues-per-linter 0

gate: fmt-check vet lint test
	@if [ -n "$$DJANGOGO_TEST_POSTGRES_DSN" ]; then \
		$(MAKE) test-postgres; \
	else \
		echo "test-postgres skipped: DJANGOGO_TEST_POSTGRES_DSN is not set"; \
	fi

clean:
	rm -f coverage.txt coverage.html
	rm -rf bin dist
