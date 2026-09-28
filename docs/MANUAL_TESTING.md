# Heimdall — Manual Testing Scenarios

**Date**: 2026-03-31
**Purpose**: Step-by-step manual testing to verify each pipeline stage works with real audio.
**Pre-req**: Both binaries built (`make build`). Cloud scenarios need API keys exported; Scenario 0 (fully offline) needs none.

---

## Pre-Flight Checks

Run these before any test scenario:

```bash
# 1. Verify binaries exist
ls -la bin/heimdall bin/heimdall-audio

# 2. Verify API keys are set
echo "Deepgram: ${DEEPGRAM_API_KEY:0:8}..."
echo "Anthropic: ${ANTHROPIC_API_KEY:0:8}..."

# 3. Run doctor
./bin/heimdall doctor
```

**Expected doctor output:**
```
heimdall doctor -- checking prerequisites...

  [pass] macOS 15.x (>= 14.2 required)
  [pass] Deepgram API key configured
  [pass] Anthropic API key configured
  [pass] heimdall-audio helper found
  [pass] Obsidian vault: ~/path/to/vault

5/5 checks passed. Ready to record.
```

---

## Scenario 0: Fully Offline Live Meeting (no API keys, nothing leaves the Mac)

**What it proves**: the whole pipeline -- capture -> local Whisper transcription -> local model analysis -> Obsidian note -- with no network call for the meeting content. This is the acceptance test for `--transcriber whisper` (ID-014) and `--analyzer ollama` (ID-011).

### One-time prerequisites

```bash
brew install whisper-cpp ollama
heimdall model download small       # ~465 MB, the default
heimdall model download medium      # ~1.4 GB; worth trying for Turkish or jargon-heavy
                                    # meetings (the earlier "small vs medium" numbers were
                                    # measured on loop-poisoned transcripts -- ID-018 --
                                    # so the size of the gain is unproven)
ollama serve &                      # or launch the Ollama app
ollama pull qwen3:14b               # ~9.3 GB
heimdall config set claude.analyzer ollama
heimdall config set ollama.base_url http://localhost:11434   # pin it: a previously
                                                             # configured remote host
                                                             # would receive transcripts
heimdall config set obsidian.vault_path ~/path/to/vault
```

**macOS privacy permissions** (System Settings -> Privacy & Security) for the app you run heimdall from -- Terminal, iTerm, or whichever launches it:

| Permission | Why | Without it |
|---|---|---|
| **Microphone** | your own voice | `record` fails or blocks at the permission prompt |
| **Screen & System Audio Recording** | the other participants (Core Audio tap) | `heimdall-audio --check-permissions` prints `denied`; only your mic is recorded |

Verify before the meeting -- this must print `granted`:

```bash
./bin/heimdall-audio --check-permissions && ./bin/heimdall doctor
```

`doctor` should show `[pass] Ollama reachable ... with qwen3:14b` and `[pass] whisper-cli found`. `DEEPGRAM_API_KEY`/`ANTHROPIC_API_KEY` may be unset: this path needs neither.

### 60-second capture preflight (run it before joining)

Two independent checks. A meeting is only worth recording once both pass. (An earlier version of this preflight only asked that "both channel peaks are above 0" -- which the buggy build passed, because both channels were the microphone. ID-017.)

**1. The system-audio tap itself** -- play a sound and read the helper's own summary:

```bash
( sleep 1; afplay /System/Library/Sounds/Sosumi.aiff ) &
sleep 5 | ./bin/heimdall-audio > /dev/null
# last line: audio capture stopped (tap delivered N frames, raw peak P)
```

`P` must be **above 0**. `raw peak 0.0` with a `warning: ... digital silence` line means macOS is giving the tap silence because the app you run heimdall from lacks the "Screen & System Audio Recording" permission -- grant it and restart that app.

**2. The two channels are independent** -- record 20 s while a video plays *and* you speak, then let heimdall judge:

```bash
./bin/heimdall record --transcriber whisper --analyzer ollama --save-audio --title "preflight"
# play speech from a video for ~20 s, say a sentence into the mic, Ctrl+C
./bin/heimdall transcribe --file "$(ls -t ~/.heimdall/recordings/*.wav | head -1)" --model small
```

