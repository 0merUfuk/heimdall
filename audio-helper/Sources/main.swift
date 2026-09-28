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
//
// Subcommands:
//   --check-permissions  Preflight the Screen Recording permission (no
//                        system prompt, no capture). Prints a single line
//                        to stdout: "screen-recording-permission: granted"
//                        or "screen-recording-permission: denied". Exits 0
//                        when granted, 77 when denied. Used by
//                        `heimdall doctor` to surface the permission state
//                        without triggering the macOS consent dialog.

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

// MARK: - Format Conversion

/// Convert a tap buffer to the Go mixer's format (48kHz stereo Float32).
///
/// The process tap's format follows the hardware: a USB headset can present
/// 44.1kHz mono, while the mixer's contract is fixed at 48kHz stereo. Returns
/// nil on a conversion error (logged once per process, then dropped silently
/// so a broken converter cannot flood the meeting's stderr).
private nonisolated(unsafe) var conversionErrorLogged = false

private func convertBuffer(
    _ buffer: AVAudioPCMBuffer, using converter: AVAudioConverter, to format: AVAudioFormat
) -> AVAudioPCMBuffer? {
    let ratio = format.sampleRate / buffer.format.sampleRate
    // +1 frame of slack: the converter may emit one extra frame when the
    // input frame count does not divide evenly by the resampling ratio.
    let capacity = AVAudioFrameCount(Double(buffer.frameLength) * ratio) + 1
    guard let output = AVAudioPCMBuffer(pcmFormat: format, frameCapacity: capacity) else {
        return nil
    }

    var consumed = false
    var error: NSError?
    let status = converter.convert(to: output, error: &error) { _, inputStatus in
        if consumed {
            inputStatus.pointee = .noDataNow
            return nil
        }
        consumed = true
        inputStatus.pointee = .haveData
        return buffer
    }

    if status == .error || output.frameLength == 0 {
        if !conversionErrorLogged {
            conversionErrorLogged = true
            logError("audio conversion failed: \(error?.localizedDescription ?? "unknown error")")
        }
        return nil
    }
    return output
}

// MARK: - Audio Buffer Writer

