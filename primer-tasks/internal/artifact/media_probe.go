package artifact

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// ProbeDuration extracts duration from the encoded container/frame metadata,
// never from browser metadata. It intentionally supports the common fixture
// formats; unsupported codecs are routed to review rather than auto-accepted.
func ProbeDuration(kind Kind, data []byte) (int64, bool) {
	switch kind {
	case Audio:
		return probeMP3(data)
	case Video:
		return probeMP4(data)
	case Image:
		return 0, true
	default:
		return 0, false
	}
}
func probeMP4(data []byte) (int64, bool) {
	i := bytes.Index(data, []byte("mvhd"))
	if i < 0 || i+24 > len(data) {
		return 0, false
	}
	version := data[i+4]
	if version == 0 {
		if i+24 > len(data) {
			return 0, false
		}
		timescale, duration := binary.BigEndian.Uint32(data[i+16:i+20]), binary.BigEndian.Uint32(data[i+20:i+24])
		if timescale == 0 {
			return 0, false
		}
		return int64(duration) * 1000 / int64(timescale), true
	}
	if version == 1 && i+40 <= len(data) {
		timescale := binary.BigEndian.Uint32(data[i+28 : i+32])
		duration := binary.BigEndian.Uint64(data[i+32 : i+40])
		if timescale == 0 {
			return 0, false
		}
		return int64(duration) * 1000 / int64(timescale), true
	}
	return 0, false
}
func probeMP3(data []byte) (int64, bool) {
	start := bytes.Index(data, []byte("ID3"))
	if start < 0 {
		start = 0
	}
	for i := start; i+4 < len(data); i++ {
		if data[i] != 0xff || data[i+1]&0xe0 != 0xe0 {
			continue
		}
		version := (data[i+1] >> 3) & 3
		bitrateIndex := (data[i+2] >> 4) & 15
		sampleIndex := (data[i+2] >> 2) & 3
		if version == 1 || bitrateIndex == 0 || bitrateIndex == 15 || sampleIndex == 3 {
			continue
		}
		bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
		bitrate := bitrates[bitrateIndex] * 1000
		if bitrate == 0 {
			continue
		}
		return int64(len(data)-i) * 8 * 1000 / int64(bitrate), true
	}
	return 0, false
}

var errUnsupportedDuration = errors.New("encoded media duration is unavailable")