`transcribe` prints `Warning: the left (system audio) and right (microphone) channels are near-identical ...` if they are copies. **No such warning** is the pass condition. (The same check also runs live during `record`, after about 15 seconds of signal.)

### The meeting

```bash
./bin/heimdall record --transcriber whisper --whisper-model medium \
  --analyzer ollama --title "Weekly sync"
# --whisper-model medium + an explicit --language are the safer choice for Turkish
# or jargon-heavy meetings (ID-018 explains why the earlier evidence is not
# conclusive). For English, small is enough.
# Wear closed headphones: if the mic hears the meeting audio, both channels carry
# the same sound and the speaker split collapses to a single speaker.
```

There is **no live transcript** in this mode -- the terminal says so, and that is expected. Press Ctrl+C when the meeting ends; transcription and analysis then run on the Mac (roughly 2-4 minutes each for a 1-hour meeting).

### Acceptance checklist

- [ ] Terminal shows `STT: whisper (on this machine, after you stop)` and the offline-capture notice
- [ ] Ctrl+C is followed by `Transcribing ... locally via Whisper`, then `Analyzing via Ollama at http://localhost:11434 (on-device -- the transcript does not leave this machine)`. If that label says **REMOTE host**, stop: `ollama.base_url` points somewhere else and the transcript would be sent there
- [ ] `Meeting note saved: <vault>/meetings/<date>/<title>.md`
- [ ] The note's **Action Items** table contains the task, owner, and deadline that were actually spoken
- [ ] **Key Decisions** contains the decision that was actually made, attributed to whoever made it
- [ ] The raw transcript at the end of the note matches what was said (Whisper accuracy)
- [ ] Speakers are split as you (mic) vs. everyone else (system audio) -- per-person names only appear when someone is named out loud
- [ ] `ls ~/.heimdall/recovery/` is empty (the transcript is cleaned up only after a successful note)
- [ ] Turkish meeting: the summary and action items are in Turkish
- [ ] While recording, `nettop`/Little Snitch show no heimdall traffic leaving the machine

### If analysis fails

The meeting is never lost: the note is written with the raw transcript, the transcript file is **kept**, and the terminal prints the exact retry command, e.g.

```bash
heimdall analyze --file ~/.heimdall/recovery/<file>.json --analyzer ollama
```

A transcript longer than the local context window (~1.5 h of English) is refused rather than truncated; re-run that command with `--analyzer api` or `--analyzer codex` if you want it analyzed in the cloud.

---

## Scenario 0b: Live Meeting With Soniox (system audio + real speaker separation)

**What it proves**: the two things the first live test could not: (a) system audio is actually captured -- the helper reads a real process tap (ID-017), and (b) Soniox separates the people in the meeting by voice (AD-012). This is the acceptance test for both.

### Prerequisites

```bash
export SONIOX_API_KEY=your_key          # https://soniox.com/ -- never commit it
heimdall config set claude.analyzer ollama      # or claude-code / codex / api
heimdall config set obsidian.vault_path ~/path/to/vault
make build                               # rebuilds the Swift helper too
```

Run the **60-second capture preflight** from Scenario 0 first (both checks). Then put **closed headphones on** for the meeting: if your mic can hear the remote voices, the two channels overlap and a meeting-long duplicate-channel warning can be a false alarm from your own room.

### The meeting

```bash
./bin/heimdall record --title "Soniox live test" --analyzer ollama --save-audio
# the header must say: Transcriber: soniox (auto: SONIOX_API_KEY is set)
```

Use a real meeting with at least **two other people** speaking (or play a two-voice recording from a second device), and talk yourself a few times. Ctrl+C at the end.

### Acceptance checklist

