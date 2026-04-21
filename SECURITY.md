# heimdall — Security Policy

## Reporting a vulnerability

Please report security vulnerabilities privately. Do **not** open a public GitHub issue for security-sensitive findings.

Preferred channels (in order):

1. **GitHub Security Advisories** — open a private advisory at [github.com/0merUfuk/heimdall/security/advisories/new](https://github.com/0merUfuk/heimdall/security/advisories/new).
2. **Email** — `trypix.ai@gmail.com`.

We commit to:

- **Acknowledge** receipt within 72 hours.
- Work toward a fix or mitigation under a **90-day coordinated disclosure** window. If we need more time, we will say so and coordinate a revised date.
- Credit reporters in release notes unless anonymity is requested.

## Scope

**In scope**:

- The `heimdall` Go binary (`cmd/heimdall/` + `internal/` packages).
- The `heimdall-audio` Swift helper (`audio-helper/`).
- Build artifacts distributed via official release channels (GitHub Releases, Homebrew tap once published).

**Out of scope**:

- Vulnerabilities in Deepgram, Anthropic, Obsidian, or macOS itself — please report those to the respective vendors.
- Misconfigurations on the user's own machine (file permissions, vault sync setup, shell history containing keys, etc.).
- Social engineering or physical access to a user's machine.
- Issues that require a malicious or compromised operating system to exploit.

## Supported versions

heimdall is in v0.x. While on the 0.x line, **only the latest minor release** receives security fixes. Once v1.0 ships, a version-support matrix will be published here.

| Version | Supported |
|---------|-----------|
| latest 0.x minor | yes |
| older 0.x | no |

## Known sensitive surfaces

- **API keys** (`~/Library/Application Support/heimdall/config.yaml`) — owner's responsibility; mode `0600` recommended. `config set` and `config get` mask values by default (see [`.claude/KNOWN_ISSUES.md`](.claude/KNOWN_ISSUES.md) SEC-01, SEC-02).
- **Crash-recovery files** (`~/.heimdall/recovery/*.json`) — contain partial transcripts; written mode `0600`, deleted on clean shutdown. See V-006 in [`docs/architecture/ASSESSMENT.md`](docs/architecture/ASSESSMENT.md).
- **Temporary audio buffers** — the ring buffer is memory-only; frames are not persisted to disk.
- **Swift subprocess stdout** — carries raw PCM; not logged. If you pipe heimdall's stderr to a shared location, be aware that error messages may include config paths.

## Not a substitute for provider security

heimdall streams audio to Deepgram and transcript text to Anthropic over TLS. The security of those channels depends on the providers' infrastructure. See [PRIVACY.md](PRIVACY.md) for the full data-flow table.
