package imageproc

import (
	"encoding/binary"
	"time"
)

// ExifInfo is what is read from an EXIF block (TIFF structure, without the "Exif" prefix).
type ExifInfo struct {
	Orientation int // 1 when absent or invalid
	TakenAt     *time.Time
}

type ifdEntry struct {
	tag, typ uint16
	value    int // offset of the value bytes in the block
}

func byteOrder(b []byte) binary.ByteOrder {
	if len(b) < 8 {
		return nil
	}
	switch {
	case string(b[:4]) == "II*\x00":
		return binary.LittleEndian
	case string(b[:4]) == "MM\x00*":
		return binary.BigEndian
	}
	return nil
}

var typeSize = map[uint16]uint32{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 7: 1, 9: 4, 10: 8}

func ifd(b []byte, o binary.ByteOrder, off uint32) []ifdEntry {
	if uint64(off)+2 > uint64(len(b)) {
		return nil
	}
	n := int(o.Uint16(b[off:]))
	var out []ifdEntry
	for i := range n {
		pos := int(off) + 2 + 12*i
		if pos+12 > len(b) {
			break
		}
		tag, typ, count := o.Uint16(b[pos:]), o.Uint16(b[pos+2:]), o.Uint32(b[pos+4:])
		value := pos + 8
		if uint64(typeSize[typ])*uint64(count) > 4 {
			value = int(o.Uint32(b[pos+8:]))
		}
		out = append(out, ifdEntry{tag: tag, typ: typ, value: value})
	}
	return out
}

func find(entries []ifdEntry, tag uint16) (ifdEntry, bool) {
	for _, e := range entries {
		if e.tag == tag {
			return e, true
		}
	}
	return ifdEntry{}, false
}

func ParseExif(b []byte) ExifInfo {
	info := ExifInfo{Orientation: 1}
	o := byteOrder(b)
	if o == nil {
		return info
	}
	root := ifd(b, o, o.Uint32(b[4:]))
	if e, ok := find(root, 0x0112); ok && e.typ == 3 && e.value+2 <= len(b) {
		if v := int(o.Uint16(b[e.value:])); v >= 1 && v <= 8 {
			info.Orientation = v
		}
	}
	sub, ok := find(root, 0x8769)
	if !ok || sub.typ != 4 || sub.value+4 > len(b) {
		return info
	}
	if e, ok := find(ifd(b, o, o.Uint32(b[sub.value:])), 0x9003); ok && e.typ == 2 && e.value+19 <= len(b) {
		if t, err := time.Parse("2006:01:02 15:04:05", string(b[e.value:e.value+19])); err == nil {
			info.TakenAt = &t
		}
	}
	return info
}

// ResetOrientation returns a copy of the block with the orientation set to 1.
func ResetOrientation(b []byte) []byte {
	out := append([]byte(nil), b...)
	o := byteOrder(out)
	if o == nil {
		return out
	}
	if e, ok := find(ifd(out, o, o.Uint32(out[4:])), 0x0112); ok && e.typ == 3 && e.value+2 <= len(out) {
		o.PutUint16(out[e.value:], 1)
	}
	return out
}
