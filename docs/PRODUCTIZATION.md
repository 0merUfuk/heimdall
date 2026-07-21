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

> Full research with live pricing-page sources: `docs/MONETIZATION_RESEARCH.md`

| Competitor | Model | Pricing | Local-first | Heimdall's advantage |
|---|---|---|---|---|
| **Granola** ($1.5B) | Native macOS app, MCP server | Free / $14/mo / $35/mo | ❌ Cloud audio | CLI-first, open-source, provider-swappable |
| **Meetily** (25.8K⭐) | Local Whisper + Ollama, Tauri GUI | Free CE / $10/mo PRO | ✅ | CLI-first, Go distribution, MCP-native, not tied to a GUI |
| **Talat** (Mar 2026) | Core Audio Taps, local LLM, Obsidian export, MCP | 10hr free / $9/mo / **$189 lifetime** | ✅ On-device | Open-source, provider-agnostic, established ADRs, test coverage |
| **Vexa** (2.6K⭐) | Open-source + hosted SaaS | Free self-host / $12/mo / $0.30-0.50/hr | ✅ Self-host | CLI-first, Obsidian-native, no bot required |
| **Otter.ai** | SaaS, cloud transcription | $6.67–20/mo per seat | ❌ Cloud | Local-first, no account, no data leaves machine |
| **Fireflies.ai** | SaaS, bot-joins-meeting | $10–39/mo per seat | ❌ Cloud | No bot in meeting, CLI composability, Obsidian integration |
| **Read.ai** | SaaS, meeting intelligence + MCP | $15–39.75/mo per seat | ❌ Cloud | Local-first, open-source, own your data |
| **tl;dv** | SaaS, bot + MCP server | $18–29/mo per seat | ❌ Cloud | No bot, local-first, open-source |
| **Apple Notes** (free) | On-device transcription + summary | Free | ✅ | Turkish diarization, structured output, Obsidian, MCP, CLI |

**Key pricing insight from research**: local-first tools are NOT priced at a premium — they're priced **at or below** cloud competitors ($9-10/mo vs $10-18/mo). Privacy is a **competitive differentiator at the same price**, not a premium feature. Talat's $189 lifetime license is explicitly cited as a purchase reason ("own your tools, like software used to work").

**MCP market finding**: no one charges for MCP servers as a standalone product. MCP is a feature included in paid tiers (tl;dv, Read.ai gate it higher; Talat includes it in all paid tiers). Do NOT try to charge for MCP separately — use it as a stickiness driver.

**GitHub Actions marketplace finding**: the meeting/transcription category on GitHub Marketplace is **empty** (0 results for "meeting transcription"). Actions are designed for CI/CD, not desktop apps. GitHub Sponsors is the most common monetization for popular CLI tools (lazygit, act) but revenue is modest (<$1K/mo for most).

**The gap no one fills**: CLI-first + Obsidian-native + Turkish diarization + no-bot + open-source + MCP-native + lifetime license option. Heimdall owns this intersection.

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

### 3.2 Paid tier: Heimdall Pro (open-core + lifetime license)

Based on the competitive research, the proven model for local-first meeting tools is **Meetily's open-core** (free MIT CE + paid PRO) + **Talat's lifetime license** (users explicitly cite "own your tools" as a purchase reason). Price at or below cloud competitors ($9-14/mo range).

#### Heimdall Community Edition (Free, MIT — everything that works today)

| Feature | Status |
|---|---|
| `heimdall record` — full pipeline (capture → transcribe → analyze → Obsidian) | ✅ Free |
| `heimdall doctor` / `config` / `recover` / `analyze` / `list` | ✅ Free |
| Deepgram + Soniox provider support (BYO API key) | ✅ Free |
| Whisper.cpp local STT (Phase 2B — zero API keys) | ✅ Free |
| Ollama local LLM (Phase 3) | ✅ Free |
| Obsidian markdown output with templates | ✅ Free |
| Crash recovery, profiles, multi-language | ✅ Free |
| `heimdall mcp` — MCP server (local stdio, basic tools: search/get/list) | ✅ Free |
| `heimdall search`, `heimdall list --json`, `heimdall export` | ✅ Free |

**Principle**: the full local CLI is free forever. No feature gating on anything that runs on the user's machine with their own keys. MCP is free — it drives adoption and stickiness (Talat includes it in all tiers; no one charges for MCP separately).

#### Heimdall Pro ($9/mo or $99 lifetime)

