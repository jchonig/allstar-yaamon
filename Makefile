APP     := yaamon
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Builder image for tests, lint, and deps — Debian-based so the race detector works.
BUILDER := golang:1.26

# Mount the host Go module and build caches so Docker runs don't re-download modules.
GOMODCACHE := $(shell go env GOMODCACHE 2>/dev/null || echo $(HOME)/go/pkg/mod)
GOCACHE    := $(shell go env GOCACHE    2>/dev/null || echo $(HOME)/.cache/go-build)

DOCKER_GO := docker run --rm \
  -v "$(CURDIR):/src" \
  -v "$(GOMODCACHE):/go/pkg/mod" \
  -v "$(GOCACHE):/root/.cache/go-build" \
  -w /src \
  $(BUILDER)

# Names and credentials used by the integration test setup.
TEST_NET             := yaamon-test-net
TEST_SUT             := yaamon-sut
TEST_ADMIN_PASSWORD  := testpassword
TEST_VIEWER_PASSWORD := viewerpassword

.PHONY: all build build-multi test test-unit coverage lint deps \
        compile run stop logs watch test-integration snapshot \
        install-service uninstall-service version clean cleanall

## Default — build the yaamon Docker image for the current platform.
all: build

## Build the Docker image.
build:
	docker build \
	  --build-arg TARGETOS=linux \
	  --build-arg TARGETARCH=$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') \
	  -t yaamon:dev .

## Build multi-arch Docker image (requires buildx).
build-multi:
	docker buildx build --platform linux/amd64,linux/arm64 -t yaamon:dev .

## Full test suite: unit tests → build image → integration tests.
test: test-unit build test-integration

## Run unit tests inside Docker.
test-unit:
	$(DOCKER_GO) go test ./... -race -count=1

## Run unit tests with coverage report (output written to host via volume mount).
coverage:
	$(DOCKER_GO) sh -c "go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out -o coverage.html"
	@echo "Coverage report: coverage.html"

## Run linter inside Docker.
lint:
	docker run --rm \
	  -v "$(CURDIR):/src" \
	  -v "$(GOMODCACHE):/go/pkg/mod" \
	  -w /src \
	  golangci/golangci-lint:latest golangci-lint run ./...

## Tidy and verify go.mod/go.sum inside Docker.
deps:
	$(DOCKER_GO) sh -c "go mod tidy && go mod verify"

## Cross-compile for the dev container (linux, native arch, no CGO).
## While 'make watch' is running, this triggers a fast container restart instead of a full image rebuild.
compile:
	CGO_ENABLED=0 GOOS=linux go build -o test/yaamon .

## Start the server in the background on http://localhost:8080.
## test/config/ is mounted read-only at /etc/yaamon; test/data/ persists the DB.
## Override credentials: TEST_ADMIN_PASSWORD=xxx TEST_VIEWER_PASSWORD=xxx make run
run:
	mkdir -p test/data
	docker compose -f test/docker-compose.yml up -d --build

## Stop the server.
stop:
	docker compose -f test/docker-compose.yml down

## Follow server logs.
logs:
	docker compose -f test/docker-compose.yml logs -f

## Start docker-compose in foreground watch mode.
## In a separate terminal, run 'make compile' to push binary updates without a full image rebuild.
watch:
	docker compose -f test/docker-compose.yml watch

## Integration tests: start the yaamon container and a Go test runner on a shared
## Docker network. test/data/ is preserved after the run for post-failure inspection.
test-integration:
	@docker rm -f $(TEST_SUT) 2>/dev/null; \
	docker network rm $(TEST_NET) 2>/dev/null; \
	mkdir -p test/data; \
	docker network create $(TEST_NET); \
	docker run -d \
	  --name $(TEST_SUT) \
	  --network $(TEST_NET) \
	  -v "$(CURDIR)/test/config:/etc/yaamon:ro" \
	  -v "$(CURDIR)/test/data:/data" \
	  -e YAAMON_STATE_FILE=/etc/yaamon/state.yaml \
	  -e TEST_ADMIN_PASSWORD=testpassword \
	  -e TEST_VIEWER_PASSWORD=viewerpassword \
	  yaamon:dev; \
	echo "Waiting for server..."; \
	timeout 30 sh -c 'until docker exec $(TEST_SUT) curl -sf http://localhost/health >/dev/null 2>&1; do sleep 1; done'; \
	echo "Server ready. Running integration tests..."; \
	docker run --rm \
	  --network $(TEST_NET) \
	  -v "$(CURDIR):/src" \
	  -v "$(GOMODCACHE):/go/pkg/mod" \
	  -v "$(GOCACHE):/root/.cache/go-build" \
	  -w /src \
	  -e YAAMON_TEST_URL=http://$(TEST_SUT):80 \
	  -e TEST_ADMIN_PASSWORD=testpassword \
	  -e TEST_VIEWER_PASSWORD=viewerpassword \
	  $(BUILDER) \
	  go test ./integration_tests/... -v -tags=integration -timeout=120s; \
	EXIT=$$?; \
	docker stop $(TEST_SUT) >/dev/null 2>&1; \
	docker rm   $(TEST_SUT) >/dev/null 2>&1; \
	docker network rm $(TEST_NET) >/dev/null 2>&1; \
	exit $$EXIT

## Local GoReleaser snapshot (builds all release artifacts without publishing).
snapshot:
	goreleaser release --snapshot --clean

## Install and enable systemd service on a Linux host (not for Docker installs).
install-service:
	sudo cp contrib/yaamon.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable yaamon
	sudo systemctl start yaamon
	@echo "Service started. Check: sudo systemctl status yaamon"

uninstall-service:
	sudo systemctl stop yaamon    || true
	sudo systemctl disable yaamon || true
	sudo rm -f /etc/systemd/system/yaamon.service
	sudo systemctl daemon-reload

## Remove build artifacts.
clean:
	rm -rf coverage.out coverage.html

## Remove build artifacts and test state (DB, WAL files).
cleanall: clean
	rm -rf test/data/

version:
	@echo $(VERSION)
