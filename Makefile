VERSION ?= 0.1.0-dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test heimdall audio-helper lint doctor

build: heimdall audio-helper

heimdall:
	mkdir -p bin/
	go build $(LDFLAGS) -o bin/heimdall ./cmd/heimdall

audio-helper:
	cd audio-helper && swift build -c release
	mkdir -p bin/
	cp audio-helper/.build/release/heimdall-audio bin/heimdall-audio

clean:
	rm -rf bin/
	rm -rf audio-helper/.build/

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

doctor:
	@echo "Checking prerequisites..."
	@which go > /dev/null 2>&1 && echo "  ✓ Go installed" || echo "  ✗ Go not found"
	@sw_vers -productVersion 2>/dev/null | awk -F. '{if ($$1>=14 && $$2>=2) print "  ✓ macOS " $$0 " (>=14.2)"; else print "  ✗ macOS " $$0 " (<14.2 — Core Audio Taps not available)"}' || echo "  ✗ Not macOS"
	@which swift > /dev/null 2>&1 && echo "  ✓ Swift compiler available" || echo "  ✗ Swift not found"
	@test -f bin/heimdall-audio && echo "  ✓ heimdall-audio binary built" || echo "  ✗ heimdall-audio not built (run: make audio-helper)"
