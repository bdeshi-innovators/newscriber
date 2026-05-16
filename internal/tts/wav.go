package tts

import (
	"bytes"
	"encoding/binary"
)

const (
	SampleRate     = 24000
	NumChannels    = 1
	BitsPerSample  = 16
)

// PcmToWav prepends a WAV/RIFF header to raw 16-bit LE mono PCM bytes.
func PcmToWav(pcm []byte) []byte {
	dataSize := len(pcm)
	riffSize := 4 + 24 + 8 + dataSize

	buf := new(bytes.Buffer)

	// RIFF header
	buf.Write([]byte("RIFF"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(riffSize))
	buf.Write([]byte("WAVE"))

	// fmt chunk
	buf.Write([]byte("fmt "))
	_ = binary.Write(buf, binary.LittleEndian, uint32(16)) // chunk size
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))  // PCM format
	_ = binary.Write(buf, binary.LittleEndian, uint16(NumChannels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(SampleRate))

	byteRate := uint32(SampleRate * NumChannels * (BitsPerSample / 8))
	_ = binary.Write(buf, binary.LittleEndian, byteRate)

	blockAlign := uint16(NumChannels * (BitsPerSample / 8))
	_ = binary.Write(buf, binary.LittleEndian, blockAlign)
	_ = binary.Write(buf, binary.LittleEndian, uint16(BitsPerSample))

	// data chunk
	buf.Write([]byte("data"))
	_ = binary.Write(buf, binary.LittleEndian, uint32(dataSize))
	buf.Write(pcm)

	return buf.Bytes()
}
