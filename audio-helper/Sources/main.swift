// heimdall-audio — System audio capture via Core Audio Taps
//
// Captures system audio and outputs raw PCM to stdout.
// Format: 48kHz, 32-bit float, stereo (interleaved).
//
// Protocol:
//   stdout  -> raw PCM audio bytes (no header, no framing)
//   stderr  -> status/error messages
//   stdin   -> "stop\n" triggers graceful shutdown
//   exit 0  -> clean exit
//   exit 1  -> fatal error (macOS too old, no audio device, etc.)
//   exit 77 -> Screen Recording permission denied

import AVFAudio
import CoreAudio
import CoreGraphics
import Darwin
import Foundation

// MARK: - Constants

/// Target audio format: 48kHz, 32-bit float, stereo (interleaved).
/// The Go side (subtask 0.6/0.7) handles resampling to 16kHz and conversion to 16-bit int.
private let kSampleRate: Double = 48000.0
private let kChannelCount: UInt32 = 2
private let kBufferSize: AVAudioFrameCount = 4096

// MARK: - Exit Codes

private enum ExitCode: Int32 {
    case success = 0
    case fatalError = 1
    case permissionDenied = 77
}

// MARK: - Stderr Logging

/// Write a message to stderr. Stdout is reserved for audio data.
private func logError(_ message: String) {
    var msg = "heimdall-audio: \(message)\n"
    msg.withUTF8 { buffer in
        _ = fwrite(buffer.baseAddress, 1, buffer.count, stderr)
        fflush(stderr)
    }
}

// MARK: - Shutdown Coordination

/// Thread-safe shutdown flag. When set, the audio engine stops and the process exits cleanly.
private let shutdownRequested = DispatchSemaphore(value: 0)
private var isShuttingDown = false
private let shutdownLock = NSLock()

private func requestShutdown() {
    shutdownLock.lock()
    defer { shutdownLock.unlock() }
    if !isShuttingDown {
        isShuttingDown = true
        shutdownRequested.signal()
    }
}

private func shouldShutdown() -> Bool {
    shutdownLock.lock()
    defer { shutdownLock.unlock() }
    return isShuttingDown
}

// MARK: - Signal Handling

/// Install SIGTERM and SIGINT handlers for graceful shutdown.
private func installSignalHandlers() {
    let sigTermSource = DispatchSource.makeSignalSource(signal: SIGTERM, queue: .global())
    sigTermSource.setEventHandler {
        logError("received SIGTERM, shutting down")
        requestShutdown()
    }
    sigTermSource.resume()

    // Ignore default signal handling so our dispatch sources work.
    signal(SIGTERM, SIG_IGN)

    let sigIntSource = DispatchSource.makeSignalSource(signal: SIGINT, queue: .global())
    sigIntSource.setEventHandler {
        logError("received SIGINT, shutting down")
        requestShutdown()
    }
    sigIntSource.resume()
    signal(SIGINT, SIG_IGN)

    // Keep sources alive for the lifetime of the process.
    _signalSources = [sigTermSource, sigIntSource]
}

/// Prevent signal dispatch sources from being deallocated.
private var _signalSources: [Any] = []

// MARK: - Stdin Reader

/// Listen for "stop\n" on stdin. Runs on a background queue.
private func startStdinReader() {
    DispatchQueue.global(qos: .utility).async {
        while let line = readLine(strippingNewline: true) {
            if line == "stop" {
                logError("received stop command on stdin")
                requestShutdown()
                return
            }
        }
        // stdin closed (parent process died or pipe broken) -- shut down.
        logError("stdin closed, shutting down")
        requestShutdown()
    }
}

// MARK: - Audio Buffer Writer

/// Write interleaved PCM data from an AVAudioPCMBuffer to stdout.
///
/// AVAudioPCMBuffer stores channels in separate arrays (non-interleaved):
///   channel 0: [L0, L1, L2, ...]
///   channel 1: [R0, R1, R2, ...]
///
/// We interleave to: [L0, R0, L1, R1, L2, R2, ...]
/// Each sample is a Float32 (4 bytes). Stereo frame = 8 bytes.
private func writeBufferToStdout(_ buffer: AVAudioPCMBuffer) {
    guard let channelData = buffer.floatChannelData else { return }
    let frameCount = Int(buffer.frameLength)
    let channels = Int(buffer.format.channelCount)

    if channels == 0 || frameCount == 0 { return }

    // Allocate interleaved buffer: frameCount * channels * sizeof(Float32)
    let totalSamples = frameCount * channels
    let byteCount = totalSamples * MemoryLayout<Float32>.size

    let interleaved = UnsafeMutableBufferPointer<Float32>.allocate(capacity: totalSamples)
    defer { interleaved.deallocate() }

    // Interleave: for each frame, write all channels in order.
    for frame in 0..<frameCount {
        for ch in 0..<channels {
            interleaved[frame * channels + ch] = channelData[ch][frame]
        }
    }

    // Write raw bytes to stdout using POSIX write for thread safety.
    interleaved.baseAddress!.withMemoryRebound(to: UInt8.self, capacity: byteCount) { ptr in
        var written = 0
        while written < byteCount {
            let result = Darwin.write(STDOUT_FILENO, ptr + written, byteCount - written)
            if result < 0 {
                if errno == EINTR { continue }
                // Broken pipe or other write error -- shut down.
                requestShutdown()
                return
            }
            written += result
        }
    }
}

