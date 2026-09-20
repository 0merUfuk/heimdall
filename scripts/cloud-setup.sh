#!/usr/bin/env bash
# Cloud environment setup for heimdall -- Codex cloud environments and
# Claude Code on the web (both run Linux containers).
#
# What it does (idempotent; safe to re-run):
#   1. Ensures the exact Go version go.mod requires is on PATH. Preferred
#      source: Go's own toolchain module via proxy.golang.org (always
#      reachable from Claude Code cloud VMs, whose default allowlist does not
#      include dl.google.com, where go.dev/dl redirects). Fallback when the
#      image has no Go >= 1.21: the official go.dev tarball.
#   2. Ensures a C compiler exists (cgo is required: internal/audio uses
#      malgo/miniaudio).
#   3. Downloads all modules and builds + vets every package, so the agent
#      phase works even when the container has no internet access (Codex
#      cloud's default after setup).
#
# What a cloud container can NOT do: capture audio (Core Audio Taps + the
# Swift helper are macOS-only), run whisper.cpp, or reach a local Ollama.
# Unit tests, `go vet`, lint, and `heimdall eval` against a remote backend
# all work. See AGENTS.md "Cloud environments".
#
# Usage:
#   scripts/cloud-setup.sh              # Codex cloud "Setup script" field
#   scripts/cloud-setup.sh --if-remote  # Claude Code SessionStart hook: no-op
#                                       # unless CLAUDE_CODE_REMOTE=true
set -euo pipefail

if [[ "${1:-}" == "--if-remote" && "${CLAUDE_CODE_REMOTE:-}" != "true" ]]; then
  exit 0
fi

cd "$(dirname "$0")/.."

log() { printf '[cloud-setup] %s\n' "$*" >&2; }

if [[ "$(uname -s)" != "Linux" ]]; then
  log "not Linux ($(uname -s)); this script targets cloud containers. On macOS use: make build && make test"
  exit 0
fi

go_required="$(awk '$1 == "go" { print $2; exit }' go.mod)"

# Official SHA-256 sums for the go.mod toolchain's Linux tarballs, from
# https://go.dev/dl/?mode=json -- pinned here so the fallback install is
# verified against a value committed to this repo rather than one fetched
# from the same place as the tarball. Update both when go.mod's Go version
# changes; an unpinned version fails closed instead of installing blind.
declare -A go_sha256=(
  [amd64]=63d339f0da5ab53635a56f2490a7984dfe12dfcff22ad749f63edaf590168445
  [arm64]=3450b45a3f9ee8568792736a5c5e70a1f2e9b36c35a8f74958c03e51d7d92bec
)
go_sha256_version="1.27.1"
if [[ "$go_required" != "$go_sha256_version" ]]; then
  # Stale pins: keep the verified module-proxy path, refuse the tarball one.
  unset 'go_sha256[amd64]' 'go_sha256[arm64]'
fi

