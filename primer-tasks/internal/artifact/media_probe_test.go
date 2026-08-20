package artifact

import (
	"encoding/binary"
	"testing"
)

func TestProbeDurationUsesEncodedMediaMetadata(t *testing.T) {
	mp4 := make([]byte, 40)
	copy(mp4[0:], []byte("mvhd"))
	mp4[4] = 0
	binary.BigEndian.PutUint32(mp4[16:20], 1000)
	binary.BigEndian.PutUint32(mp4[20:24], 1500)
	if duration, ok := ProbeDuration(Video, mp4); !ok || duration != 1500 {
		t.Fatalf("mp4 duration=%d ok=%v", duration, ok)
	}
	mp3 := []byte{0xff, 0xfb, 0x40, 0x00}
	mp3 = append(mp3, make([]byte, 1000)...)
	if duration, ok := ProbeDuration(Audio, mp3); !ok || duration <= 0 {
		t.Fatalf("mp3 duration=%d ok=%v", duration, ok)
	}
	if _, ok := ProbeDuration(Audio, []byte("truncated")); ok {
		t.Fatal("truncated audio received duration")
	}
}
