# Heimdall Distribution & Adoption Strategy: Deep-Dive Research

## Executive Summary

Research across 8 dimensions for heimdall's go-to-market. Key findings: the local-first meeting tool space is **crowded but mostly with GUI apps** — CLI-first is genuinely differentiated. Hacker News Show HN performance for this category is **poor** (most posts get 1-18 points), but the one breakout hit (Trace, 205 points) shows the formula. Talat's TechCrunch coverage provides a replicable PR template. GitHub Sponsors at $500-2000/mo is **achievable but requires 500+ stars AND active community building**. Homebrew distribution is table stakes, not a growth channel.

---

## 1. Homebrew Distribution

### Install Funnel Reality
- **Homebrew has ~8,503 formulae** in homebrew-core (as of July 2026)
- The install funnel: `brew install <formula>` → user already knows your tool name. Homebrew **does not drive discovery** — it reduces friction for users who already found you.
- Getting into homebrew-core requires: stable release, >0 GitHub stars, real users, passing `brew audit --strict`
- **Alternative: custom tap** (`brew tap user/repo`) — lower bar but requires users to add your tap first, adding friction
- Charmbracelet (VHS, Bubbletea, Gum) maintains their own tap: `charmbracelet/homebrew-tap` (40 stars) alongside homebrew-core inclusion

### Meeting/Productivity CLI Tools in Homebrew
Relevant formulae already in homebrew-core:
- **whisper-cpp** — Port of OpenAI's Whisper (1,000s of installs)
- **openai-whisper** — General-purpose speech recognition
- **whisperkit-cli** — Swift on-device speech recognition for Apple Silicon
- **yap** — On-device audio transcription using Speech.framework
- **distill-cli** — AWS Transcribe + Bedrock for audio summaries
- **git-standup** — Standup meeting reports from git history
- **joplin-cli**, **nb**, **zk**, **jrnl**, **dnote**, **jot** — Note-taking CLIs

**Key insight**: No meeting transcription CLI exists in homebrew-core. The closest tools (whisper-cpp, yap) are audio-only — none provide meeting detection, speaker diarization, or summary generation. **Heimdall would be the first end-to-end meeting CLI in Homebrew.**

### Successful CLI Tool Launch Patterns
- **Charmbracelet ecosystem** (VHS: 20.4k stars, Bubbletea: 43.8k stars, Gum: 24.1k stars): Launched via Show HN, maintained both homebrew-core formula AND custom tap, heavy emphasis on visual demos (GIFs in README)
- **lazygit** (80.6k stars): Started with custom homebrew tap, eventually moved to homebrew-core. Commit history shows "Stop updating Jesse's homebrew tap" — they outgrew the custom tap
- **asciinema** (17.6k stars): Has homebrew formula, but growth was driven by the asciinema.org sharing platform, not Homebrew discovery

### What Makes a Tap Discoverable
1. **Being in homebrew-core** (not a custom tap) — this is what users search first
2. **Good formula description** — searchable keywords in the `desc` field
3. **Bottles (pre-compiled binaries)** — users abandon if builds take too long
4. **Listed on formulae.brew.sh** — the public formula index

### Recommendations for Heimdall
- **Phase 1**: Create custom tap `brew tap user/heimdall` for initial releases
- **Phase 2**: Submit to homebrew-core once you have 50+ stars and real users
- Formula description should include: "meeting", "transcription", "local", "privacy"
- Ship bottles from day one (pre-compiled for Apple Silicon)
- **Homebrew is necessary infrastructure, not a growth strategy.** No one discovers tools through Homebrew search.

---

## 2. Obsidian Community

### Launch Strategy in the Obsidian Ecosystem

**Forum structure** (forum.obsidian.md):
- **Share & showcase** category — primary launch venue for plugins/tools
- Popular showcase posts get 40k+ views (e.g., "Setting up Obsidian Git on Windows": 41k views, 16 replies)
- **Plugin releases** tagged `plugin-release` get community attention
- macOS-specific content performs well (46.4k views on "Using Obsidian on macOS" meta post)

