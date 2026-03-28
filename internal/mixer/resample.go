// Package mixer implements audio resampling, format conversion, and stereo
// interleaving for the heimdall pipeline Stage 2 (MIX).
//
// The mixer consumes two AudioSource streams (system audio at 48kHz/32-bit float/stereo
// and microphone at 16kHz/16-bit int/mono), resamples, converts, and interleaves them
// into a single stereo stream at 16kHz/16-bit int for Deepgram.
//
// Dual-channel convention (AD-007):
//   - Left channel  = system audio (remote meeting participants)
//   - Right channel = microphone (local user)
package mixer

import "math"

// Resample48to16 resamples audio from 48kHz to 16kHz using the 3:1 integer ratio
// fast path. It takes every 3rd sample (simple decimation).
//
// For speech audio in v1.0, simple decimation without an anti-aliasing filter is
// acceptable. The speech frequency band (300Hz-3400Hz) is well below the Nyquist
// frequency of 8kHz at the target sample rate.
//
// Input: float32 samples at 48kHz (mono).
// Output: int16 samples at 16kHz.
func Resample48to16(input []float32) []int16 {
	if len(input) == 0 {
		return nil
	}
	outputLen := len(input) / 3
	output := make([]int16, outputLen)
	for i := 0; i < outputLen; i++ {
		output[i] = Float32ToInt16(input[i*3])
	}
	return output
}

// Resample48to16Mono resamples interleaved stereo 48kHz float32 audio to mono 16kHz int16.
// This is a convenience function that performs stereo-to-mono conversion and resampling
// in a single pass for efficiency.
//
// Input: interleaved stereo float32 samples at 48kHz [L0, R0, L1, R1, ...].
// Output: mono int16 samples at 16kHz.
func Resample48to16Mono(stereo []float32) []int16 {
	mono := StereoToMono(stereo)
	return Resample48to16(mono)
}

// Float32ToInt16 converts a 32-bit float sample in the range [-1.0, 1.0] to a
// 16-bit signed integer. Values outside [-1.0, 1.0] are clamped to prevent overflow.
func Float32ToInt16(f float32) int16 {
	// Clamp to [-1.0, 1.0]
	if f > 1.0 {
		f = 1.0
	} else if f < -1.0 {
		f = -1.0
	}
	// Scale to int16 range. Use math.Round for correct rounding at boundaries.
	return int16(math.Round(float64(f) * 32767.0))
}

// StereoToMono converts interleaved stereo float32 samples to mono by averaging
// the left and right channels.
//
// Input: interleaved stereo [L0, R0, L1, R1, ...] — must have even length.
// Output: mono [M0, M1, ...] where M[i] = (L[i] + R[i]) / 2.
//
// If input has odd length, the last sample is dropped.
func StereoToMono(stereo []float32) []float32 {
	if len(stereo) < 2 {
		return nil
	}
	monoLen := len(stereo) / 2
	mono := make([]float32, monoLen)
	for i := 0; i < monoLen; i++ {
		mono[i] = (stereo[2*i] + stereo[2*i+1]) / 2.0
	}
	return mono
}

// InterleaveInt16 interleaves two mono int16 sample slices into a stereo stream.
// left becomes channel 0 (system audio) and right becomes channel 1 (microphone),
// per the AD-007 dual-channel convention.
//
// Output: [L0, R0, L1, R1, ...].
//
// If slices differ in length, the shorter one is padded with silence (zeros).
func InterleaveInt16(left, right []int16) []int16 {
	n := len(left)
	if len(right) > n {
		n = len(right)
	}
	if n == 0 {
		return nil
	}
	stereo := make([]int16, n*2)
	for i := 0; i < n; i++ {
		if i < len(left) {
			stereo[2*i] = left[i]
		}
		// else: stereo[2*i] is already 0 (silence)
		if i < len(right) {
			stereo[2*i+1] = right[i]
		}
		// else: stereo[2*i+1] is already 0 (silence)
	}
	return stereo
}

// Int16ToBytes converts a slice of int16 samples to little-endian byte representation.
// This is the format expected by Deepgram (linear16 encoding).
func Int16ToBytes(samples []int16) []byte {
	buf := make([]byte, len(samples)*2)
	for i, s := range samples {
		buf[2*i] = byte(s)
		buf[2*i+1] = byte(s >> 8)
	}
	return buf
}

// BytesToInt16 converts little-endian byte pairs to int16 samples.
// If the byte slice has an odd length, the last byte is ignored.
func BytesToInt16(data []byte) []int16 {
	n := len(data) / 2
	if n == 0 {
		return nil
	}
	samples := make([]int16, n)
	for i := 0; i < n; i++ {
		samples[i] = int16(data[2*i]) | int16(data[2*i+1])<<8
	}
	return samples
}

// BytesToFloat32 converts little-endian byte quads to float32 samples.
// If the byte slice length is not a multiple of 4, trailing bytes are ignored.
func BytesToFloat32(data []byte) []float32 {
	n := len(data) / 4
	if n == 0 {
		return nil
	}
	samples := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := uint32(data[4*i]) | uint32(data[4*i+1])<<8 | uint32(data[4*i+2])<<16 | uint32(data[4*i+3])<<24
		samples[i] = math.Float32frombits(bits)
	}
	return samples
}
