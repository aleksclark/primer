package securityreview

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
)

// JPEGWithEXIF is a real, decodable JPEG with a deterministic EXIF APP1 block.
// The GPS values are deliberately synthetic review data, never student data.
func JPEGWithEXIF() []byte {
	pixels := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			pixels.Set(x, y, color.RGBA{R: uint8(40 + x*30), G: uint8(80 + y*30), B: 120, A: 255})
		}
	}
	var encoded bytes.Buffer
	_ = jpeg.Encode(&encoded, pixels, &jpeg.Options{Quality: 90})
	jpegBytes := encoded.Bytes()
	// APP1: Exif header plus a valid little-endian TIFF GPS IFD. The values
	// are synthetic review metadata and never contain student/device data.
	tiff := make([]byte, 68)
	tiff[0], tiff[1], tiff[2], tiff[3] = 'I', 'I', 42, 0
	binary.LittleEndian.PutUint32(tiff[4:8], 8) // IFD0 offset
	binary.LittleEndian.PutUint16(tiff[8:10], 1)
	binary.LittleEndian.PutUint16(tiff[10:12], 0x8825) // GPSInfo pointer
	binary.LittleEndian.PutUint16(tiff[12:14], 4)      // LONG
	binary.LittleEndian.PutUint32(tiff[14:18], 1)
	binary.LittleEndian.PutUint32(tiff[18:22], 26) // GPS IFD offset
	binary.LittleEndian.PutUint32(tiff[22:26], 0)
	binary.LittleEndian.PutUint16(tiff[26:28], 1)
	binary.LittleEndian.PutUint16(tiff[28:30], 2) // GPSLatitude
	binary.LittleEndian.PutUint16(tiff[30:32], 5) // RATIONAL
	binary.LittleEndian.PutUint32(tiff[32:36], 3)
	binary.LittleEndian.PutUint32(tiff[36:40], 44)
	binary.LittleEndian.PutUint32(tiff[40:44], 0)
	for i, degrees := range []uint32{36, 7, 30} {
		offset := 44 + i*8
		binary.LittleEndian.PutUint32(tiff[offset:offset+4], degrees)
		binary.LittleEndian.PutUint32(tiff[offset+4:offset+8], 1)
	}
	exif := append([]byte{0xff, 0xe1, 0, 0}, append([]byte{'E', 'x', 'i', 'f', 0, 0}, tiff...)...)
	binary.BigEndian.PutUint16(exif[2:4], uint16(len(exif)-2))
	return append(append(append([]byte{}, jpegBytes[:2]...), exif...), jpegBytes[2:]...)
}

// WAVWithPCM is a valid PCM/WAVE file, useful for capability and upload tests
// that must not substitute a string pretending to be media.
func WAVWithPCM() []byte {
	const sampleRate = uint32(8000)
	pcm := []byte{0, 0, 0x20, 0x10, 0x40, 0x20, 0x20, 0x10}
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(pcm)))
	b.WriteString("WAVEfmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, sampleRate)
	_ = binary.Write(&b, binary.LittleEndian, sampleRate*2)
	_ = binary.Write(&b, binary.LittleEndian, uint16(2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}
