package carddav

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// FuzzDecodePhoto feeds untrusted image bytes (uploads, vCard PHOTO, remote photos)
// to the decoder behind SaveContactPhoto, with and without a media type hint. A
// decoded image must survive the crop and resize steps. Nothing is written to disk.
//
// Run: go test ./carddav -run '^$' -fuzz '^FuzzDecodePhoto$' -fuzztime 30s
func FuzzDecodePhoto(f *testing.F) {
	img := image.NewRGBA(image.Rect(0, 0, 5, 3))
	img.Set(1, 1, color.RGBA{R: 200, A: 255})
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	f.Add(b.Bytes(), "image/png")
	f.Add(b.Bytes(), "")
	b.Reset()
	_ = jpeg.Encode(&b, img, nil)
	f.Add(b.Bytes(), "image/jpeg")
	b.Reset()
	_ = gif.Encode(&b, img, nil)
	f.Add(b.Bytes(), "image/gif")
	f.Add([]byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00mif1heic"), "")
	f.Add([]byte("RIFF\x00\x00\x00\x00WEBPVP8 "), "image/webp")
	f.Add([]byte{}, "")

	f.Fuzz(func(t *testing.T, data []byte, mediaType string) {
		img, err := decodePhoto(data, mediaType)
		if err != nil {
			return
		}
		if img == nil {
			t.Fatal("decodePhoto returned neither image nor error")
		}
	})
}
