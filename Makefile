APP := ankiced
WEB_APP := ankiced-web
DESKTOP_APP := ankiced-desktop

.PHONY: help test test-unit test-integration test-e2e run run-web run-desktop run-desktop-dev build build-web build-desktop fmt tidy vet

help:
	@echo "Available targets:"
	@echo "  test              - run all tests"
	@echo "  test-unit         - run unit tests (internal/*)"
	@echo "  test-integration  - run integration tests"
	@echo "  test-e2e          - run e2e tests"
	@echo "  run               - run CLI app"
	@echo "  run-web           - run web app (API + browser open)"
	@echo "  run-desktop       - run desktop app (quiet mode)"
	@echo "  run-desktop-dev   - run desktop app (dev mode, verbose logs)"
	@echo "  build             - build binary to ./bin/ankiced"
	@echo "  build-web         - build binary to ./bin/ankiced-web"
	@echo "  build-desktop     - build desktop app binary"
	@echo "  fmt               - format code"
	@echo "  tidy              - go mod tidy"
	@echo "  vet               - go vet"

test:
	go test ./...

test-unit:
	go test ./internal/...

test-integration:
	go test ./tests/integration/...

test-e2e:
	go test ./tests/e2e/...

run:
	go run ./cmd/ankiced

run-web:
	go run ./cmd/ankiced-web

run-desktop:
	go run -tags production ./cmd/ankiced-desktop

run-desktop-dev:
	go run -tags dev ./cmd/ankiced-desktop

build:
	go build -o ./bin/$(APP) ./cmd/ankiced

build-web:
	go build -o ./bin/$(WEB_APP) ./cmd/ankiced-web

build-desktop:
	go build -tags production -o ./bin/$(DESKTOP_APP) ./cmd/ankiced-desktop

fmt:
	gofmt -w ./cmd ./internal ./tests

tidy:
	go mod tidy

vet:
	go vet ./...