/// Write interleaved stereo PCM data from an AVAudioPCMBuffer to stdout.
///
/// AVAudioPCMBuffer stores channels in separate arrays (non-interleaved):
///   channel 0: [L0, L1, L2, ...]
///   channel 1: [R0, R1, R2, ...]
///
/// We interleave to: [L0, R0, L1, R1, L2, R2, ...]
/// Each sample is a Float32 (4 bytes). Stereo frame = 8 bytes.
///
/// If the buffer is mono (1 channel), the mono data is duplicated to both L and R
/// channels to produce stereo output. This ensures the Go mixer always receives
/// stereo (2ch) from the Swift helper regardless of hardware tap format.
private func writeBufferToStdout(_ buffer: AVAudioPCMBuffer) {
    guard let channelData = buffer.floatChannelData else { return }
    let frameCount = Int(buffer.frameLength)
    let channels = Int(buffer.format.channelCount)

    if channels == 0 || frameCount == 0 { return }

    // Output is always stereo (2 channels), even if input is mono.
    let outputChannels = 2
    let totalSamples = frameCount * outputChannels
    let byteCount = totalSamples * MemoryLayout<Float32>.size

    let interleaved = UnsafeMutableBufferPointer<Float32>.allocate(capacity: totalSamples)
    defer { interleaved.deallocate() }

    if channels == 1 {
        // Mono input: duplicate the single channel to both L and R.
        let monoData = channelData[0]
        for frame in 0..<frameCount {
            let sample = monoData[frame]
            interleaved[frame * outputChannels]     = sample  // L
            interleaved[frame * outputChannels + 1] = sample  // R
        }
    } else {
        // Stereo or multichannel input: interleave first two channels.
        for frame in 0..<frameCount {
            interleaved[frame * outputChannels]     = channelData[0][frame]  // L
            interleaved[frame * outputChannels + 1] = channelData[1][frame]  // R
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

// MARK: - Capture Diagnostics

/// Running totals for the IOProc, touched only from its serial queue (and once
/// at shutdown after the queue has stopped), so no lock is needed.
private nonisolated(unsafe) var diagFrames: Int = 0
private nonisolated(unsafe) var diagRawPeak: Float = 0
private nonisolated(unsafe) var diagWarnedSilent = false

/// Peak absolute sample over every channel of a Float32 buffer (either
/// layout). Returns 0 for non-Float32 data rather than misreading it.
private func peakOf(_ buffer: AVAudioPCMBuffer) -> Float {
    guard buffer.format.commonFormat == .pcmFormatFloat32, let data = buffer.floatChannelData else {
        return 0
    }
    let frames = Int(buffer.frameLength)
    let channels = buffer.format.isInterleaved ? 1 : Int(buffer.format.channelCount)
    let stride = buffer.format.isInterleaved ? Int(buffer.format.channelCount) : 1
    var peak: Float = 0
    for c in 0..<channels {
        let ch = data[c]
        for i in 0..<(frames * stride) { peak = max(peak, abs(ch[i])) }
    }
    return peak
}

/// Track the raw tap signal. A tap that is running but has only ever produced
/// digital zeros for ~5s is the signature of a missing system-audio
/// permission (macOS delivers silence instead of an error); say so once.
private func recordDiagnostics(_ buffer: AVAudioPCMBuffer, sampleRate: Double) {
    diagFrames += Int(buffer.frameLength)
    diagRawPeak = max(diagRawPeak, peakOf(buffer))
    if !diagWarnedSilent && diagRawPeak == 0 && Double(diagFrames) >= sampleRate * 5 {
        diagWarnedSilent = true
        logError(
            "warning: the system-audio tap has delivered 5s of digital silence. Either nothing is playing, or this app lacks the system audio recording permission (System Settings -> Privacy & Security -> Screen & System Audio Recording)"
        )
    }
}

// MARK: - Core Audio Taps Capture (macOS 14.2+)

/// UID of the current default *system* output device. Used only to log which
/// device the global tap is following.
@available(macOS 14.2, *)
private func defaultOutputDeviceName() -> String {
    var deviceID = AudioObjectID(kAudioObjectUnknown)
    var size = UInt32(MemoryLayout<AudioObjectID>.size)
    var addr = AudioObjectPropertyAddress(
        mSelector: kAudioHardwarePropertyDefaultSystemOutputDevice,
        mScope: kAudioObjectPropertyScopeGlobal,
        mElement: kAudioObjectPropertyElementMain)
    guard
        AudioObjectGetPropertyData(
            AudioObjectID(kAudioObjectSystemObject), &addr, 0, nil, &size, &deviceID) == noErr,
        deviceID != kAudioObjectUnknown
    else { return "unknown" }

    var name: Unmanaged<CFString>?
    var nameSize = UInt32(MemoryLayout<Unmanaged<CFString>?>.size)
    var nameAddr = AudioObjectPropertyAddress(
        mSelector: kAudioObjectPropertyName,
        mScope: kAudioObjectPropertyScopeGlobal,
        mElement: kAudioObjectPropertyElementMain)
    guard AudioObjectGetPropertyData(deviceID, &nameAddr, 0, nil, &nameSize, &name) == noErr,
        let name
    else { return "unknown" }
    return name.takeRetainedValue() as String
}

/// Run the audio capture pipeline using Core Audio Taps.
/// This function contains all macOS 14.2+ API usage, guarded by #available at the call site.
///
/// A process tap is only a *source*: creating one does nothing until it is
/// attached to an aggregate device whose IOProc reads it. (An earlier version
/// created the tap and then read AVAudioEngine.inputNode -- which is the
/// default INPUT device, i.e. the microphone -- so "system audio" was a second
/// copy of the mic. See .claude/DECISIONS.md ID-017.)
///
/// The aggregate deliberately has an empty sub-device list: with only the tap
/// in it, no physical input (a headset mic, the built-in mic) can ever appear
/// in the buffers this IOProc receives.
@available(macOS 14.2, *)
private func runAudioCapture() -> Int32 {
    // --- 1. Create the process tap for system audio ---
    //
    // A global tap with an empty exclusion list captures the output of every
    // process. Private: not visible to other apps. Unmuted: the user still
    // hears the meeting.
    let tapDescription = CATapDescription(stereoGlobalTapButExcludeProcesses: [])
    tapDescription.name = "heimdall system audio"
    tapDescription.isPrivate = true
    tapDescription.muteBehavior = .unmuted

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
    logError("process tap created (ID: \(tapID)), following output: \(defaultOutputDeviceName())")

    // --- 2. Read the tap's real format ---
    var asbd = AudioStreamBasicDescription()
    var asbdSize = UInt32(MemoryLayout<AudioStreamBasicDescription>.size)
    var formatAddr = AudioObjectPropertyAddress(
        mSelector: kAudioTapPropertyFormat,
        mScope: kAudioObjectPropertyScopeGlobal,
        mElement: kAudioObjectPropertyElementMain)
    let formatStatus = AudioObjectGetPropertyData(tapID, &formatAddr, 0, nil, &asbdSize, &asbd)
    guard formatStatus == noErr, let tapFormat = AVAudioFormat(streamDescription: &asbd) else {
        logError("failed to read the tap format: OSStatus \(formatStatus)")
        AudioHardwareDestroyProcessTap(tapID)
        return ExitCode.fatalError.rawValue
    }

    // Desired output format: 48kHz, 32-bit float, stereo (deinterleaved
    // standard format). The Go mixer's contract is fixed at this.
    guard
        let desiredFormat = AVAudioFormat(
            standardFormatWithSampleRate: kSampleRate, channels: kChannelCount)
    else {
        logError("failed to create audio format (48kHz, Float32, stereo)")
        AudioHardwareDestroyProcessTap(tapID)
        return ExitCode.fatalError.rawValue
    }

    // Compare the whole format, not just rate and channel count: interleaved
    // or non-Float32 data would break writeBufferToStdout, which assumes
    // deinterleaved Float32.
    var converter: AVAudioConverter?
    if !tapFormat.isEqual(desiredFormat) {
        converter = AVAudioConverter(from: tapFormat, to: desiredFormat)
        if converter == nil {
            logError(
                "failed to create converter \(tapFormat.sampleRate)Hz/\(tapFormat.channelCount)ch -> \(desiredFormat.sampleRate)Hz/\(desiredFormat.channelCount)ch"
            )
            AudioHardwareDestroyProcessTap(tapID)
            return ExitCode.fatalError.rawValue
        }
        logError(
            "converting \(tapFormat.sampleRate)Hz/\(tapFormat.channelCount)ch/\(tapFormat.isInterleaved ? "interleaved" : "deinterleaved") -> \(Int(desiredFormat.sampleRate))Hz/\(desiredFormat.channelCount)ch Float32"
        )
    }

    // --- 3. Wrap the tap in a private aggregate device ---
    let aggregateDescription: [String: Any] = [
        kAudioAggregateDeviceNameKey: "heimdall-audio tap",
        kAudioAggregateDeviceUIDKey: UUID().uuidString,
        kAudioAggregateDeviceIsPrivateKey: true,
        kAudioAggregateDeviceIsStackedKey: false,
        kAudioAggregateDeviceTapAutoStartKey: true,
        // Empty on purpose -- see the function comment.
        kAudioAggregateDeviceSubDeviceListKey: [] as [[String: Any]],
        kAudioAggregateDeviceTapListKey: [
            [
                kAudioSubTapUIDKey: tapDescription.uuid.uuidString,
                kAudioSubTapDriftCompensationKey: true,
            ]
        ],
    ]

    var aggregateID: AudioObjectID = 0
    let aggregateStatus = AudioHardwareCreateAggregateDevice(
        aggregateDescription as CFDictionary, &aggregateID)
    guard aggregateStatus == noErr else {
        logError("failed to create the aggregate device for the tap: OSStatus \(aggregateStatus)")
        AudioHardwareDestroyProcessTap(tapID)
        return ExitCode.fatalError.rawValue
    }

    // --- 4. Read the tap through an IOProc on the aggregate ---
    //
    // The block runs on our own queue (not the realtime thread), so allocating
    // and blocking on the stdout pipe here cannot glitch the device.
    let ioQueue = DispatchQueue(label: "heimdall-audio.tap", qos: .userInteractive)
    var ioProcID: AudioDeviceIOProcID?
    let ioStatus = AudioDeviceCreateIOProcIDWithBlock(&ioProcID, aggregateID, ioQueue) {
        _, inInputData, _, _, _ in
        if shouldShutdown() { return }
        guard
            let buffer = AVAudioPCMBuffer(
                pcmFormat: tapFormat, bufferListNoCopy: inInputData, deallocator: nil)
        else { return }
        recordDiagnostics(buffer, sampleRate: tapFormat.sampleRate)
        guard let converter else {
            writeBufferToStdout(buffer)
            return
        }
        guard let converted = convertBuffer(buffer, using: converter, to: desiredFormat) else {
            return
        }
        writeBufferToStdout(converted)
    }

    func teardown() {
        if let ioProcID {
            AudioDeviceStop(aggregateID, ioProcID)
            AudioDeviceDestroyIOProcID(aggregateID, ioProcID)
        }
        AudioHardwareDestroyAggregateDevice(aggregateID)
        AudioHardwareDestroyProcessTap(tapID)
    }

    guard ioStatus == noErr, let ioProcID else {
        logError("failed to create the IOProc on the tap aggregate: OSStatus \(ioStatus)")
        teardown()
        return ExitCode.fatalError.rawValue
    }

    // --- 5. Start capturing ---
    let startStatus = AudioDeviceStart(aggregateID, ioProcID)
    guard startStatus == noErr else {
        logError("failed to start the tap aggregate: OSStatus \(startStatus)")
        teardown()
        return startStatus == kAudioHardwareIllegalOperationError
            ? ExitCode.permissionDenied.rawValue : ExitCode.fatalError.rawValue
    }

    logError(
        "audio capture started: tap=\(tapFormat.sampleRate)Hz/\(tapFormat.channelCount)ch, output=\(kSampleRate)Hz/\(kChannelCount)ch, Float32"
    )

    // --- 6. Wait for shutdown signal, then tear everything down ---
    shutdownRequested.wait()

    logError("shutting down...")
    teardown()
    logError(
        "audio capture stopped (tap delivered \(diagFrames) frames, raw peak \(diagRawPeak))")

    return ExitCode.success.rawValue
}

// MARK: - Permission Preflight Subcommand

/// Handle `--check-permissions`: report Screen Recording permission state
/// without triggering the system prompt. Prints a single machine-parseable
/// line to stdout so the Go side (`heimdall doctor`) can key off either the
/// exit code or the string.
///
/// Uses CGPreflightScreenCaptureAccess() only. Calling
/// CGRequestScreenCaptureAccess() here would surface the macOS consent
/// dialog, which `doctor` must not do without explicit user action.
private func runPermissionCheck() -> Int32 {
    let granted = CGPreflightScreenCaptureAccess()
    var line = granted
        ? "screen-recording-permission: granted\n"
        : "screen-recording-permission: denied\n"
    line.withUTF8 { buffer in
        _ = fwrite(buffer.baseAddress, 1, buffer.count, stdout)
        fflush(stdout)
    }
    return granted ? ExitCode.success.rawValue : ExitCode.permissionDenied.rawValue
}

// MARK: - Entry Point

// --- Early subcommand dispatch (no audio engine, no macOS 14.2 guard) ---
// CommandLine.arguments[0] is the binary path; real args start at index 1.
if CommandLine.arguments.count >= 2 {
    switch CommandLine.arguments[1] {
    case "--check-permissions":
        exit(runPermissionCheck())
    default:
        break
    }
}

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
