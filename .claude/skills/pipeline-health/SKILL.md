---
description: >
  Heimdall-specific pipeline health check. Tests all 6 pipeline stages
  individually and as a system. Verifies audio capture, mixer, transcriber,
  analyzer, renderer, and recovery subsystems are functional.
argument-hint: "[--stage <1-6>]"
allowed-tools: Read, Grep, Glob, Bash, Agent
---

# Pipeline Health Check

## When to Use
- Before a release
- After modifying any pipeline stage
- When audio capture or transcription stops working
- As part of /strategy-weekly

## Arguments
- `--stage N`: check only stage N (1=capture, 2=mix, 3=transcribe, 4=accumulate, 5=analyze, 6=render)
- No argument: check all 6 stages

## Execution

### Stage 1: Capture
- Verify `internal/audio/microphone.go` compiles and tests pass
- Verify `internal/audio/system.go` compiles and tests pass
- Check `bin/heimdall-audio` exists (Swift helper)
- Run `cd audio-helper && swift build` if source exists

### Stage 2: Mix
- Verify `internal/mixer/` compiles and tests pass
- Check resample.go: 48kHz->16kHz ratio is 3:1
- Check ring_buffer.go: overflow drops oldest

### Stage 3: Transcribe
- Verify `internal/transcriber/` compiles and tests pass
- Check deepgram.go: V-001 reconnection implemented
- Check deepgram.go: V-005 retry logic implemented

### Stage 4: Accumulate
- Verify `internal/session/` compiles and tests pass
- Check session.go: segments accumulated correctly
- Check session.go: OnSegment callback fires

### Stage 5: Analyze
- Verify `internal/analyzer/` compiles and tests pass
- Check claude.go: V-009 retry + fallback implemented
- Check prompts.go: V-013 anti-hallucination present
- Check prompts.go: V-014 injection mitigation present

### Stage 6: Render
- Verify `internal/output/` compiles and tests pass
- Check renderer.go: V-016 vault validation
- Check renderer.go: V-017 collision prevention
- Check templates/: meeting-note.md.tmpl embedded

### Cross-Stage
- `go build ./...` passes
- `go test ./... -race -count=1` all green
- `go vet ./...` clean
- No import cycles between stages

### Report

```
Pipeline Health -- {date}

Stage 1 (Capture):    [PASS/FAIL] {details}
Stage 2 (Mix):        [PASS/FAIL] {details}
Stage 3 (Transcribe): [PASS/FAIL] {details}
Stage 4 (Accumulate): [PASS/FAIL] {details}
Stage 5 (Analyze):    [PASS/FAIL] {details}
Stage 6 (Render):     [PASS/FAIL] {details}

Cross-Stage: [PASS/FAIL]
Overall: N/6 stages healthy
```