- [ ] Header shows `Transcriber: soniox (auto: SONIOX_API_KEY is set)` and the status line reads `STT: soniox (connected)`
- [ ] Live transcript appears while the meeting runs (Soniox is a streaming provider, unlike `--transcriber whisper`)
- [ ] **No** `WARNING: the left (system audio) and right (microphone) channels are near-identical` appears at any point
- [ ] The live display shows **more than one speaker number** for the remote people, and your own voice is consistently a different one from theirs
- [ ] Turkish (or mixed TR/EN) speech is transcribed in the right language without passing `--language` (the Soniox default here is `multi`)
- [ ] The note's Action Items and Key Decisions match what was said; speaker names appear only where someone was named out loud
- [ ] Afterwards: `./bin/heimdall transcribe --file "$(ls -t ~/.heimdall/recordings/*.wav | head -1)" --model medium --language tr` prints **no** duplicate-channel warning, and the transcript has no line repeated more than a couple of times in a row
- [ ] If Soniox fails mid-meeting: the WAV under `~/.heimdall/recordings/` still holds the whole meeting and `heimdall transcribe --file` recovers it

### If something is off

| Symptom | Meaning |
|---|---|
| `raw peak 0.0` in the tap preflight | Screen & System Audio Recording permission missing for your terminal app |
| duplicate-channel warning during the meeting | the helper is not reading the tap (re-run the preflight), or the same signal reaches both channels through your setup |
| `unknown or retired model` | `soniox.model` is set to a retired model; use `stt-rt-v5` |
| auth error from Soniox | wrong or expired `SONIOX_API_KEY`; this is the first live use of the Soniox adapter (AD-012), so report exactly what the terminal prints |

---

## Scenario 1: Microphone-Only Recording (Isolate Mic Path)

**Goal**: Verify the microphone → mixer → Deepgram → transcript path works without system audio complexity.

### Steps

```bash
# Record for 30 seconds, speak clearly into your mic
./bin/heimdall record --title "Mic Test"
# Wait 5 seconds, then say: "Hello, this is a microphone test. Today is Monday."
# Wait 5 more seconds, then press Ctrl+C
```

### Expected Output

```
heimdall 0.1.0-dev -- recording "Mic Test"
Audio: system off  mic on  | STT: deepgram (connected) | recording...

[00:00:05] Speaker 0: Hello, this is a microphone test.
[00:00:08] Speaker 0: Today is Monday.

Stopping recording...
------------------------------------------------------------
Meeting recorded: 00:00:15 | 2 segments | 1 speakers
------------------------------------------------------------
```

### What Could Go Wrong

| Symptom | Likely Cause | Bug # |
|---------|-------------|-------|
| `failed to start recording: microphone start failed` | macOS mic permission denied | — (handled correctly) |
| Deepgram connected but **no transcript lines appear** | **BUG #1**: `diarize=true` + `multichannel=true` conflict | #1 |
| `transcriber connect failed: websocket: bad handshake` | Invalid API key or Deepgram URL params rejected | #2 |
| Transcript appears but all timestamps are `[00:00:00]` | Mixer elapsed not advancing for skipped frames | #16 |

