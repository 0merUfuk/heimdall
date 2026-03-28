**Version**: 1.0
**Created**: 2026-03-28
**Last Updated**: 2026-03-28
**Authors:** Omer Ufuk

---

# Audio Safety Rules

> Auto-loaded when working in heimdall. These rules prevent audio pipeline bugs that cause data loss, deadlocks, or corruption.

---

## Channel Safety

- **Never block on audio channels** — always use `select` with timeout or context cancellation
- Ring buffer overflow → **drop oldest frames**, never block the producer goroutine
- Audio goroutines must have `defer cleanup()` and respond to `ctx.Done()`
- All `AudioSource` implementations must be safe to call `Stop()` multiple times (idempotent)

```go
// CORRECT — non-blocking send with timeout
select {
case frameCh <- frame:
case <-ctx.Done():
    return
case <-time.After(100 * time.Millisecond):
    // Frame dropped — log at debug level, never block
}

// WRONG — blocking send that can deadlock the pipeline
frameCh <- frame
```

---

## Goroutine Lifecycle

- Every goroutine that processes audio must accept a `context.Context`
- Goroutines must exit cleanly when context is cancelled
- Use `errgroup` or `sync.WaitGroup` for coordinating pipeline stage goroutines
- On shutdown: drain channels before closing them to prevent panics on closed channels

```go
// CORRECT — drain before close
close(stopCh)
for range frameCh {} // drain remaining frames
close(frameCh)

// WRONG — close while producer may still be sending
close(frameCh) // panic if producer sends after close
```

---

## Swift Subprocess Safety (V-002)

- Monitor subprocess health with a watchdog goroutine
- Detect crash within **2 seconds** (check process.Wait())
- Auto-restart with exponential backoff (max 3 retries, then fail)
- Track audio gap during restart (report gap duration to user)
- Send "stop" to stdin for graceful shutdown — never SIGKILL unless unresponsive for 5s

---

## Sample Rate and Format

- Microphone: 16kHz, 16-bit signed int, mono
- System audio (from Swift): 48kHz, 32-bit float, stereo → resample to 16kHz, 16-bit int
- Deepgram expects: 16kHz, 16-bit linear PCM, 2 channels (L=system, R=mic)
- Use integer-ratio fast path when resampling 48→16 (ratio 3:1, take every 3rd sample)

---

## Temp File Safety (V-006)

- Write crash recovery files atomically: temp file → `os.Rename()` (never partial writes)
- Recovery file permissions: `0600` (owner read/write only)
- Recovery directory: `~/.heimdall/recovery/`
- Write interval: every 30 seconds during recording
- On clean shutdown: delete recovery file after successful Obsidian write
