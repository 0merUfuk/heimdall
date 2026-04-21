# heimdall — Privacy Notice

**Last Updated**: 2026-04-21

heimdall is a local-first CLI tool. It runs on your machine, with your API keys, and writes meeting notes to a local Obsidian vault. This document explains what data heimdall processes, where that data goes, and what that means for you as a user, a data controller, or a compliance reviewer.

This is not legal advice. If you are adopting heimdall inside an organization, consult qualified counsel.

## 1. What data heimdall processes

- **Microphone audio** — captured locally via `malgo` (16 kHz, 16-bit mono PCM).
- **System audio** — captured locally on macOS 14.2+ via Apple's Core Audio Taps API (Swift helper `heimdall-audio`, 48 kHz → 16 kHz stereo → mono, see `.claude/DECISIONS.md` ID-001).
- **Meeting-note text output** — transcripts, summaries, action items, and speaker maps generated during and after a recording.
- **API keys** — Deepgram and Anthropic credentials, stored in the local config file or provided via environment variables.

## 2. Where data goes — data-flow table

| Data | Destination | Provider | Retention |
|------|-------------|----------|-----------|
| Microphone + system audio (PCM, streamed over WebSocket) | Transcription | Deepgram Inc. (US; EU endpoint `api.eu.deepgram.com` available via config) | Zero retention beyond the live session. heimdall sets `mip_opt_out=true` by default so audio is **not** retained for model training. See [Deepgram's Model Improvement Partnership docs](https://developers.deepgram.com/docs/the-deepgram-model-improvement-partnership-program). |
| Transcript text (HTTPS request body) | Summarization / speaker-map / action-item generation | Anthropic PBC (US) | 7-day API log retention per [Anthropic's retention policy](https://privacy.claude.com/en/articles/10023548-how-long-do-you-store-my-data) (as of 2025-09-15). API data is excluded from model training by default under Anthropic's [Commercial Terms](https://www.anthropic.com/legal/commercial-terms). |
| Meeting notes + raw transcript (markdown files) | Local Obsidian vault | Your machine | User-controlled. Note: vaults synced via iCloud / Dropbox / Obsidian Sync extend the data trail to those providers. |
| API keys | `~/Library/Application Support/heimdall/config.yaml` or environment variables | Your machine | User-controlled. Mode `0600` recommended. |
| Crash-recovery snapshots | `~/.heimdall/recovery/*.json` | Your machine | Written every 30 seconds during recording, mode `0600`, deleted on clean shutdown (see V-006 in `docs/architecture/ASSESSMENT.md`). |

## 3. Data controller

**You are the GDPR data controller.** heimdall is a local CLI tool, not a service. Deepgram and Anthropic are sub-processors you have contracted with **directly** via your own API keys and your acceptance of their terms. heimdall-the-project has no data-processing contract with them on your behalf.

Practical implication: if a participant in a recording exercises a GDPR Article 15 (access) or Article 17 (erasure) right against your recording, **you** are the controller who must respond. heimdall does not mediate that request.

## 4. Recording consent

Recording conversations is regulated. You are responsible for ensuring all participants consent where law requires it. See the [Recording Consent section in the README](README.md#privacy-and-data-flow) for the short version.

**US all-party-consent jurisdictions** (verify against the current state-by-state reference before relying on this list): California, Connecticut, Delaware, Florida, Illinois, Maryland, Massachusetts, Michigan, Montana, Nevada, New Hampshire, Oregon, Pennsylvania, Washington, and the District of Columbia. Other states are generally one-party consent at the state level, but federal law and additional statutes can apply.

**Current reference**: [Justia — Recording of Conversations Laws, 50-State Survey](https://www.justia.com/50-state-surveys/recording-of-conversations-laws/).

**EU / UK / Switzerland (GDPR)**: participants must be informed, and you must have a lawful basis (usually their consent).

**Turkey (KVKK)**: explicit consent is required.

When in doubt, inform all participants at the start of the meeting that recording is active and get a verbal acknowledgement.

## 5. Biometric data notice — BIPA (Illinois)

Voiceprints created during speaker diarization may be considered biometric identifiers under the Illinois Biometric Information Privacy Act ([740 ILCS 14](https://www.ilga.gov/legislation/ilcs/ilcs3.asp?ActID=3004)).

heimdall does not store voiceprints beyond the live Deepgram session, and with `mip_opt_out=true` (default) Deepgram does not retain audio for model training. However:

- If you intend to record meetings with Illinois residents **and publish transcripts externally**, obtain written consent before recording.
- If you process voice data for employment or access-control purposes, BIPA's written-policy and retention-schedule requirements likely apply. That use case is explicitly out of scope for heimdall (see §7).

BIPA statutory damages are $1,000 per negligent violation and $5,000 per intentional violation, per person. Active class actions in 2025–2026 (Microsoft Teams, Fireflies.AI) target meeting-recording products — take it seriously.

## 6. EU / GDPR

- **EU residency**: you can point Deepgram at `api.eu.deepgram.com` via the `deepgram.endpoint` config option if your compliance posture requires EU data residency. Anthropic's API is served from the US.
- **Transfers**: both Deepgram and Anthropic self-certify under the [EU-US Data Privacy Framework](https://www.data-privacy-framework.com/) (in force as of early 2026; General Court dismissed an annulment action on 2025-09-03). If the DPF is invalidated in the future, heimdall's roadmap includes local Whisper + Ollama backends that do not send audio or text off your machine.
- **DPA**: you, not heimdall-the-project, accept Deepgram's and Anthropic's Data Processing Addenda when you sign up for their APIs.

## 7. Not for

heimdall is **not designed for**, and should not be deployed as:

- Workplace surveillance.
- Employee monitoring, performance evaluation, or HR observation.
- Biometric authentication or access control.
- A substitute for HR-approved recording workflows.
- A bot that joins meetings on behalf of an absent user.

These use cases likely fall under EU AI Act Annex III (high-risk AI systems in employment contexts, enforceable August 2026) and BIPA (Illinois biometric regime). heimdall's architecture — local CLI, user-owned keys, user-as-controller — does not defend against them if you repurpose the tool.

## 8. Questions / contact

Please file an issue at [github.com/0merUfuk/heimdall/issues](https://github.com/0merUfuk/heimdall/issues) for privacy questions, data-flow corrections, or documentation gaps.

For security-sensitive reports (vulnerabilities, credential leaks), see [SECURITY.md](SECURITY.md) instead — do not report those via public issues.
