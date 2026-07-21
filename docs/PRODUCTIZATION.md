# Heimdall — Productization Strategy

**Version**: 1.0
**Created**: 2026-07-21
**Authors:** Ömer Ufuk (synthesized via Hermes)
**Status**: Proposed — awaiting ratification
**Supersedes**: Nothing (complements `docs/STRATEGY_V2.md` which covers engineering + go-to-market but not monetization)

---

## 1. The Thesis

> **Heimdall is the open-source, local-first meeting companion where your audio never leaves your machine unless you choose it. The core is free forever. Value accrues at the integration layer — MCP, team workflows, and managed cloud pipelines.**

This aligns with the owner's productizing preference: **open-source self-hosted core + paid add-ons**, not pure SaaS. The core CLI (`heimdall record`, `analyze`, `recover`, `config`) is MIT-licensed and free. Revenue comes from reducing friction for users who want cloud-grade features without managing their own infrastructure.

---

## 2. Competitive landscape (2026-07)

| Competitor | Model | Pricing | Heimdall's advantage |
|---|---|---|---|
| **Granola** ($1.5B) | Native app, MCP server | $14/mo | CLI-first, Obsidian-native, open-source, provider-swappable |
| **Meetily** (10.8K⭐) | Local Whisper + Ollama, Tauri GUI | Free | CLI-first, MCP-native, Go distribution story, not tied to a GUI |
| **Talat** (Mar 2026) | Core Audio Taps, local LLM, Obsidian export, MCP | Unknown (free beta) | Open-source, provider-agnostic, established ADRs, test coverage |
| **Otter.ai** | SaaS, cloud transcription | $8.33–16.67/mo per seat | Local-first, no account, no data leaves machine |
| **Fireflies.ai** | SaaS, bot-joins-meeting | $10–19/mo per seat | No bot in meeting, CLI composability, Obsidian integration |
| **Apple Notes** (free) | On-device transcription + summary | Free | Turkish diarization, structured output, Obsidian, MCP, CLI |
| **Read.ai** | SaaS, meeting intelligence + MCP | Free tier + paid | Local-first, open-source, own your data |

**The gap no one fills**: CLI-first + Obsidian-native + Turkish diarization + no-bot + open-source + MCP-native. Heimdall owns this intersection.

---

## 3. Productization model: Open Core + Integration Layer

### 3.1 Free core (MIT, forever)

Everything that works today stays free:

| Feature | Status |
|---|---|
| `heimdall record` — full pipeline (capture → transcribe → analyze → Obsidian) | ✅ Free |
| `heimdall doctor` / `config` / `recover` / `analyze` / `list` | ✅ Free |
| Deepgram + Soniox provider support (BYO API key) | ✅ Free |
| Whisper.cpp local STT (Phase 2B — zero API keys) | ✅ Free |
| Ollama local LLM (Phase 3) | ✅ Free |
| Obsidian markdown output with templates | ✅ Free |
| Crash recovery, profiles, multi-language | ✅ Free |
| `heimdall mcp` — MCP server (Phase 2C) | ✅ Free (local, stdio transport) |

**Principle**: if it runs on the user's machine with their own keys, it's free. No feature gating on the core CLI.

### 3.2 Paid tier: Heimdall Cloud (managed infrastructure)

The paid product reduces friction for users who don't want to manage API keys, local models, or infrastructure. Three potential revenue streams:

#### Stream A: Managed MCP Server (SaaS — MCP-as-a-service)

| What | Free | Paid |
|---|---|---|
| `heimdall mcp` (local stdio, reads your vault) | ✅ | ✅ |
| Hosted MCP server (cloud endpoint, 24/7 uptime, team access) | — | $8/mo per user |
| Cross-meeting search across all your meetings (server-side index) | — | ✅ |
| "Ask Heimdall" — natural-language queries over meeting history via MCP | — | ✅ |
| Team-shared meeting library (read-only, permission-scoped) | — | $5/mo per additional seat |

**Why this works**: the local MCP server (Phase 2C) is free, but it only works when your machine is on and heimdall is running. A hosted MCP endpoint means Claude Desktop / Cursor / any agent can query your meetings anytime, anywhere. Teams can share a meeting library without exposing raw audio.

