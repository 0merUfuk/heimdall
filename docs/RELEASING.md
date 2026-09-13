# Releasing heimdall

## One-time setup (owner action required)

Two things need to exist before the first tag-triggered release can succeed. Neither can be created by an agent autonomously -- both are account-level actions.

1. **Create the Homebrew tap repository**: an empty public GitHub repo at `github.com/0merUfuk/homebrew-heimdall`. GoReleaser pushes the generated cask file to it on every release; it does not create the repo itself.
2. **Create `HOMEBREW_TAP_TOKEN`**: a GitHub Personal Access Token (fine-grained, `contents:write` scoped to just the `homebrew-heimdall` repo) added as a repository secret on `0merUfuk/heimdall`. The default `GITHUB_TOKEN` in Actions is scoped only to the repo the workflow runs in, so it cannot push to a second repository.

Without both, `.github/workflows/release.yml` will fail at the "homebrew cask" step -- everything else (build, archive, checksum, GitHub Release) still succeeds.

## Release process

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

Pushing a `v*` tag triggers `.github/workflows/release.yml`, which runs `goreleaser release --clean`. That:
- Builds `heimdall` (darwin/amd64 + darwin/arm64) and bundles the universal `heimdall-audio` Swift binary (see `Makefile`'s `audio-helper-universal` target and `.claude/DECISIONS.md` for why the path-detection there is fragile)
- Publishes a GitHub Release with archives + `checksums.txt`, changelog generated from commits since the previous tag
- Pushes an updated cask to `0merUfuk/homebrew-heimdall` (once the one-time setup above is done)

To dry-run locally without publishing anything: `goreleaser release --snapshot --clean`.

## Known gap: code signing

Release binaries are **not code-signed or notarized** -- that requires an Apple Developer Program account (paid, tied to the owner's Apple ID), which is a credential only the repo owner can provision. Until then, `.goreleaser.yml`'s `homebrew_casks` config includes a `postflight` hook that strips the `com.apple.quarantine` xattr after install, so `brew install` still works without a Gatekeeper prompt. Users building from source or downloading the tarball directly will still hit Gatekeeper's "unidentified developer" warning on first run (`xattr -d com.apple.quarantine <path>` or right-click > Open works around it). Revisit this once signing is set up: add `notarize`/`sign` steps to `.goreleaser.yml` and drop the xattr workaround.

## Stray tags

`git tag` currently lists ~18 `oracle/cycle-*` tags left over from earlier agent work -- they are not release tags and GoReleaser ignores non-semver tags for versioning, but they do clutter `git describe` output and any tooling that lists "the latest tag" (see `goreleaser release --snapshot` output, which picks up the most recent `oracle/cycle-*` tag chronologically rather than a real version). Left in place pending an owner decision on whether to delete them -- not done unilaterally since their purpose in the multi-agent workflow that created them isn't fully clear from git history alone.