### Verification Checklist
- [ ] Transcript lines appear within 2-3 seconds of speaking
- [ ] Speaker ID is shown (`Speaker 0`)
- [ ] Timestamps advance (not all `00:00:00`)
- [ ] Ctrl+C stops cleanly (not hanging for 5+ seconds — see Bug #13)
- [ ] Summary line shows correct segment count

---

## Scenario 2: System Audio Capture (Isolate System Path)

**Goal**: Verify the Swift helper captures system audio (e.g., YouTube playing in browser).

### Steps

```bash
# 1. Open a YouTube video in your browser with speech (a podcast or news clip)
# 2. Start recording
./bin/heimdall record --title "System Audio Test"
# 3. Let the video play for 15-20 seconds
# 4. Press Ctrl+C
```

### Expected Output

```
heimdall 0.1.0-dev -- recording "System Audio Test"
Audio: system on  mic on  | STT: deepgram (connected) | recording...

[00:00:03] Speaker 0: [transcript of what the YouTube video is saying]
[00:00:08] Speaker 0: [more transcript from the video]
```

### What Could Go Wrong

| Symptom | Likely Cause | Bug # |
|---------|-------------|-------|
| `Audio: system off` with no error message | Screen Recording permission denied — error swallowed silently | **#4** |
| System audio captures but transcript is garbled/wrong | Swift buffer size mismatch (4096 vs 960 frame reads) | #15 |
| System audio captures but NO transcript at all | BUG #1 (diarize+multichannel) kills all output | **#1** |
| Audio: system on but recording produces only mic transcript | System audio samples arriving as silence due to timing | #15, #16 |

### How to Grant Screen Recording Permission
1. System Settings → Privacy & Security → Screen Recording
2. Find `heimdall-audio` (or Terminal if running from terminal)
3. Toggle ON
4. **Restart the terminal** (macOS caches permissions)

---

## Scenario 3: Dual-Channel Meeting Simulation

**Goal**: Simulate a real meeting with both system audio (remote participant) and mic (you).

### Steps

```bash
# 1. Open a YouTube video with someone talking (simulates remote participant)
# 2. Start recording
./bin/heimdall record --title "Meeting Simulation" --participants "Alice,Bob"
# 3. Let the video play for 10 seconds (system audio = Alice)
# 4. Then speak into your mic: "I think we should proceed with option B" (you = Bob)
# 5. Let the video play for 10 more seconds
# 6. Press Ctrl+C
```

### Expected Output (After Bug #1 is Fixed)

```
heimdall 0.1.0-dev -- recording "Meeting Simulation"
Audio: system on  mic on  | STT: deepgram (connected) | recording...

[00:00:03] Speaker 0: [YouTube video speech - left channel/system]
[00:00:08] Speaker 0: [more YouTube speech]
[00:00:12] Speaker 1: I think we should proceed with option B
[00:00:18] Speaker 0: [YouTube video continues]

Stopping recording...
------------------------------------------------------------
Meeting recorded: 00:00:25 | 4 segments | 2 speakers
------------------------------------------------------------
Generating meeting summary via Claude...
Meeting note saved: ~/Vault/meetings/2026-03-31/meeting-simulation.md
```

### Key Verifications
- [ ] System audio (YouTube) appears as Speaker 0 (channel 0 / left)
- [ ] Mic audio (your voice) appears as Speaker 1 (channel 1 / right)
- [ ] Claude analysis runs and produces summary
- [ ] Obsidian note is written with correct YAML frontmatter
- [ ] Recovery file is cleaned up after successful write (check `ls ~/.heimdall/recovery/`)

---

## Scenario 4: Turkish Language Meeting

**Goal**: Verify Turkish transcription works.

### Steps

```bash
# Record in Turkish
./bin/heimdall record --title "Turkce Toplanti" --language tr \
  --keywords "sprint,deploy,API,backlog,refactor"
# Speak in Turkish for 15 seconds
# Press Ctrl+C
```

### Expected Output

```
[00:00:03] Speaker 0: [Turkish speech transcribed]
```

### What Could Go Wrong

| Symptom | Likely Cause | Bug # |
|---------|-------------|-------|
| Filename is `turkce-toplanti.md` (missing ç,ö,ü) | Regex strips Turkish chars | **#22** |
| Claude summary is in English despite Turkish transcript | `AnalyzeOpts.Language` never wired | **#23** |
| Transcript quality very poor for mixed TR+EN | Deepgram `multi` doesn't include Turkish | **#6** |

---

## Scenario 5: Crash Recovery

**Goal**: Verify recovery works after a forced kill.

### Steps

```bash
# 1. Start a recording
./bin/heimdall record --title "Recovery Test"
# 2. Speak for 30+ seconds (recovery writes every 30s)
# 3. Force kill the process (NOT Ctrl+C)
kill -9 $(pgrep heimdall)

# 4. Check recovery files exist
ls -la ~/.heimdall/recovery/

# 5. Recover
./bin/heimdall recover
```

### Expected Output

```
Found 1 recovery file(s):
  Recovery Test (2026-03-31 12:00:00) - 5 segments
Re-analyzing via Claude...
Meeting note saved: ~/Vault/meetings/2026-03-31/recovery-test.md
```

### What Could Go Wrong

| Symptom | Likely Cause | Bug # |
|---------|-------------|-------|
| Recovery file found but also exists after SUCCESSFUL recording | `defer Stop()` recreates file after `Cleanup()` | **#5** |
| Running `recover` twice creates duplicate notes | Processed files never deleted | **#26** |
| `recover` fails with no useful error | Config parse error swallowed | — |

---

## Scenario 6: Long Meeting (55+ Minutes)

**Goal**: Verify proactive reconnection at 55 minutes works.

### Steps

```bash
# This is a long test — record for 56+ minutes
./bin/heimdall record --title "Long Meeting Test"
# Let it run past the 55-minute mark
# Watch for reconnection log messages
# Then Ctrl+C
```

### What Could Go Wrong

| Symptom | Likely Cause | Bug # |
|---------|-------------|-------|
| Process hangs on Ctrl+C after 55 min | `Close()` deadlock — untracked goroutine | **#14** |
| Transcript gaps during reconnection | Ring buffer dead code, audio dropped | **#10** |
| keepAlive stops after reconnection | keepAliveLoop not restarted | **#11** |
| Connection dies shortly after reconnect | No keepAlive → Deepgram idle timeout | **#11** |

---

## Scenario 7: Config and Doctor Validation

**Goal**: Verify config and doctor commands work correctly.

### Steps

```bash
# Initialize config
./bin/heimdall config init

# Check individual values
./bin/heimdall config get deepgram.model
./bin/heimdall config get obsidian.vault_path

# Set a value
./bin/heimdall config set claude.model claude-sonnet-4-6

# List meetings
./bin/heimdall list
./bin/heimdall list --since 2026-03-01

# Check version
./bin/heimdall version
```

### What Could Go Wrong

| Symptom | Likely Cause | Bug # |
|---------|-------------|-------|
| `config get` shows `${DEEPGRAM_API_KEY}` literally | ResolveEnvVars never called | **#29** |
| `config set deepgram.api_key sk-real-key` stores plaintext | No validation despite docs saying "never plaintext" | **#27** |
| `list --since "last week"` shows "No meetings" silently | No date format validation | **#28** |
| Doctor passes on macOS 13 | Version check is cosmetic | **#24** |

---

## Post-Fix Verification Matrix

After fixing all 31 bugs, re-run scenarios 1-4 and verify:

| Check | Scenario | Expected After Fix |
|-------|----------|--------------------|
| Transcript appears with real audio | 1, 2, 3 | Yes — diarize removed, multichannel-only |
| Deepgram errors visible in terminal | 1 | `log.Printf` for non-Results messages |
| Screen Recording denied → clear error | 2 | Terminal shows "Screen Recording permission required" |
| Config env vars resolved | 7 | `config get` shows resolved values |
| Doctor validates macOS ≥14.2 | 7 | Fails on macOS 13.x |
| Recovery file absent after clean exit | 5 | `ls ~/.heimdall/recovery/` is empty |
| Turkish filenames preserve characters | 4 | `turkce-toplanti.md` with proper chars |
| Claude output in meeting language | 4 | Turkish summary for Turkish meetings |
| Ctrl+C stops in <2 seconds | 1, 2, 3 | No 5-second delay |
| 55-min reconnect works | 6 | No hang, no gaps |

---

## Quick Smoke Test (2 minutes)

The fastest way to verify heimdall works end-to-end:

```bash
# Build
make build

# Export keys
export DEEPGRAM_API_KEY=your_key
export ANTHROPIC_API_KEY=your_key

# Record for 15 seconds while speaking
./bin/heimdall record --title "Smoke Test"
# Speak: "Testing one two three. The quick brown fox jumps over the lazy dog."
# Press Ctrl+C after 15 seconds

# Verify:
# 1. Did transcript lines appear? (YES = Deepgram works)
# 2. Did Claude analysis run? (YES = Anthropic works)
# 3. Was an Obsidian note created? (YES = output works)
# 4. Is ~/.heimdall/recovery/ empty? (YES = cleanup works)
```

**If Step 1 fails (no transcript)**: Bug #1 is your blocker. Fix `diarize=true` → remove it.
