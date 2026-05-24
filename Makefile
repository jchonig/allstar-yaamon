APP     := yaamon
CMD     := .
DIST    := ./dist
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -s -w"

.PHONY: all build test test-unit test-integration lint clean run \
        docker-build docker-build-multi docker-run snapshot install deps

all: build

build:
	mkdir -p $(DIST)
	go build $(LDFLAGS) -o $(DIST)/$(APP) $(CMD)

run: build
	$(DIST)/$(APP) serve --config config.yaml

test: test-unit docker-build test-integration

test-unit:
	go test ./... -race -count=1

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint:
	golangci-lint run ./...

deps:
	go mod tidy
	go mod verify

build-all: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(DIST)/$(APP)-linux-amd64 $(CMD)

build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(DIST)/$(APP)-linux-arm64 $(CMD)

docker-build:
	docker build -t yaamon:dev .

docker-build-multi:
	docker buildx build --platform linux/amd64,linux/arm64 -t yaamon:dev .

docker-run:
	docker run --rm -p 8080:80 \
	  -v $(PWD)/config.yaml:/etc/yaamon/config.yaml \
	  -v $(PWD)/state.yaml:/etc/yaamon/state.yaml \
	  -e YAAMON_STATE_FILE=/etc/yaamon/state.yaml \
	  yaamon:dev

# Start container with test/ config+data, run integration tests, stop container.
# test/data/ is preserved after the run for post-failure inspection.
test-integration:
	@mkdir -p test/data
	@echo "Starting test container..."
	$(eval CID := $(shell docker run -d \
	  -v $(PWD)/test/config.yaml:/etc/yaamon/config.yaml:ro \
	  -v $(PWD)/test/data:/data \
	  -v $(PWD)/test/state.yaml:/etc/yaamon/state.yaml:ro \
	  -e YAAMON_STATE_FILE=/etc/yaamon/state.yaml \
	  -e TEST_ADMIN_PASSWORD=testpassword \
	  -p 18080:80 \
	  yaamon:dev))
	@trap 'echo "Stopping container $(CID)..."; docker stop $(CID) >/dev/null' EXIT; \
	  echo "Waiting for container $(CID)..."; \
	  until curl -sf http://localhost:18080/health >/dev/null 2>&1; do sleep 1; done; \
	  echo "Container ready."; \
	  YAAMON_TEST_URL=http://localhost:18080 \
	  TEST_ADMIN_PASSWORD=testpassword \
	  go test ./integration_tests/... -v -tags=integration -timeout=120s

snapshot:
	goreleaser release --snapshot --clean

install: build
	sudo cp $(DIST)/$(APP) /usr/local/bin/$(APP)
	sudo chmod 755 /usr/local/bin/$(APP)

install-service: install
	sudo cp contrib/yaamon.service /etc/systemd/system/
	sudo systemctl daemon-reload
	sudo systemctl enable yaamon
	sudo systemctl start yaamon
	@echo "Service started. Check status with: sudo systemctl status yaamon"

uninstall-service:
	sudo systemctl stop yaamon || true
	sudo systemctl disable yaamon || true
	sudo rm -f /etc/systemd/system/yaamon.service
	sudo systemctl daemon-reload

clean:
	rm -rf $(DIST) coverage.out coverage.html

version:
	@echo $(VERSION)
