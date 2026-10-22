package imageproc

import (
	"bytes"
	"encoding/binary"
	"errors"
)

var exifPrefix = []byte("Exif\x00\x00")

// ExifOf returns the EXIF block of a JPEG, PNG, WebP or HEIC file, or nil.
func ExifOf(data []byte) []byte {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8}):
		return exifOfJPEG(data)
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return exifOfPNG(data)
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return ExifOfWebP(data)
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return exifOfHEIC(data)
	}
	return nil
}

func exifOfJPEG(data []byte) []byte {
	for pos := 2; pos+4 <= len(data) && data[pos] == 0xFF; {
		marker, size := data[pos+1], int(binary.BigEndian.Uint16(data[pos+2:]))
		if marker == 0xDA || size < 2 || pos+2+size > len(data) {
			return nil
		}
		if payload := data[pos+4 : pos+2+size]; marker == 0xE1 && bytes.HasPrefix(payload, exifPrefix) {
			return payload[len(exifPrefix):]
		}
		pos += 2 + size
	}
	return nil
}

func exifOfPNG(data []byte) []byte {
	for pos := 8; pos+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[pos:]))
		if size < 0 || pos+12+size > len(data) {
			return nil
		}
		if string(data[pos+4:pos+8]) == "eXIf" {
			return data[pos+8 : pos+8+size]
		}
		pos += 12 + size
	}
	return nil
}

type riffChunk struct {
	id   string
	data []byte
}

func riffChunks(data []byte) []riffChunk {
	var out []riffChunk
	for pos := 12; pos+8 <= len(data); {
		size := int(binary.LittleEndian.Uint32(data[pos+4:]))
		if size < 0 || pos+8+size > len(data) {
			break
		}
		out = append(out, riffChunk{string(data[pos : pos+4]), data[pos+8 : pos+8+size]})
		pos += 8 + size + size%2
	}
	return out
}

func ExifOfWebP(data []byte) []byte {
	for _, c := range riffChunks(data) {
		if c.id == "EXIF" {
			return bytes.TrimPrefix(c.data, exifPrefix)
		}
	}
	return nil
}

func riff(chunks []riffChunk) []byte {
	body := []byte("WEBP")
	for _, c := range chunks {
		body = append(body, c.id...)
		body = binary.LittleEndian.AppendUint32(body, uint32(len(c.data))) //nolint:gosec // chunks are far below 4 GB
		body = append(body, c.data...)
		if len(c.data)%2 == 1 {
			body = append(body, 0)
		}
	}
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...) //nolint:gosec // files are far below 4 GB
	return append(out, body...)
}

// AddExifToWebP adds an EXIF chunk (and the VP8X header that must announce it) to a WebP
// file that has none. width and height are the pixel size; alpha says the image has transparency.
func AddExifToWebP(data, exif []byte, width, height int, alpha bool) ([]byte, error) {
	chunks := riffChunks(data)
	if len(chunks) == 0 {
		return nil, errors.New("not a WebP file")
	}
	if chunks[0].id == "VP8X" && len(chunks[0].data) == 10 {
		head := append([]byte(nil), chunks[0].data...)
		head[0] |= 0x08
		chunks[0].data = head
	} else {
		head := make([]byte, 10)
		head[0] = 0x08
		if alpha {
			head[0] |= 0x10
		}
		for i, v := range []int{width - 1, height - 1} {
			head[4+3*i], head[5+3*i], head[6+3*i] = byte(v&0xFF), byte(v>>8&0xFF), byte(v>>16&0xFF)
		}
		chunks = append([]riffChunk{{"VP8X", head}}, chunks...)
	}
	return riff(append(chunks, riffChunk{"EXIF", exif})), nil
}

// exifOfHEIC finds the Exif item through the iinf and iloc boxes of the meta box.
func exifOfHEIC(data []byte) []byte {
	meta := findBox(data, "meta")
	if len(meta) < 4 {
		return nil
	}
	meta = meta[4:]
	var exifID uint32
	var found bool
	if iinf := findBox(meta, "iinf"); len(iinf) >= 6 {
		skip := 6
		if iinf[0] != 0 {
			skip = 8
		}
		for rest := iinf[min(skip, len(iinf)):]; len(rest) >= 8; {
			size := int(binary.BigEndian.Uint32(rest))
			if size < 8 || size > len(rest) {
				break
			}
			if body := rest[8:size]; string(rest[4:8]) == "infe" && len(body) >= 12 && body[0] >= 2 {
				idLen := 2
				if body[0] == 3 {
					idLen = 4
				}
				if 4+idLen+2+4 <= len(body) && string(body[4+idLen+2:4+idLen+6]) == "Exif" {
					exifID, found = readUint(body[4:4+idLen]), true
				}
			}
			rest = rest[size:]
		}
	}
	if !found {
		return nil
	}
	offset, length, ok := ilocExtent(findBox(meta, "iloc"), exifID)
	if !ok || offset > uint64(len(data)) || length > uint64(len(data))-offset || length < 4 {
		return nil
	}
	item := data[offset : offset+length]
	start := 4 + uint64(binary.BigEndian.Uint32(item))
	if start >= uint64(len(item)) {
		return nil
	}
	return bytes.TrimPrefix(item[start:], exifPrefix)
}

func readUint(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}

// findBox returns the payload of the first box of the given type in data.
func findBox(data []byte, typ string) []byte {
	for len(data) >= 8 {
		size := int(binary.BigEndian.Uint32(data))
		header := 8
		if size == 1 && len(data) >= 16 {
			size, header = int(min(binary.BigEndian.Uint64(data[8:]), 1<<31)), 16
		}
		if size < header || size > len(data) {
			return nil
		}
		if string(data[4:8]) == typ {
			return data[header:size]
		}
		data = data[size:]
	}
	return nil
}

func ilocExtent(iloc []byte, id uint32) (offset, length uint64, ok bool) {
	if len(iloc) < 8 {
		return 0, 0, false
	}
	version := iloc[0]
	offSize, lenSize := int(iloc[4]>>4), int(iloc[4]&0xF)
	baseSize, idxSize := int(iloc[5]>>4), 0
	if version >= 1 {
		idxSize = int(iloc[5] & 0xF)
	}
	pos, count := 6, uint32(0)
	if version < 2 {
		count, pos = uint32(binary.BigEndian.Uint16(iloc[pos:])), pos+2
	} else {
		if len(iloc) < pos+4 {
			return 0, 0, false
		}
		count, pos = binary.BigEndian.Uint32(iloc[pos:]), pos+4
	}
	take := func(n int) (uint64, bool) {
		if pos+n > len(iloc) {
			return 0, false
		}
		v := uint64(readUint(iloc[pos : pos+min(n, 4)]))
		if n == 8 {
			v = binary.BigEndian.Uint64(iloc[pos:])
		}
		pos += n
		return v, true
	}
	for range count {
		idLen := 2
		if version == 2 {
			idLen = 4
		}
		itemID, ok1 := take(idLen)
		if version >= 1 {
			_, ok1 = take(2)
		}
		_, ok2 := take(2)
		base, ok3 := take(baseSize)
		extents, ok4 := take(2)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return 0, 0, false
		}
		for range extents {
			if idxSize > 0 {
				take(idxSize)
			}
			o, okO := take(offSize)
			l, okL := take(lenSize)
			if !okO || !okL {
				return 0, 0, false
			}
			if itemID == uint64(id) {
				return base + o, l, true
			}
		}
	}
	return 0, 0, false
}