// MARK: - Core Audio Taps Capture (macOS 14.2+)

/// Run the audio capture pipeline using Core Audio Taps.
/// This function contains all macOS 14.2+ API usage, guarded by #available at the call site.
@available(macOS 14.2, *)
private func runAudioCapture() -> Int32 {
    // --- 1. Create the process tap for system audio ---
    //
    // CATapDescription with an empty exclusion list captures all system audio
    // from all processes. No processes are excluded.
    let tapDescription = CATapDescription(stereoGlobalTapButExcludeProcesses: [])

    var tapID: AudioObjectID = 0
    let tapStatus = AudioHardwareCreateProcessTap(tapDescription, &tapID)

    guard tapStatus == noErr else {
        // Instead of heuristically mapping OSStatus codes (which may vary across
        // macOS versions), use CGPreflightScreenCaptureAccess() as a reliable
        // indicator. Screen Recording and Audio Tap permissions are related on
        // macOS -- if screen capture access is not granted, the process tap
        // failure is almost certainly a permission issue.
        if !CGPreflightScreenCaptureAccess() {
            logError(
                "Screen Recording permission required. Grant in System Settings -> Privacy & Security -> Screen Recording"
            )
            return ExitCode.permissionDenied.rawValue
        }
        logError("failed to create audio process tap: OSStatus \(tapStatus)")
        return ExitCode.fatalError.rawValue
    }

    logError("process tap created (ID: \(tapID))")

    // --- 2. Configure AVAudioEngine ---
    let engine = AVAudioEngine()

    // Desired output format: 48kHz, 32-bit float, stereo.
    guard
        let desiredFormat = AVAudioFormat(
            standardFormatWithSampleRate: kSampleRate, channels: kChannelCount)
    else {
        logError("failed to create audio format (48kHz, Float32, stereo)")
        AudioHardwareDestroyProcessTap(tapID)
        return ExitCode.fatalError.rawValue
    }

    // Access the input node. On macOS, when a process tap is active, the engine's
    // input node will route audio from the tap.
    let inputNode = engine.inputNode

    // Disable voice processing -- we want raw audio, not processed.
    do {
        try inputNode.setVoiceProcessingEnabled(false)
    } catch {
        // Non-fatal: voice processing disable may fail on some configurations.
        logError("warning: could not disable voice processing: \(error)")
    }

    // Get the input format (may differ from our desired format).
    let inputFormat = inputNode.outputFormat(forBus: 0)
    logError(
        "input format: \(inputFormat.sampleRate)Hz, \(inputFormat.channelCount)ch"
    )

    // Use our desired format if channel count matches, otherwise use the input format.
    // Core Audio will handle any necessary format conversion.
    let tapFormat = inputFormat.channelCount == kChannelCount ? desiredFormat : inputFormat

    // Install a tap on the input node to capture audio buffers.
    inputNode.installTap(onBus: 0, bufferSize: kBufferSize, format: tapFormat) { buffer, _ in
        if shouldShutdown() { return }
        writeBufferToStdout(buffer)
    }

    // --- 3. Start the engine ---
    do {
        try engine.start()
    } catch {
        let nsError = error as NSError
        // Permission denied errors from Core Audio.
        if nsError.domain == NSOSStatusErrorDomain {
            logError(
                "Screen Recording permission required. Grant in System Settings -> Privacy & Security -> Screen Recording"
            )
            inputNode.removeTap(onBus: 0)
            AudioHardwareDestroyProcessTap(tapID)
            return ExitCode.permissionDenied.rawValue
        }
        logError("failed to start audio engine: \(error)")
        inputNode.removeTap(onBus: 0)
        AudioHardwareDestroyProcessTap(tapID)
        return ExitCode.fatalError.rawValue
    }

    logError(
        "audio capture started: \(tapFormat.sampleRate)Hz, \(tapFormat.channelCount)ch, Float32"
    )

    // --- 4. Wait for shutdown signal ---
    shutdownRequested.wait()

    // --- 5. Graceful shutdown: stop engine, destroy tap ---
    logError("shutting down...")
    inputNode.removeTap(onBus: 0)
    engine.stop()
    AudioHardwareDestroyProcessTap(tapID)
    logError("audio capture stopped")

    return ExitCode.success.rawValue
}

// MARK: - Entry Point

// --- macOS version check (runtime, covers pre-14.0 as well) ---
let osVersion = ProcessInfo.processInfo.operatingSystemVersion
guard osVersion.majorVersion > 14
    || (osVersion.majorVersion == 14 && osVersion.minorVersion >= 2)
else {
    logError(
        "macOS 14.2 or later required (Core Audio Taps). Current: \(ProcessInfo.processInfo.operatingSystemVersionString)"
    )
    exit(ExitCode.fatalError.rawValue)
}

// --- Install signal handlers and stdin reader ---
installSignalHandlers()
startStdinReader()

// --- Run capture (guarded by #available for compile-time safety) ---
if #available(macOS 14.2, *) {
    let code = runAudioCapture()
    exit(code)
} else {
    // This branch is unreachable due to the runtime check above,
    // but satisfies the compiler's availability requirements.
    logError(
        "macOS 14.2 or later required (Core Audio Taps). Current: \(ProcessInfo.processInfo.operatingSystemVersionString)"
    )
    exit(ExitCode.fatalError.rawValue)
}