**Why it fits the open-source-core model**: the local `heimdall mcp` command is the open-source reference implementation. The hosted version is the managed deployment of the same code — no feature gating, just infrastructure.

#### Stream B: Managed Cloud Transcription (usage-based)

| What | Free | Paid |
|---|---|---|
| BYO Deepgram/Anthropic key (user pays Deepgram directly) | ✅ | ✅ |
| Heimdall-managed transcription (no API key needed) | — | $0.50/hr (prepaid) or $12/mo (20hr included) |
| Heimdall-managed Claude analysis | — | $0.02/meeting (included in transcription cost) |
| Automatic re-transcription when better models arrive | — | ✅ |

**Why this works**: the current cost barrier ($1.16/hr Deepgram + $0.02 Claude) requires users to sign up for Deepgram + Anthropic accounts, get API keys, configure `.env`. The managed tier removes this: `brew install heimdall && heimdall record --cloud` — zero API keys, zero configuration.

**Pricing logic**: $0.50/hr is 43% of the raw Deepgram cost ($1.16/hr stereo). Heimdall benefits from mono billing ($0.58/hr) + volume discounts + the Soniox fallback (cheaper for some use cases). Margin comes from the spread + volume pricing. Prepaid credits (like Deepgram's $200 credit) avoid SaaS subscription fatigue.

**Why it fits**: the core CLI with BYO keys is free forever. The managed tier is a convenience layer — users can always use their own keys and pay Deepgram directly.

#### Stream C: GitHub Actions — Meeting Notes in CI (marketplace)

| What | Free | Paid |
|---|---|---|
| `heimdall` binary in a GitHub Action (self-hosted runner) | ✅ Free (the action YAML is MIT) | ✅ |
| Hosted runner with heimdall pre-installed (no macOS runner needed) | — | $0.05/min of meeting audio |
| Meeting notes posted to GitHub Discussions / Issues / PR comments | — | ✅ |
| Auto-generate PR summaries from meeting decisions | — | ✅ |
| Link meeting decisions to issue/PR auto-closing | — | ✅ |

**Why this works**: development teams record architecture meetings and want decisions linked to PRs. A GitHub Action that takes a meeting recording (uploaded artifact), runs heimdall, and posts structured notes + action items to the PR/issue closes the "meeting → code" loop.

**Why it fits**: the Action wrapper is open-source. The paid tier is the hosted runner (GitHub's macOS runners cost $0.08/min; heimdall's rate undercuts that and adds meeting-specific value). This is the GitHub Marketplace monetization the owner prefers.

---

## 4. Pricing summary

| Tier | Price | Target user | What you get |
|---|---|---|---|
| **Core** (open-source) | Free | Developers, privacy-first users | Full CLI, BYO keys, local Whisper + Ollama, local MCP |
| **Cloud STT** | $12/mo (20hr) or $0.50/hr | Users who want zero-config | No API keys, managed Deepgram + Claude, auto-retranscription |
| **MCP Hosted** | $8/mo/user | Power users, agent-first workflows | 24/7 MCP endpoint, cross-meeting search, "Ask Heimdall" |
| **Team** | $5/mo/seat (on top of MCP) | Small teams (2-10) | Shared meeting library, permission-scoped access |
| **GitHub Action** | $0.05/min | Dev teams linking meetings to PRs | Hosted runner, PR-summary generation, issue auto-closing |

**Annual discount**: 2 months free on all subscription tiers.

**Free trial**: Cloud STT + MCP Hosted get 10 hours free / 14-day trial, no credit card.

---

## 5. What stays open-source vs. what's proprietary

| Component | License | Why |
|---|---|---|
| `heimdall` Go binary (`cmd/`, `internal/`) | MIT | Core, always free — this is the moat's foundation |
| `heimdall-audio` Swift helper | MIT | Core, always free |
| `templates/` (Obsidian output) | MIT | Core |
| Local Whisper/Ollama adapters | MIT | Core — local-first is the positioning |
| Local MCP server (`heimdall mcp` stdio) | MIT | Core — reference implementation |
| **Hosted MCP server** (cloud deployment) | Proprietary | Infrastructure layer — the managed version |
| **Cloud transcription proxy** (Deepgram key management) | Proprietary | Infrastructure layer — key management + billing |
| **GitHub Action** (the YAML wrapper) | MIT (action YAML) | The action is open; the hosted runner is paid |
| **Cross-meeting search index** (server-side) | Proprietary | Requires server infrastructure |

**The line**: anything that runs on the user's machine = MIT. Anything that runs on heimdall's servers = proprietary. The user can always self-host the open-source equivalent; the paid version is convenience + infrastructure.

---

## 6. Revenue projections (conservative)

| Horizon | Users | Revenue/mo | Source | Assumptions |
|---|---|---|---|---|
| **Month 1** (launch) | 50 free, 0 paid | $0 | All free | STRATEGY_V2 targets 50 stars, 10 users |
| **Month 3** | 100 free, 5 Cloud STT | $60 | Cloud STT $12/mo × 5 | 5% conversion to paid after trial |
| **Month 6** | 200 free, 15 Cloud STT, 3 MCP | $210 | $12×15 + $8×3 | MCP launches month 3; team features month 4 |
| **Month 12** | 500 free, 40 Cloud STT, 10 MCP, 2 team (5 seats) | $590 | $12×40 + $8×10 + $5×10 | GitHub Action launches month 6; team adoption |
| **Year 2** | 2000 free, 100 Cloud STT, 30 MCP, 5 teams (avg 4 seats) | $1,570 | Sustained growth | Kill criteria check at month 6; if passing, continue |

**These are conservative.** The open-source core drives adoption; conversion happens at the integration layer. The 5% free-to-paid conversion is below the SaaS average (3-5% for freemium open-source).

**Break-even**: Deepgram + Anthropic API costs for Cloud STT users. At $0.50/hr selling price and ~$0.30/hr effective cost (mono + volume), margin is $0.20/hr. 20 paid users at 20hr/mo = $80/mo gross margin. Server costs (MCP hosting + Cloud proxy) start at ~$20-50/mo (Railway/Fly.io). **Break-even at ~15-20 Cloud STT users.**

---

## 7. Reversibility analysis (G/Y/R tags)

| Decision | Tag | Rationale |
|---|---|---|
| Open-source core (MIT) | 🟢 GREEN | Trivially reversible — could relicense to AGPL/BSL later if needed |
| Managed Cloud STT (usage-based) | 🟢 GREEN | Can shut down anytime; users fall back to BYO keys |
| Hosted MCP server | 🟡 YELLOW | Requires infrastructure investment; can shut down but users lose their search index |
| GitHub Action (marketplace) | 🟢 GREEN | Can deprecate; the local action YAML still works on self-hosted runners |
| Team features (shared library) | 🔴 RED | One-way door: once teams store meeting history in a shared library, migration is painful. Don't build until MCP is proven. |
| $0.50/hr pricing for Cloud STT | 🟢 GREEN | Adjust anytime; prepaid credits mean no lock-in |
| Annual discount (2 months free) | 🟡 YELLOW | Standard SaaS pattern; hard to reverse once offered |

**Recommendation**: ship Core + Cloud STT + MCP Hosted first. Team features (RED) wait until MCP has 20+ paying users.

---

## 8. Gap analysis (3-7 unasked gaps)

| # | Gap | Why it bites | Trigger | Fail-closed default |
|---|---|---|---|---|
| 1 | **Deepgram ToS reselling** — can heimdall resell Deepgram capacity at a markup? | Deepgram's ToS may prohibit reselling API access. If so, Cloud STT is a Terms violation. | Before launching Cloud STT | Email Deepgram DevRel for written confirmation; default to BYO keys if no answer |
| 2 | **Anthropic ToS reselling** — same for Claude API | Same risk | Before launching Cloud STT | Same — email Anthropic; default to BYO keys |
| 3 | **MCP server auth** — how does a hosted MCP server authenticate users? | MCP stdio is local-only; a remote MCP server needs auth (API key? OAuth? session token?). The MCP spec may not define remote auth yet. | Before launching Hosted MCP | Use API-key-in-header auth initially; follow MCP spec evolution |
| 4 | **Obsidian vault sync latency** — if the user's vault is on iCloud/Dropbox, the hosted MCP server can't read it | The hosted MCP needs vault access, but the vault is local. This breaks the "query anytime" promise. | Before launching Hosted MCP | Require `heimdall sync` (uploads meeting notes to heimdall's index); vault stays local, index is cloud copy |
| 5 | **Apple FoundationModels (fall 2026)** — free on-device STT + summarization | Could commoditize the entire transcription + analysis pipeline, making both the free core and paid Cloud STT irrelevant. | Apple WWDC 2026 announcements | Pivot to MCP + cross-meeting intelligence (the integration layer), which Apple won't build |
| 6 | **Turkish market size** — STRATEGY_V2 positions Turkish as a unique advantage, but how big is the Turkish meeting-notes market? | If too small, the niche doesn't sustain a product. | Before investing in Turkish-specific features | Use Turkish as a differentiator, not the sole market; English-first with Turkish as a premium-quality language |
| 7 | **Self-hosted MCP competition** — if Meetily ships a free MCP server, the hosted MCP loses its value prop | Meetily has 10.8K stars and could ship MCP before heimdall | Monthly competitive check | Differentiate on Obsidian-native + cross-meeting search + team features, not just "we have MCP" |

---

## 9. Productization roadmap (aligned with STRATEGY_V2 phases)

| STRATEGY_V2 Phase | Productization deliverable | Revenue impact |
|---|---|---|
| **2A: Ship & Fix** | Tag v0.1.0, publish to Homebrew, post to communities | $0 (all free) |
| **2B: Local Transcription** | Whisper.cpp support (free core) | $0 (but removes the "needs API keys" barrier → drives adoption) |
| **2C: MCP Server** | Local MCP (free) + **launch Hosted MCP ($8/mo)** | First revenue stream |
| **2D: Launch** | Community posts, README rewrite, demo GIF | Adoption drives the funnel |
| **3: Intelligence** | Cross-meeting memory (free core) + **Cloud STT ($12/mo)** launches | Second revenue stream |
| **4: Platform** | **GitHub Action** ($0.05/min) + **Team features** ($5/mo/seat) | Third + fourth revenue streams |

**Critical path**: MCP Hosted (Stream A) is the first revenue. It requires Phase 2C to ship the local `heimdall mcp` command first. The hosted version is the same server deployed on cloud infrastructure with auth + search index.

---

## 10. Pre-launch checklist (before any revenue)

- [ ] Tag v0.1.0 on GitHub (triggers GoReleaser, creates the first release)
- [ ] Homebrew tap set up (`brew install heimdall`)
- [ ] Real-voice smoke test (STRATEGY_V2 kill-criteria gate)
- [ ] `heimdall mcp` local command shipped (Phase 2C)
- [ ] Deepgram ToS review for reselling (Gap #1)
- [ ] Anthropic ToS review for reselling (Gap #2)
- [ ] MCP remote auth mechanism decided (Gap #3)
- [ ] `heimdall sync` command for vault→cloud index (Gap #4)
- [ ] Privacy policy updated for Cloud STT (audio passes through heimdall's server)
- [ ] Payment infrastructure (Stripe or GitHub Sponsors for managed tiers)

---

## 11. The honest assessment

**What makes this productization viable**:
- The open-source core is genuinely good (A-grade engineering, 270 tests, 4 deps, provider-agnostic)
- MCP-native is a forming category with no dominant player yet
- Obsidian-native is a real moat — no competitor writes wikilink-native markdown
- Local-first is a real positioning (Meetily proves the demand)
- The owner's engineering capacity is the constraint, not market demand

**What makes this hard**:
- Zero users today (F grade on distribution)
- Granola has $1.5B and a native app
- Meetily has 10.8K stars and a GUI
- Apple FoundationModels could commoditize the pipeline in fall 2026
- The owner dislikes pure SaaS — Cloud STT is adjacent to SaaS, but managed infrastructure (not feature-gating) keeps it aligned

**The bet**: MCP + Obsidian-native + CLI-first is a niche big enough for a solo developer to monetize at $500-1,500/mo, but not big enough for a venture-scale company. That's the right size for a personal project productization.

---

## References

- `docs/STRATEGY_V2.md` — engineering roadmap + go-to-market (this document adds the monetization layer)
- `docs/GRILL_REPORT.md` — 20-agent audit with competitive landscape
- `docs/architecture/DECISIONS.md` AD-011 — Option A "Ship-and-Hide" (ratified)
- `README.md` — cost table ($1.16/hr stereo, $0.58/hr mono)