| Feature | Free CE | Pro |
|---|---|---|
| Everything in CE | ✅ | ✅ |
| **Enhanced transcription models** (optimized multi-language, better Turkish) | — | ✅ |
| **Custom meeting templates** (standup, 1:1, planning, retro) | — | ✅ |
| **Cross-meeting intelligence** (feed last N meeting summaries as context) | — | ✅ |
| **Advanced MCP tools** (get_action_items, get_decisions, vault-as-memory) | — | ✅ |
| **People pages** (auto-generated `[[Person Name]]` pages with meeting history) | — | ✅ |
| **Daily note integration** (auto-append meeting summaries to today's note) | — | ✅ |
| **Calendar integration** (iCal auto-populate participants) | — | ✅ |
| **Priority support** (GitHub issue priority, faster response) | — | ✅ |
| **`heimdall upgrade --pro`** (license key activation, same binary) | — | ✅ |

**Pricing logic**:
- $9/mo matches Talat (the closest competitor) and undercuts Granola ($14/mo)
- $99 lifetime undercuts Talat's $189 lifetime by ~48% — aggressive but captures the "own your tools" crowd
- Pro is the same binary with license-key activation (like Meetily PRO) — no separate download
- No SaaS infrastructure needed for the core Pro tier — features unlock locally

#### Heimdall Cloud (optional add-on, managed infrastructure)

For users who want cloud-grade features without managing infrastructure:

| What | Price | What it is |
|---|---|---|
| **Managed MCP endpoint** | $5/mo add-on to Pro | 24/7 hosted MCP server (same code, cloud deployment) so Claude Desktop/Cursor can query meetings when your machine is off |
| **Cloud transcription** | $0.50/hr prepaid | No API keys needed — `heimdall record --cloud`. Uses heimdall's Deepgram volume pricing. Users can always fall back to BYO keys for free. |

Cloud is an **add-on to Pro**, not a replacement. Users who want everything local pay $99 lifetime and never touch cloud. Users who want convenience add cloud on top.

---

## 4. Pricing summary

| Tier | Price | Target user | What you get |
|---|---|---|---|
| **Community Edition** (open-source, MIT) | Free | Developers, privacy-first users | Full CLI, BYO keys, local Whisper + Ollama, local MCP, Obsidian output |
| **Pro** | $9/mo or **$99 lifetime** | Power users, Obsidian-centric workflows | Enhanced models, custom templates, cross-meeting intelligence, advanced MCP tools, people pages, daily note integration, calendar, priority support |
| **Cloud MCP** (add-on to Pro) | +$5/mo | Agent-first workflows, always-on query | 24/7 hosted MCP endpoint, cross-meeting search index |
| **Cloud STT** (add-on to Pro) | $0.50/hr prepaid | Zero-config users | No API keys needed, managed Deepgram + Claude |
| **Enterprise** | Custom | Teams (5+) | Self-hosted team deployment, audit trails, volume licensing |

**Lifetime license**: $99 one-time — all current + future Pro features, no subscription. This is the key differentiator: Talat charges $189, Meetily has no lifetime option. Heimdall offers the lowest lifetime price in the local-first meeting tools market.

**Annual discount**: 2 months free on the monthly subscription ($90/yr vs $108/yr).

**Free trial**: 10 hours free (matches Talat's model — full features, no credit card, no time-bombed features).

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

Based on Meetily's trajectory (25.8K stars in ~8 months) and Talat's conversion data:

| Horizon | Users | Revenue/mo | Source | Assumptions |
|---|---|---|---|---|
| **Month 1** (launch) | 50 free, 0 paid | $0 | All free | STRATEGY_V2 targets 50 stars, 10 users |
| **Month 3** | 100 free, 3 Pro (lifetime) | $0 (lifetime = one-time) / $27/mo (subscription) | Pro $9/mo × 3 or $99 × 3 lifetime | 3% conversion (conservative for OSS); Pro features shipped (templates, cross-meeting) |
| **Month 6** | 200 free, 10 Pro, 2 Cloud MCP | $100/mo + $198 lifetime | $9×10 + $5×2 (add-on); or $99×10 lifetime | MCP server shipped; local-first value proven |
| **Month 12** | 500 free, 30 Pro, 5 Cloud MCP, 2 Cloud STT | $290/mo + $2,970 lifetime | $9×30 + $5×5 + $0.50×20hr×2; or $99×30 lifetime | Whisper local mode shipped; Obsidian community posted |
| **Year 2** | 2000 free, 100 Pro, 15 Cloud MCP, 5 Cloud STT | $1,015/mo + $9,900 lifetime | Sustained growth | Kill criteria passed; enterprise interest |

**Mix assumption**: 60% lifetime, 40% subscription (based on Talat's experience that lifetime converts privacy-conscious users faster).

**Break-even**: Pro features are local — no server costs. The only infrastructure cost is Cloud MCP + Cloud STT hosting (~$20-50/mo on Railway/Fly.io). **Break-even at ~5 Pro users** (lifetime covers it) or ~3 Pro monthly + 1 Cloud add-on.

**Revenue ceiling for solo developer**: $1,000-2,000/mo at 100+ Pro users. This is supplementary income, not a company — which is the right scale for a personal project.

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
| **2A: Ship & Fix** | Tag v0.1.0, publish to Homebrew, post to communities | $0 (all free CE) |
| **2B: Local Transcription** | Whisper.cpp support (free CE) — removes "needs API keys" barrier | $0 (drives adoption — the funnel top) |
| **2C: MCP Server** | Local MCP shipped (free CE, basic tools) + **Pro features built** (advanced MCP tools, cross-meeting, templates) | **First revenue**: `heimdall upgrade --pro` available |
| **2D: Launch** | Community posts, README rewrite, demo GIF, **Pro landing page** | Adoption drives the conversion funnel |
| **3: Intelligence** | Cross-meeting memory (Pro), people pages (Pro), daily note integration (Pro), Ollama local LLM (free CE) | Pro tier matures |
| **4: Platform** | **Cloud MCP** ($5/mo add-on) + **Cloud STT** ($0.50/hr add-on) + Enterprise inquiries | Cloud add-ons launch |

**Critical path**: Pro features (templates, cross-meeting intelligence, advanced MCP tools) are the first revenue. They require Phase 2C (MCP server) + Phase 3 (cross-meeting) to ship first. The $99 lifetime price is available from day one of Pro — early adopters get the best deal.

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