VERSION ?= 0.1.0-dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test heimdall

build: heimdall

heimdall:
	go build $(LDFLAGS) -o bin/heimdall ./cmd/heimdall

clean:
	rm -rf bin/

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

.PHONY: doctor
doctor:
	@echo "Checking prerequisites..."
	@which go > /dev/null 2>&1 && echo "  ✓ Go installed" || echo "  ✗ Go not found"
	@sw_vers -productVersion 2>/dev/null | awk -F. '{if ($$1>=14 && $$2>=2) print "  ✓ macOS " $$0 " (>=14.2)"; else print "  ✗ macOS " $$0 " (<14.2 — Core Audio Taps not available)"}' || echo "  ✗ Not macOS"
	@which swift > /dev/null 2>&1 && echo "  ✓ Swift compiler available" || echo "  ✗ Swift not found"