# version_ge A B: true when version A >= version B (dotted numeric).
version_ge() {
  [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" == "$2" ]]
}

current_go_version() {
  command -v go >/dev/null 2>&1 || return 1
  go env GOVERSION 2>/dev/null | sed 's/^go//'
}

# Make the installed Go visible to every later shell. Agent shells are
# usually non-interactive, and Ubuntu's ~/.bashrc returns early for those
# (verified in an ubuntu:24.04 container), so an appended PATH line alone is
# not enough. Primary: symlink into /usr/local/bin, which is on PATH for all
# shell types (cloud containers run as root or with sudo; cmd/go resolves
# GOROOT through the symlink). Fallbacks: CLAUDE_ENV_FILE (Claude Code on the
# web SessionStart hooks) and a PATH line at the TOP of ~/.bashrc/~/.profile.
persist_path() {
  local dir="$1" line="export PATH=\"$1:\$PATH\""
  export PATH="$dir:$PATH"

  local sudo_cmd=""
  [[ "$(id -u)" -ne 0 ]] && command -v sudo >/dev/null 2>&1 && sudo_cmd="sudo -n"
  if $sudo_cmd mkdir -p /usr/local/bin 2>/dev/null &&
    $sudo_cmd ln -sf "$dir/go" /usr/local/bin/go 2>/dev/null &&
    $sudo_cmd ln -sf "$dir/gofmt" /usr/local/bin/gofmt 2>/dev/null; then
    log "linked go and gofmt into /usr/local/bin"
  fi

  # An older Go that the image puts earlier on PATH (e.g. /usr/local/go/bin
  # in the golang images, which also set GOTOOLCHAIN=local) would still win
  # in a non-interactive shell and refuse to build this module -- verified in
  # a golang:1.24 container. Repoint each such stale binary at the required
  # toolchain. Disposable cloud containers only: this script exits early on
  # anything that is not Linux.
  local other ver
  while read -r other; do
    [[ -z "$other" || "$other" == "$dir/go" ]] && continue
    ver="$("$other" env GOVERSION 2>/dev/null | sed 's/^go//')" || continue
    if [[ -n "$ver" ]] && ! version_ge "$ver" "$go_required"; then
      if $sudo_cmd ln -sf "$dir/go" "$other" 2>/dev/null; then
        $sudo_cmd ln -sf "$dir/gofmt" "$(dirname "$other")/gofmt" 2>/dev/null || true
        log "repointed stale Go $ver at $other -> Go $go_required"
      else
        log "warning: stale Go $ver at $other shadows Go $go_required and is not writable; prepend $dir to PATH"
      fi
    fi
  done < <(type -ap go | awk '!seen[$0]++')

  if [[ -n "${CLAUDE_ENV_FILE:-}" ]]; then
    echo "$line" >>"$CLAUDE_ENV_FILE"
  fi
  local rc
  for rc in "$HOME/.bashrc" "$HOME/.profile"; do
    if ! grep -qsF "$line" "$rc"; then
      # Write back in place (not mv) so the file keeps its own mode/owner.
      { echo "$line"; cat "$rc" 2>/dev/null || true; } >"$rc.heimdall-tmp" &&
        cat "$rc.heimdall-tmp" >"$rc" && rm -f "$rc.heimdall-tmp"
      log "prepended a PATH line to $rc"
    fi
  done
}

# install_go_toolchain_module downloads the exact toolchain through the Go
# module proxy using an existing go >= 1.21, and prints its GOROOT.
install_go_toolchain_module() {
  local have="$1" arch="$2"
  version_ge "$have" "1.21" || return 1
  GOTOOLCHAIN="go${go_required}" go version >/dev/null 2>&1 || return 1
  local root
  root="$(GOTOOLCHAIN=local go env GOMODCACHE)/golang.org/toolchain@v0.0.1-go${go_required}.linux-${arch}"
  [[ -x "$root/bin/go" ]] || return 1
  echo "$root"
}

install_go() {
  local arch
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) log "unsupported architecture $(uname -m)"; exit 1 ;;
  esac
  local root
  if [[ -n "$have_go" ]] && root="$(install_go_toolchain_module "$have_go" "$arch")"; then
    log "installed Go $go_required via the Go module proxy (toolchain module)"
    persist_path "$root/bin"
    return
  fi
  local dest="$HOME/.local/go-$go_required"
  if [[ ! -x "$dest/bin/go" ]]; then
    local want="${go_sha256[$arch]:-}"
    if [[ -z "$want" ]]; then
      log "no pinned SHA-256 for go${go_required}.linux-${arch}; refusing to install an unverified toolchain."
      log "add it to go_sha256 in this script from https://go.dev/dl/?mode=json"
      exit 1
    fi
    local url="https://go.dev/dl/go${go_required}.linux-${arch}.tar.gz"
    local tgz="$dest.tar.gz"
    log "installing Go $go_required from $url"
    mkdir -p "$dest"
    curl -fsSL -o "$tgz" "$url"
    # Verify before extracting: the toolchain runs on every later build, so
    # a tampered tarball would be code execution in the container. The
    # preferred path above (the Go module proxy) is already verified against
    # Go's checksum database; this fallback needs its own check.
    local got
    got="$(shasum -a 256 "$tgz" 2>/dev/null | awk '{print $1}')"
    [[ -z "$got" ]] && got="$(sha256sum "$tgz" | awk '{print $1}')"
    if [[ "$got" != "$want" ]]; then
      rm -f "$tgz"
      log "SHA-256 mismatch for $url"
      log "  expected $want"
      log "  got      $got"
      exit 1
    fi
    tar -xzf "$tgz" -C "$dest" --strip-components=1
    rm -f "$tgz"
  fi
  persist_path "$dest/bin"
}

have_go="$(current_go_version || true)"
if [[ -n "$have_go" ]] && version_ge "$have_go" "$go_required"; then
  log "Go $have_go already satisfies go.mod ($go_required)"
else
  log "Go ${have_go:-none} is older than go.mod requires ($go_required)"
  install_go
fi
# The official golang images set GOTOOLCHAIN=local; the exact toolchain is
# now on PATH, so never let the go command try to switch.
export GOTOOLCHAIN=local

if ! command -v cc >/dev/null 2>&1 && ! command -v gcc >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    log "installing a C compiler for cgo (malgo)"
    sudo_cmd=""
    [[ "$(id -u)" -ne 0 ]] && sudo_cmd="sudo"
    $sudo_cmd apt-get update -qq
    $sudo_cmd apt-get install -y -qq gcc libc6-dev >/dev/null
  else
    log "no C compiler and no apt-get: install gcc manually (cgo is required)"
    exit 1
  fi
fi
export CGO_ENABLED=1

log "downloading modules"
go mod download

log "building and vetting all packages"
go build ./...
go vet ./...

log "ready: $(go version). Run 'go test ./... -race -count=1' (or 'make test')."
