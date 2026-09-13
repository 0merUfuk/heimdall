VERSION ?= 0.1.0-dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build clean test heimdall audio-helper audio-helper-universal lint doctor vuln eval

build: heimdall audio-helper

heimdall:
	mkdir -p bin/
	go build $(LDFLAGS) -o bin/heimdall ./cmd/heimdall

audio-helper:
	cd audio-helper && swift build -c release
	mkdir -p bin/
	cp audio-helper/.build/release/heimdall-audio bin/heimdall-audio

# Universal (arm64 + x86_64) build for release artifacts. GoReleaser ships a
# single archive per Go arch, so the bundled Swift helper must be a fat binary
# to run on both Apple Silicon and Intel Macs.
#
# Where SPM writes the universal binary has moved at least once already
# across Swift toolchain versions (Xcode 15-era: .build/apple/Products/
# Release/; current: .build/out/Products/Release/) -- this was never caught
# because no release had ever actually been run end-to-end until this was
# found via a goreleaser snapshot build. `find` locates it by name instead
# of hardcoding a path that has already gone stale once.
#
# The path pattern must match .../Products/Release/ specifically, not just
# */Release/* -- SPM's own intermediate per-architecture object files live
# under paths like .build/out/Intermediates.noindex/heimdall-audio.build/
# Release/heimdall-audio-p.build/Objects-normal/arm64/Binary/heimdall-audio,
# which also contains the substring "/Release/" and is a single-arch
# (non-universal) file. A looser pattern intermittently matched one of
# those instead of the real product, depending on find's traversal order --
# caught by `file`-checking the result for two architectures below, not by
# inspection alone.
audio-helper-universal:
	cd audio-helper && swift build -c release --arch arm64 --arch x86_64
	mkdir -p bin/
	@bin_path=$$(find audio-helper/.build -type f -name heimdall-audio -path '*/Products/Release/*' -not -path '*.dSYM*' | head -1); \
	if [ -z "$$bin_path" ]; then \
		echo "error: could not locate the built universal heimdall-audio binary under audio-helper/.build/" >&2; \
		exit 1; \
	fi; \
	if ! file "$$bin_path" | grep -q "2 architectures"; then \
		echo "error: $$bin_path is not a universal (arm64+x86_64) binary -- got: $$(file "$$bin_path")" >&2; \
		exit 1; \
	fi; \
	cp "$$bin_path" bin/heimdall-audio

clean:
	rm -rf bin/
	rm -rf audio-helper/.build/

test:
	go test ./... -race -count=1

lint:
	go vet ./...
	gofmt -l . | grep -v '^audio-helper/.build' | (! grep .)
	golangci-lint run --disable errcheck ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Meeting-analysis quality suite against golden transcripts (see
# internal/eval and docs/EVALUATION.md). Needs a built binary and either
# ANTHROPIC_API_KEY or a logged-in `claude` CLI (--analyzer claude-code).
eval: heimdall
	./bin/heimdall eval

doctor:
	@echo "Checking prerequisites..."
	@which go > /dev/null 2>&1 && echo "  ✓ Go installed" || echo "  ✗ Go not found"
	@sw_vers -productVersion 2>/dev/null | awk -F. '{if ($$1>=14 && $$2>=2) print "  ✓ macOS " $$0 " (>=14.2)"; else print "  ✗ macOS " $$0 " (<14.2 — Core Audio Taps not available)"}' || echo "  ✗ Not macOS"
	@which swift > /dev/null 2>&1 && echo "  ✓ Swift compiler available" || echo "  ✗ Swift not found"
	@test -f bin/heimdall-audio && echo "  ✓ heimdall-audio binary built" || echo "  ✗ heimdall-audio not built (run: make audio-helper)"
