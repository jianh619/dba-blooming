.PHONY: build test test-unit test-e2e test-integration coverage lint clean

BINARY_NAME=pgdba
BUILD_DIR=bin

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/pgdba

test:
	go test ./... -v -race

test-unit:
	go test ./tests/unit/... -v -race

test-e2e: build
	go test ./tests/e2e/... -v -timeout 60s

# Requires a running Docker Compose cluster (deployments/docker/).
# Exercises all pgdba features against the real Patroni API.
test-integration: build
	go test -tags integration ./tests/integration/... -v -timeout 120s

coverage:
	go test ./tests/unit/... -coverprofile=coverage.out -covermode=atomic \
		-coverpkg=github.com/luckyjian/pgdba/internal/...
	go tool cover -func=coverage.out
	@TOTAL=$$(go tool cover -func=coverage.out | grep "^total:" | awk '{gsub(/%/,""); print int($$3)}'); \
	if [ "$$TOTAL" -lt 80 ]; then echo "Coverage $$TOTAL% is below 80%"; exit 1; fi; \
	echo "Coverage $$TOTAL% meets the 80% requirement"

lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR)/ coverage.out