### Format That Works
1. **Plugin releases** (tagged `plugin-release`): These are the bread and butter. The Obsidian community plugin directory is the #1 discovery mechanism.
2. **Showcase posts** with detailed tutorials: "Setting up X for Y" format with images/screenshots
3. **Workflow posts**: "How I use Obsidian for Z" — practical workflows get engagement

### Plugin vs Companion Tool
- **Obsidian plugins** (installed via Community Plugins directory): This is the dominant path. The plugin directory has built-in discovery.
- **Companion tools** (external CLIs that export to Obsidian): Harder to gain traction. Must explain why a user should install something outside the plugin ecosystem.
- **Talat does both**: It's a standalone Mac app that auto-exports to Obsidian. TechCrunch explicitly mentions "auto-export to Obsidian" as a feature.

### Community Attitude Toward Paid Tools
- **Obsidian itself** uses freemium: free for personal use, paid sync/publish
- **Community plugins**: Overwhelmingly free and open-source. Paid plugins exist but are controversial.
- **Companion apps**: More acceptance for paid (e.g., Remindersync app at 155 HN points)
- The community values **privacy and local-first** — this aligns perfectly with heimdall's positioning

### Successful Examples
- **Remindersync** (macOS app syncing to Obsidian): 155 points on HN, active forum discussion
- **Obsidian Git plugin**: 41k views on setup guide, core plugin for many users
- **Odin** (LLM + Obsidian integration): 160 HN points, 88 comments
- **Bramses' Highly Opinionated Vault**: 258 HN points — workflow showcase

### Recommendations for Heimdall
- **Do NOT build an Obsidian plugin** — build a companion tool that exports to Obsidian
- Post in Share & showcase with: screenshots/GIF of the Obsidian export workflow
- Title format: "[Tool] Heimdall — Local-first CLI meeting notes that export to Obsidian"
- Emphasize: markdown output, no cloud, works with existing vault structure
- Show a specific vault folder structure recommendation
- The Obsidian angle is your **best community strategy** — this community actively wants local-first tools

---

## 3. GitHub Stars → Users Conversion

### Data Points from Research

**lazygit** (jesseduffield):
- 80,600 GitHub stars
- 190 current GitHub Sponsors (monthly)
- Sponsor goal: 200 monthly sponsors
- Has been actively maintained for 5+ years
- **Stars to active sponsors ratio: ~423:1** (80,600 stars → 190 sponsors)

**Charmbracelet ecosystem** (VHS, Bubbletea, Gum, etc.):
- Combined stars: ~125,000+ across major repos
- Organization has 15.9k followers
- Funded through GitHub Sponsors + commercial offerings
- VHS alone: 20.4k stars, 445 forks

**sindresorhus** (mega-maintainer):
- Maintains 1100+ npm packages, 2 billion downloads/month
- 189 current sponsors, 1,881 past sponsors
- Featured repos: awesome (487k stars), ava (20.8k stars), got (14.9k stars)
- Tiers: $5, $10, $50, $100, $200, $800, $1,000/month

**Hacker News data point** (from Ask HN post):
- One developer reported: 1 main user with 20,000 end users, struggling to get from 10 to 100 GitHub stars
- This suggests **stars severely undercount actual users** for libraries/tools used internally

### Is 50 Stars = 10 Users Realistic?

**Yes, this is conservative.** Here's the reasoning:
- For **CLI tools specifically**, the stars-to-users ratio is **lower** than for libraries (users install and use without starring)
- For **developer tools** consumed via Homebrew/npm, many users never visit the GitHub repo after installing
- **lazygit** with 80,600 stars likely has 100,000+ actual users (Homebrew installs dwarf stars)
- A **more realistic ratio for a CLI tool**: 1 star per 3-5 actual users (once you have distribution)
- At 50 stars with Homebrew distribution: **20-50 actual users** is plausible
- At 50 stars without distribution: **5-15 users** is more realistic

### Key Insight
**Stars are a vanity metric. Install count is what matters.** The funnel is:
- GitHub stars → social proof → more stars → more visibility
- Homebrew/npm installs → actual users → GitHub issues → feature requests
- **50 stars is a credibility threshold, not a user count.** It signals "real project" to browsers of GitHub.

---

