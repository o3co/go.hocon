.PHONY: help test lint check

help:
	@echo "Targets:"
	@echo "  make test   — go test -race ./..."
	@echo "  make lint   — golangci-lint run"
	@echo "  make check  — test + lint"

test:
	go test -race ./...

lint:
	golangci-lint run

check: test lint
