# Heimdall — Productization Strategy v3

**Version**: 3.0
**Created**: 2026-07-21
**Authors:** Ömer Ufuk (synthesized via 5-subagent deep research)
**Status**: Proposed — awaiting ratification
**Supersedes**: v1 and v2 of this document
**Research sources**: `docs/MONETIZATION_RESEARCH.md` (27KB), `docs/DISTRIBUTION_RESEARCH.md` (24KB), MCP ecosystem research

---

## 1. The Thesis (revised)

> **Heimdall is the CLI-first, Obsidian-native meeting companion for macOS. The full local tool is free and open-source forever. Pro is a $49 one-time purchase — not a subscription. The moat is the Obsidian knowledge graph two-way bridge: heimdall writes meeting notes into your vault AND exposes them back to any AI agent via MCP. No competitor does both.**

This is the third revision. v1 proposed managed SaaS tiers ($8-12/mo). v2 revised to open-core + $99 lifetime. v3 incorporates deep research from 5 subagents: Talat's $49 one-time model works better than $99, HN/developer audiences hate subscriptions, MCP is table stakes (not a differentiator), and the Obsidian community is the real launch channel.

---

## 2. What the research changed

| Assumption (v2) | Research finding (v3) | Source |
|---|---|---|
| $9/mo subscription, $99 lifetime | **$49 one-time** — Talat proved $49 pre-release → $99 at 1.0. HN audience hates subscriptions. $49 is the conversion sweet spot. | Distribution research (Talat launch analysis, HN Show HN data) |
| MCP server is a differentiator | **MCP is table stakes** — 3-4 meeting vendors already ship it (Meeting-BaaS ~1yr, Read.ai, Vexa, Talat). First-mover advantage is gone. | MCP ecosystem research (live spec + competitor analysis) |
| Cloud STT as a revenue stream | **Deprioritized** — the research shows local-first users don't want their audio in the cloud. Cloud STT contradicts the positioning. | Competitive research (Talat/Meetily positioning analysis) |
| GitHub Actions marketplace monetization | **Not viable** — the meeting/transcription category on GitHub Marketplace is empty (0 results). Actions are for CI/CD, not desktop apps. | Monetization research (GitHub Marketplace search) |
| GitHub Sponsors as supplementary revenue | **Useless below 5K stars** — $5-75/mo at 500 stars. Need 10K+ stars for $2K/mo. | Distribution research (lazygit: 132K stars → 190 sponsors → $2-5K/mo) |
| HN Show HN as a launch channel | **Flops for meeting tools** — most get 1-18 points (Talat got 6). One outlier (Trace, 205pts). Don't bet on HN. | Distribution research (HN data: 9 meeting tool Show HN posts) |
| $99 lifetime undercuts Talat's $189 | **Talat actually charges $49 pre-release, $99 at 1.0** — our $99 matches them, doesn't undercut. $49 is the real wedge. | Competitive research (talat.app live pricing) |

---

## 3. The defensible position (research-validated)

**What's commoditized by 2026** (Apple Notes, Apple FoundationModels, Meetily, Talat all do this):
- ❌ On-device transcription (Apple ships it free in macOS)
- ❌ Post-meeting summarization (Apple FoundationModels will do this fall 2026)
- ❌ Local Whisper support (Meetily, Talat, 30+ repos all have it)
- ❌ MCP server (3-4 competitors already ship it — table stakes)
- ❌ "Local-first" positioning (everyone says it now — not a differentiator)

**What stays defensible** (no competitor does these):
- ✅ **Obsidian knowledge graph two-way bridge** — heimdall writes meeting notes into the vault AND exposes meeting-enriched vault data back to AI agents via MCP. 12+ Obsidian MCP repos exist, but all expose vault *contents* — none inject meeting-derived knowledge. This is the unique play.
- ✅ **CLI-first** — every competitor is a GUI app (Talat, Meetily, Granola, Otter). No meeting CLI exists in Homebrew. Heimdall would be the first.
- ✅ **Provider-swappable architecture** — Deepgram, Soniox, Whisper.cpp, Ollama, any LLM. No competitor offers this flexibility.
- ✅ **Turkish diarization** — no competitor serves Turkish meetings with speaker identification.
- ✅ **macOS-native (Go + Swift)** — Core Audio Taps, not Electron/Tauri. This is a feature for the macOS audience, not a limitation.