## 4. Developer Tool Marketing: Hacker News

### Show HN Performance for CLI/Meeting Tools (2025-2026)

**Meeting transcription tools on HN (recent):**

| Tool | Points | Comments | Date | Angle |
|------|--------|----------|------|-------|
| **Trace** | 205 | 84 | 1 month ago | "non-intrusive, shortcut-driven" + flag mid-call |
| **Summit** | 37 | 10 | 7 months ago | local-first + privacy |
| **Ownscribe** | 18 | 0 | 4 months ago | local transcription, summarization, search |
| **Dictly** | 10 | 2 | 9 months ago | sub-100ms latency, no cloud |
| **Talat** | 6 | 0 | 4 months ago | on-device, built on FluidAudio |
| **Platypus** | 3 | 0 | 3 months ago | Tauri/Rust, Granola alternative |
| **Biscotti** | 1 | 1 | 7 days ago | free, local, native macOS |
| **LokalBot** | 1 | 0 | 20 days ago | meetings + notes + autocomplete |
| **Kumbuka** | 1 | 0 | 7 months ago | Whisper + Claude, broke solo funder |

### What Angle Works on HN

**The 205-point hit (Trace) used these elements:**
1. **Self-deprecating honesty**: "I know, another meeting transcription app. Please bear with me though, I'm confident that this is at least a little novel."
2. **Novel differentiator**: "shortcut-driven" + "flag mid-call" (something no one else does)
3. **Privacy/local-first**: Emphasized on-device processing
4. **Non-intrusive**: "no bot joins your call"

**What flopped (1-6 points):**
- Talat's own Show HN (6 points, 0 comments) — despite TechCrunch coverage
- Most local-first meeting tools get **1-10 points and 0 comments**
- Common failure: posting without a truly novel angle, or posting as pure product launch

**What works for CLI tools generally (top performers):**
- **Sidekick** (452 points): "self-host any app with two commands" — simplicity + self-hosting
- **monolith** (640 points): "save web pages as a single file" — clear single use case
- **Simon Willison's LLM CLI** (529 points): established author + tools ecosystem
- **Nano PDF** (176 points): leveraged new model (Gemini Nano Banana) for novel use

### The Winning Formula for Heimdall on HN
1. **Lead with the CLI angle** — "I run my meetings from the terminal" is novel
2. **Show, don't tell**: asciinema/VHS demo embedded in the post
3. **Privacy as table stakes, not headline** — everyone says "local-first" now
4. **Novel hook needed**: What does heimdall do that Trace/Talat/Biscotti don't?
5. **Timing**: Post Tuesday-Thursday, 8-10 AM ET
6. **Title**: "Show HN: Heimdall – CLI meeting companion for macOS (local, private, no bot)"
7. **First comment**: Detailed technical writeup of how it works

### Recommendations
- **Expect 10-50 points** for a first Show HN, not 200+
- The 205-point Trace hit is an outlier, not the baseline
- **HN is one-shot**: you can't repost. Make it count.
- Better strategy: build audience first (blog posts, Twitter/X), then Show HN when you have a community to upvote
- **Alternative**: Ask HN format ("Ask HN: What do you use for local meeting notes?") then mention heimdall in comments

---

## 5. GitHub Sponsors Viability

### Actual Sponsor Data

**Jesse Duffield (lazygit, lazydocker):**
- 80,600 stars (lazygit) + 52,100 stars (lazydocker) = **132,700 total stars**
- 190 current monthly sponsors
- 605 past sponsors
- Tiers: $5, $15, $25, $100, $250, $500, $1,500, $3,000/month
- Goal: 200 monthly sponsors (85% there)
- **Estimated monthly income**: $2,000-5,000/mo (if average is $10-25/sponsor)

**Sindre Sorhus (1100+ npm packages):**
- 487,000+ stars on awesome alone, massive package ecosystem
- 189 current sponsors, 1,881 past sponsors
- Tiers: $5, $10, $50, $100, $200, $800, $1,000/month
- **Estimated monthly income**: $5,000-15,000/mo (high-tier corporate sponsors)

**Charmbracelet (VHS, Bubbletea, Gum, etc.):**
- 125,000+ combined stars
- Organization sponsor page (not individual)
- Funded through sponsors + commercial Charm platform
- **Estimated: $10,000+/mo** (org-level with corporate sponsors)

### Is $500-2000/mo Realistic for a 500-Star Tool?

**Realistic assessment: $200-800/mo at 500 stars, $500-2000/mo at 1,000-2,000 stars**

Reasoning:
- lazygit at **132,700 stars** has 190 sponsors. Ratio: ~700 stars per sponsor
- At 500 stars, expect **1-3 sponsors** at $5-25/mo = **$5-75/mo**
- At 2,000 stars, expect **5-10 sponsors** = **$50-250/mo**
- To hit $500/mo, you need either:
  - 50+ sponsors at $10/mo average = needs ~5,000+ stars
  - A few corporate sponsors at $100-500/mo (hard to get without enterprise adoption)
- **$2,000/mo requires 10,000+ stars OR corporate sponsors**

### What Tier Structure Works

Based on successful sponsor pages:

**Jesse Duffield's model (developer tools):**
- $5/mo — "Sponsor badge" (entry level)
- $15/mo — mid-tier
- $25/mo — "get me to 200 sponsors"
- $100/mo — serious supporter
- $250-$3,000/mo — corporate tiers

**Sindre Sorhus's model (ecosystem maintainer):**
- $5/mo — badge
- $10/mo — badge
- $50/mo — name on thanks page
- $100-$1,000/mo — company logo placement