---

## 4. Productization model: Open Core + $49 One-Time Pro

### 4.1 Community Edition (Free, MIT — forever)

Everything that works today stays free. The full local CLI with all providers.

| Feature | Status |
|---|---|
| `heimdall record` — full pipeline (capture → transcribe → analyze → Obsidian) | ✅ Free |
| `heimdall doctor` / `config` / `recover` / `analyze` / `list` | ✅ Free |
| Deepgram + Soniox provider support (BYO API key) | ✅ Free |
| Whisper.cpp local STT (Phase 2B — zero API keys) | ✅ Free |
| Ollama local LLM (Phase 3) | ✅ Free |
| Obsidian markdown output (YAML frontmatter, wikilinks) | ✅ Free |
| `heimdall mcp` — local MCP server (stdio, basic tools: search/get/list) | ✅ Free |
| Crash recovery, profiles, multi-language, consent | ✅ Free |

**Principle**: if it runs on the user's machine with their own keys, it's free. No feature gating on the core pipeline. MCP is free — it's table stakes (Talat includes it in all tiers; no one charges for it).

### 4.2 Pro ($49 one-time — not a subscription)

| Feature | Free CE | Pro |
|---|---|---|
| Everything in CE | ✅ | ✅ |
| **Cross-meeting intelligence** (feed last N meeting summaries as context) | — | ✅ |
| **Custom meeting templates** (standup, 1:1, planning, retro) | — | ✅ |
| **Advanced MCP tools** (get_action_items, get_decisions, vault-as-memory) | — | ✅ |
| **People pages** (auto-generated `[[Person Name]]` with meeting history) | — | ✅ |
| **Daily note integration** (auto-append meeting summaries to today's note) | — | ✅ |
| **Calendar integration** (iCal auto-populate participants) | — | ✅ |
| **`heimdall search <query>`** (full-text search across all meetings) | — | ✅ |
| **Priority GitHub issue support** | — | ✅ |
| **`heimdall upgrade --pro`** (license key activation, same binary) | — | ✅ |

**Pricing logic**:
- **$49 one-time** — matches Talat's pre-release price, undercuts their $99 1.0 price
- **Not a subscription** — HN/developer audience hates subscriptions (Talat's one-time purchase is explicitly cited as a purchase reason)
- **Same binary, license-key activation** — `heimdall upgrade --pro` enters a key, unlocks features locally. No separate download. No SaaS infrastructure.
- **All features are local** — Pro features run on the user's machine. No server costs for the developer.

**Why $49, not $99**:
- Talat charges $49 pre-release → $99 at 1.0. Heimdall is pre-1.0 → $49 is the market-clearing price.
- At 1.0, raise to $79 (still undercuts Talat's $99).
- Lifetime guarantee: "buy once, all future Pro features included."

### 4.3 Enterprise (Custom pricing — contact)

| Feature | Price |
|---|---|
| Everything in Pro | Custom |
| Team-shared meeting library (self-hosted) | Custom |
| SSO/SCIM | Custom |
| Custom model deployment | Custom |
| On-premise support | Custom |
| SLA | Custom |

Enterprise is deferred — don't build until Pro has 50+ paying users. It requires infrastructure (team library) and an enterprise sales motion that a solo developer can't execute alone.

---

## 5. What stays open-source vs. what's Pro

| Component | License | Why |
|---|---|---|
| `heimdall` Go binary (core pipeline, providers, config) | MIT | Core, always free — this is the adoption engine |
| `heimdall-audio` Swift helper | MIT | Core |
| `templates/` (Obsidian output) | MIT | Core |
| Local Whisper/Ollama adapters | MIT | Core — local-first is the positioning |
| Local MCP server (basic tools: search/get/list) | MIT | Core — MCP is table stakes, not a differentiator |
| **Cross-meeting intelligence** | Pro (feature-gated) | Unique value — no competitor does this for free |
| **Advanced MCP tools** (action items, decisions, vault-as-memory) | Pro (feature-gated) | The Obsidian knowledge graph bridge is the moat |
| **People pages + daily note integration** | Pro (feature-gated) | Obsidian-native features no competitor offers |
| **Custom templates + calendar** | Pro (feature-gated) | Power-user features |

**The line**: the core pipeline (capture → transcribe → analyze → Obsidian) is MIT. Pro features are intelligence + integration layers — they make the vault smarter, not the pipeline functional. Users can always use the free CE for the full pipeline; Pro makes it better.

**Feature gating mechanism**: license-key activation in the same binary (`heimdall upgrade --pro`). No separate codebase (unlike Meetily which uses a separate PRO codebase). The Go binary checks the license key locally and unlocks Pro features. This is simpler for a solo developer to maintain.

---

## 6. Revenue projections (conservative, research-calibrated)

Based on Talat's launch data (2-person team, $49 one-time, TC coverage) and Meetily's trajectory (25.8K stars in 8 months):

| Horizon | Users | Pro buyers | Revenue | Assumptions |
|---|---|---|---|---|
| **Month 1** | 50 free, 0 paid | 0 | $0 | Tag v0.1.0, Homebrew, Obsidian Forum post |
| **Month 3** | 100 free, 3 Pro | 3 × $49 | $147 (one-time) | 3% conversion after Obsidian Forum + TC pitch |
| **Month 6** | 200 free, 10 Pro | 10 × $49 | $490 cumulative | Whisper local mode shipped; TC coverage landed |
| **Month 12** | 500 free, 30 Pro | 30 × $49 | $1,470 cumulative | Cross-meeting intelligence + advanced MCP shipped |
| **Year 2** | 2000 free, 100 Pro | 100 × $49 | $4,900 cumulative | 1.0 release at $79; sustained growth |

**Revenue per Pro user**: $49 one-time. No recurring revenue, no server costs. Every Pro purchase is pure margin (the features are local, no infrastructure needed).

**Break-even**: $0. There are no server costs for the Pro tier — all features run locally. The only cost is the developer's time. The first Pro purchase ($49) is break-even.

**Revenue ceiling**: $5,000-10,000 in year 1-2 at 100-200 Pro buyers. This is supplementary income, not a company. That's the right scale for a personal project.

---

## 7. Distribution strategy (research-validated)

### Launch sequence

| Phase | Action | Channel | Expected outcome |
|---|---|---|---|
| **Week 1-2** | Tag v0.1.0, set up Homebrew tap, record VHS demo GIF | GitHub + Homebrew | Repo ready for public eyes |
| **Week 3** | Obsidian Forum "Share & showcase" post | forum.obsidian.md | 5,000+ views (popular posts get 40K+) |
| **Week 4** | Show HN (Tuesday 8AM ET, lead with CLI novelty + demo) | news.ycombinator.com | 10-50 points (meeting tools mostly flop on HN) |
| **Week 5-6** | Pitch TechCrunch — Sarah Perez (sarahp@techcrunch.com) | techcrunch.com | TC article (contrarian angle: CLI, one-time, anti-Goliath) |
| **Week 7-8** | r/MacApps, r/commandline, r/ObsidianMD | Reddit | Niche community adoption |
| **Month 3** | Submit to homebrew-core (after 50+ stars) | Homebrew | Lower friction install (`brew install heimdall`) |

### The demo (critical)

**Format**: VHS-generated GIF in README (Charmbracelet pattern — every tool they ship has a GIF in the README).

**30-60 second demo sequence**:
1. `heimdall record` — auto-detect meeting, start recording
2. Live transcription appearing — both speakers identified, real-time
3. Meeting ends → Claude summary generated with decisions + action items
4. Obsidian markdown file appears in vault
5. `heimdall search "what did we decide about X?"` → instant results

### The TC pitch (Talat playbook)

1. **Contrarian angle**: "Everyone else is a GUI app with a subscription. Heimdall is CLI-first, one-time $49, no cloud."
2. **Reference Granola by name** ($1.5B valuation) — TC loves David vs Goliath
3. **Bootstrapped solo dev** — this is a feature, not a limitation
4. **Privacy as table stakes, CLI as the hook** — "your audio never leaves your Mac, and it runs from your terminal"
5. **Open-source core** — credibility like Talat's AudioTee

### What NOT to bet on

- ❌ **HN as primary channel** — meeting tools flop (1-18 points average, Talat got 6)
- ❌ **GitHub Sponsors** — useless below 5K stars ($5-75/mo at 500 stars)
- ❌ **GitHub Actions marketplace** — empty for meeting tools, not viable
- ❌ **Subscriptions** — HN/developer audience hates them
- ❌ **"Local-first" as headline** — everyone says it, it's not differentiating anymore

---

## 8. Gap analysis (revised from v2, research-informed)

| # | Gap | Why it bites | Trigger | Fail-closed default |
|---|---|---|---|---|
| 1 | **Apple FoundationModels (fall 2026)** — free on-device STT + summarization | Could commoditize the entire pipeline. Apple Notes already does basic transcription for free. | WWDC 2026 announcements | Pivot to Obsidian bridge + cross-meeting intelligence (the integration layer). Apple won't build Obsidian-native output or MCP. |
| 2 | **Meetily's growth** — 25.8K stars in 8 months, still growing | Could absorb the "open-source local-first meeting" mindshare. | Monthly star count check | Differentiate on CLI-first + Obsidian-native + provider-swappable. Meetily is a GUI (Tauri), not a CLI. |
| 3 | **Talat ships MCP + Obsidian improvements** before heimdall | Talat already has MCP + Obsidian export. If they add the two-way knowledge graph bridge, heimdall's moat is gone. | Monthly Talat changelog check | Ship the Obsidian knowledge graph bridge first — it's the one thing Talat doesn't do. |
| 4 | **MCP becomes table stakes faster than expected** — if Otter, Fireflies, Granola all ship MCP by Q4 2026 | The "MCP-native" positioning loses value. | Quarterly competitor MCP check | Lead with CLI-first + Obsidian-native, not MCP-native. MCP is the interface, not the product. |
| 5 | **License key infrastructure** — how does a solo dev handle license generation, validation, fraud? | A broken license system erodes trust and is hard to build alone. | Before launching Pro tier | Use a simple license-key service (e.g. LemonSqueezy, Gumroad, or Keygen) — don't build it yourself. |
| 6 | **The $49 price may be too low** — Talat raised to $99 at 1.0 | If heimdall ships significant Pro features (cross-meeting, advanced MCP), $49 undervalues them. | At 1.0 release | Raise to $79 at 1.0 (still undercuts Talat's $99). Pre-1.0 buyers get lifetime guarantee at $49. |
| 7 | **Apple Notes + Obsidian plugin** — if someone builds an Obsidian plugin that uses Apple's free transcription + summarization | A free Obsidian-native meeting solution that doesn't need heimdall at all. | Obsidian plugin directory watch | heimdall's advantage: speaker diarization, provider-swappability, MCP, CLI composability, crash recovery. Apple Notes doesn't do diarization or structured output. |

---

## 9. Productization roadmap (aligned with STRATEGY_V2 + research)

| STRATEGY_V2 Phase | Productization deliverable | Revenue impact |
|---|---|---|
| **2A: Ship & Fix** | Tag v0.1.0, Homebrew tap, VHS demo, Obsidian Forum post, TC pitch | $0 (all free CE) |
| **2B: Local Transcription** | Whisper.cpp support (free CE) — removes "needs API keys" barrier | $0 (drives adoption — the funnel top) |
| **2C: MCP Server** | Local MCP shipped (free CE, basic tools) + **Pro features built** (advanced MCP tools, cross-meeting, templates) + **`heimdall upgrade --pro`** available at $49 | **First revenue**: $49 per Pro buyer |
| **2D: Launch** | Community posts, README with demo GIF, TC coverage | Adoption drives the Pro funnel |
| **3: Intelligence** | Cross-meeting memory (Pro), people pages (Pro), daily note integration (Pro), Ollama local LLM (free CE) | Pro tier matures; raise to $79 at 1.0 |
| **4: Platform** | Enterprise inquiries (self-hosted team deployment, custom) | Enterprise tier (custom pricing) |

**Critical path**: Pro features (cross-meeting intelligence + advanced MCP tools + custom templates) are the first revenue. They require Phase 2C (MCP server) + Phase 3 (cross-meeting) to ship first. The $49 price is available from day one of Pro.

---

## 10. Pre-launch checklist (before any revenue)

- [ ] Tag v0.1.0 on GitHub (triggers GoReleaser, creates the first release)
- [ ] Homebrew tap set up (`brew tap 0merUfuk/heimdall`) with bottles for Apple Silicon
- [ ] VHS demo GIF recorded (30-60s: record → transcribe → summary → Obsidian → search)
- [ ] README rewritten — lead with demo GIF, one-command install, "why CLI?" section
- [ ] Real-voice smoke test (STRATEGY_V2 kill-criteria gate)
- [ ] `heimdall mcp` local command shipped (Phase 2C)
- [ ] License key service chosen (LemonSqueezy / Gumroad / Keygen — don't build it)
- [ ] Obsidian Forum "Share & showcase" post drafted
- [ ] TC pitch email drafted for Sarah Perez (sarahp@techcrunch.com)

---

## 11. The honest assessment (revised)

**What makes this viable**:
- The open-source core is genuinely good (A-grade engineering, 270 tests, 4 deps, provider-agnostic)
- **CLI-first is genuinely differentiated** — no meeting CLI exists in Homebrew; every competitor is a GUI
- **Obsidian knowledge graph two-way bridge** is a real moat — 12+ Obsidian MCP repos exist, none inject meeting data
- **$49 one-time** is proven by Talat (TC coverage, users buy for privacy + one-time pricing)
- The developer's engineering capacity is the constraint, not market demand

**What makes this hard**:
- Zero users today (F grade on distribution)
- Apple FoundationModels could commoditize the pipeline in fall 2026
- Meetily has 25.8K stars and a GUI — mindshare is already taken in the "local-first open-source" niche
- MCP is table stakes, not a differentiator — 3-4 competitors already ship it
- HN Show HN mostly flops for meeting tools (1-18 points average)
- The solo developer must handle: license infrastructure, distribution, marketing, AND continued engineering

**The bet**: CLI-first + Obsidian-native + $49 one-time is a niche big enough for a solo developer to earn $1,000-5,000 in year 1-2, but not venture-scale. The Obsidian knowledge graph bridge is the one thing no competitor does — ship it first.

**The pivot trigger**: if Apple FoundationModels ships free Turkish diarization + structured output + Obsidian export in fall 2026, pivot to "the MCP bridge for Obsidian meeting data" and stop building the pipeline. The integration layer survives commoditization.

---

## References

- `docs/MONETIZATION_RESEARCH.md` — 27KB competitive pricing + monetization research
- `docs/DISTRIBUTION_RESEARCH.md` — 24KB distribution + adoption strategy research
- `docs/STRATEGY_V2.md` — engineering roadmap + go-to-market (this document adds monetization)
- `docs/GRILL_REPORT.md` — 20-agent audit with competitive landscape
- `docs/architecture/DECISIONS.md` AD-011 — Option A "Ship-and-Hide" (ratified)
- MCP ecosystem research — MCP spec (2025-11-25), 5 meeting vendor MCP surveys, Obsidian MCP repo catalog
- Live pricing pages: talat.app, granola.ai/pricing, meetily.ai/pricing, read.ai/pricing, tldv.io/pricing, vexa.ai/pricing
- HN data: 9 meeting tool Show HN posts analyzed (July 2025 - July 2026)
- GitHub Sponsors data: lazygit (132K stars, 190 sponsors), sindresorhus (487K stars, 189 sponsors)