### Recommendations for Heimdall
- **Don't expect meaningful Sponsors revenue until 1,000+ stars**
- Set tiers: $5, $10, $25, $100/mo
- **Better monetization path**: paid Pro tier (like Talat's $49-99 one-time) > Sponsors
- Sponsors work for **maintainers with multiple projects**, not single tools
- If goal is $500-2000/mo: **you need a product, not a sponsor page**

---

## 6. Demo Content

### What Makes a Compelling CLI Demo

**Tools and their adoption impact:**

**asciinema** (17.6k stars):
- Terminal session recorder, produces lightweight .cast files
- Embeddable on web pages with asciinema player
- Used by many CLI tools in their READMEs
- **Pro**: text-based, copy-pasteable, tiny file size
- **Con**: requires JavaScript player, not viewable in all contexts

**VHS** (20.4k stars, by Charmbracelet):
- "Your CLI home video recorder" — produces GIFs from .tape files
- **Declarative**: write a .tape script, get a deterministic GIF
- Used by Charmbracelet for all their tools (Gum, Bubbletea, VHS itself)
- **Pro**: GIFs work everywhere (README, Twitter, HN), reproducible
- **Con**: requires ffmpeg, larger files than asciinema

**t-rec** (terminal recorder):
- "Blazingly fast terminal recorder that generates animated gif images"
- Simpler than VHS, just records and outputs GIF

### Examples of Demos That Drove Adoption

**Charmbracelet pattern** (most successful):
- GIF demos in README for every tool (Gum, Bubbletea, VHS)
- Short, focused demos showing one feature per GIF
- Consistent visual style across all tools
- README starts with a GIF, not text

**lazygit pattern**:
- Single demo GIF in README showing core workflow
- Has a `demo/` directory in the repo
- Demo shows the "wow" moment — staging hunks interactively

### What the Heimdall Demo Should Show

**Must-have demo sequence (30-60 seconds total):**

1. **The one-command start**: `heimdall` auto-detects meeting → starts recording
2. **Live transcription appearing**: Both speakers identified, text flowing in real time
3. **Meeting ends → summary generated**: Show the LLM summary with action items
4. **Obsidian export**: Show the markdown file appearing in an Obsidian vault
5. **Search**: `heimdall search "what did we decide about X?"` → instant results

**Format recommendation:**
- **Primary**: VHS-generated GIF for README (works on GitHub, HN, Twitter)
- **Secondary**: asciinema recording for the docs site (copy-pasteable commands)
- **Tertiary**: 60-second Loom/video for landing page (shows real meeting context)

**The demo must answer**: "Why CLI instead of Granola/Talat?" → Show the speed, the scriptability, the integration with existing workflows.

---

## 7. Talat's Launch Strategy

### How Talat Got TechCrunch Coverage

**Full article**: https://techcrunch.com/2026/03/24/talats-ai-meeting-notes-stay-on-your-machine-not-in-the-cloud/

**Key facts:**
- Written by Sarah Perez (Consumer News Editor at TC since 2011)
- Published March 24, 2026, 11:41 AM PDT
- Author: Nick Payne (Yorkshire, England), built with Mike Franklin
- Bootstrapped, no VC

**The launch sequence (reconstructed):**
1. **Built open-source building blocks first**: AudioTee (open source audio library for Core Audio Taps API)
2. **Discovered FluidAudio** (Swift framework for local audio AI) — 2.5k GitHub stars
3. **Assembled product** from these components over ~1 year
4. **Show HN** (6 points — flopped on HN, but the post exists as provenance)
5. **TechCrunch coverage** — likely pitched directly to Sarah Perez

**Why TC covered it (the angle):**
- **Direct contrast with Granola** ($1.5B valuation) — "one developer believes there's demand for a more private, local-only alternative"
- **One-time purchase, no subscription** — anti-SaaS angle
- **20MB app, no account required** — anti-bloat narrative
- **Privacy narrative**: "your audio never leaves your Mac"
- **Bootstrap story**: two-person team, no VC

**Talat's pricing:**
- $49 while in pre-release (pre-1.0)
- $99 at 1.0 release
- Free trial: 10 hours of recordings
- **One-time purchase, not subscription** — explicitly positioned against Granola's subscription model

### Lessons for Heimdall

1. **The "anti-Goliath" angle works**: Talat positioned directly against Granola ($1.5B). Heimdall can position against Granola + Otter + Fireflies (all cloud-based)
2. **Open-source components as PR**: Talat open-sourced AudioTee first, building credibility. Heimdall should open-source core libraries.
3. **One-time purchase > subscription for solo dev tools**: The HN/TC audience hates subscriptions. $49-99 one-time is the sweet spot.
4. **TC coverage ≠ HN success**: Talat flopped on HN (6 points) but got TC. These are different audiences.
5. **The privacy story is now table stakes**: Every new tool says "local-first." You need MORE than privacy.
6. **Bootstrapped narrative resonates**: "Two developers, no VC" is a feature, not a limitation.
7. **Build on existing open-source**: Talat didn't build transcription from scratch — it used FluidAudio (2.5k stars). Heimdall should leverage whisper.cpp / FluidAudio / existing libraries.

### How to Get TC Coverage (playbook):
1. **Build something real and shippable** — not vaporware
2. **Have a contrarian angle**: "everyone else is SaaS, we're one-time purchase" / "everyone else is GUI, we're CLI"
3. **Pitch Sarah Perez** (sarahp@techcrunch.com) — she covers consumer apps, privacy, AI tools
4. **Reference a competitor** by name — TC loves David vs Goliath stories
5. **Have a quotable founder story** — "bootstrapped solo dev" or "built it because I couldn't trust cloud tools with my audio"

---

## 8. Pricing Page Best Practices

### Talat's Pricing Approach
- **No pricing page visible in research** — TC article describes the model
- **Model**: Free trial (10 hours) → $49 (pre-release) → $99 (1.0)
- **One-time purchase**, no tiers, no enterprise
- Simple, clear, anti-subscription

### Meetily
- **Could not access** (meetily.dev did not resolve, GitHub org not found)
- Likely a smaller/no longer active project

### Solo-Developer Open-Core Pricing Best Practices

**Based on the research, here's the optimal structure:**

### Recommended Pricing for Heimdall

**Free (Open Source Core)**
- Unlimited meeting recording + transcription
- Local LLM summarization (Qwen3, Whisper)
- Markdown export
- CLI interface
- Self-hosted, MIT/Apache license

**Pro — $49 one-time (or $5/mo)**
- Obsidian vault integration
- Webhook integrations
- MCP server
- Custom summary templates
- Priority issue support
- **This is the Talat model**: one-time purchase, no subscription fatigue

**Enterprise — Contact**
- Team features (shared meeting database)
- SSO/SCIM
- Custom model deployment
- On-premise support
- SLA

### Why One-Time > Subscription for This Market
1. **Talat proved it**: $49-99 one-time, bootstrapped, TC coverage
2. **HN audience hates subscriptions**: Every Show HN thread has "why is this a subscription?" comments
3. **Developer tools have long lifecycles**: users don't want to pay $10/mo forever for a meeting tool
4. **Open-core with paid features** is more sustainable than pure SaaS for solo devs
5. **GitHub Sponsors** supplements income for the open-source core

### Pricing Page Structure
```
┌─────────────────┬─────────────────┬─────────────────┐
│     FREE        │     PRO         │   ENTERPRISE    │
│  $0 forever     │  $49 one-time   │  Let's talk     │
│                 │                 │                 │
│ ✓ CLI tool      │ Everything in   │ Everything in   │
│ ✓ Local trans   │ Free, plus:     │ Pro, plus:      │
│ ✓ Summaries     │ ✓ Obsidian sync │ ✓ Team sharing  │
│ ✓ Markdown      │ ✓ Webhooks      │ ✓ SSO/SCIM     │
│ ✓ Self-hosted   │ ✓ MCP server    │ ✓ Custom models │
│                 │ ✓ Templates     │ ✓ SLA support   │
│                 │                 │                 │
│  [Download]     │  [Buy Now]      │  [Contact]      │
└─────────────────┴─────────────────┴─────────────────┘
```

### Lifetime Option
- **Offer a lifetime tier** at $99-149: all future Pro features, no renewal
- This is extremely effective for developer tools — removes subscription anxiety
- Creates urgency ("lifetime price increases at 1.0")

---

## Actionable Recommendations Summary

### Phase 1: Pre-Launch (Weeks 1-4)
1. **Create Homebrew tap** — `brew tap user/heimdall` with bottles
2. **Record VHS demo** — 30-second GIF showing full workflow (record → transcribe → summary → Obsidian)
3. **Write the README** — lead with demo GIF, one-command install, "why CLI?" section
4. **Set up GitHub Sponsors** — $5/$10/$25/$100 tiers, but don't expect income yet
5. **Open-source core libraries** — build credibility like Talat did with AudioTee

### Phase 2: Launch (Week 4-6)
1. **Show HN** — Tuesday 8 AM ET, lead with CLI novelty + demo GIF, expect 10-50 points
2. **Obsidian Forum post** — Share & showcase, "CLI meeting notes → Obsidian" workflow
3. **Twitter/X thread** — demo GIF thread, tag @obsdmd, local-first community
4. **r/MacApps, r/commandline** — Reddit posts with demo

### Phase 3: Growth (Weeks 6-12)
1. **Pitch TechCrunch** — Sarah Perez, contrarian angle: "CLI-first meeting tool, one-time purchase, no cloud"
2. **Submit to homebrew-core** — once you hit 50+ stars
3. **Add Pro tier** — $49 one-time for Obsidian integration + webhooks + MCP
4. **Write blog posts** — "Why I built a CLI meeting tool", "Local-first meeting transcription architecture"

### Phase 4: Scale (Months 3-6)
1. **Enterprise tier** — once you have Pro users, add team features
2. **Conference talk** — submit to local-first / privacy / developer tooling conferences
3. **MCP server integration** — position as MCP-compatible for AI agent ecosystem

### Key Metrics to Track
- GitHub stars (target: 100 in month 1, 500 in month 3)
- Homebrew installs (if you can measure via download counts)
- Pro tier purchases (target: 10 in month 1, 50 in month 3)
- Obsidian forum post views (target: 5,000+ in first week)

### The Harsh Truth
- **HN Show HN for meeting tools mostly flops** (1-18 points is normal)
- **GitHub Sponsors won't fund development** until 1,000+ stars
- **Homebrew doesn't drive discovery** — it's infrastructure
- **The differentiator is CLI-first** — every competitor is a GUI app
- **The Obsidian community is the best organic channel** — they want local-first tools
- **One-time purchase > subscription** for this audience
- **PR (TC coverage) is achievable** but requires a contrarian narrative