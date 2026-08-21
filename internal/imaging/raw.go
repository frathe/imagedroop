package imaging

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
)

// RAW camera files (CR2, NEF, ARW, DNG, CR3, RAF, …) are not demosaiced
// here. They all embed a JPEG preview - typically the camera's own full-size
// review image - and that is what PicFetch displays. The TIFF IFD walk
// below is the same entry reader ReadMetadata uses; the JPEG scan is the
// fallback for ISOBMFF (CR3) and Fujifilm RAF, which are not TIFF.

const (
	tagStripOffsets          = 0x0111
	tagStripByteCounts       = 0x0117
	tagSubIFDs               = 0x014A
	tagJPEGInterchangeFormat = 0x0201
	tagJPEGInterchangeLength = 0x0202
	maxRAWIFDs               = 32
)

func isJPEG(data []byte) bool {
	return len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8
}

func isRAF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("FUJIFILM"))
}

func isISOBMFF(data []byte) bool {
	return len(data) >= 8 && string(data[4:8]) == "ftyp"
}

func tiffOrder(data []byte) (binary.ByteOrder, bool) {
	if len(data) < 8 {
		return nil, false
	}

	var bo binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return nil, false
	}

	if bo.Uint16(data[2:4]) != 0x002A {
		return nil, false
	}

	return bo, true
}

func looksLikePreviewContainer(data []byte) bool {
	_, isTIFF := tiffOrder(data)
	return isTIFF || isISOBMFF(data) || isRAF(data)
}

// embeddedJPEGPreview returns the largest valid JPEG payload embedded in a
// camera-RAW container, if any. TIFF-based RAW (CR2, NEF, ARW, DNG, …) is
// walked through its IFD chain and SubIFDs for JPEGInterchangeFormat and
// JPEG-compressed strips; CR3 and RAF fall through to a SOI/EOI scan.
func embeddedJPEGPreview(data []byte) ([]byte, bool) {
	if !looksLikePreviewContainer(data) {
		return nil, false
	}

	var found [][]byte
	if _, ok := tiffOrder(data); ok {
		found = collectTIFFJPEGs(data)
	}
	if len(found) == 0 {
		found = scanJPEGs(data)
	}

	return largestJPEG(found)
}

func collectTIFFJPEGs(data []byte) [][]byte {
	bo, ok := tiffOrder(data)
	if !ok {
		return nil
	}

	visited := make(map[uint32]bool, 8)
	var found [][]byte

	var walk func(offset uint32)
	walk = func(offset uint32) {
		if len(visited) >= maxRAWIFDs || visited[offset] {
			return
		}
		visited[offset] = true

		var jpegOff, jpegLen uint32
		var haveOff bool
		var stripOffs, stripLens, subIFDs []uint32

		walkIFD(data, bo, offset, func(tag, typ uint16, val []byte) {
			switch tag {
			case tagJPEGInterchangeFormat:
				if v, ok := uintValue(bo, typ, val); ok {
					jpegOff, haveOff = v, true
				}
			case tagJPEGInterchangeLength:
				if v, ok := uintValue(bo, typ, val); ok {
					jpegLen = v
				}
			case tagStripOffsets:
				stripOffs = uintsValue(bo, typ, val)
			case tagStripByteCounts:
				stripLens = uintsValue(bo, typ, val)
			case tagSubIFDs:
				subIFDs = uintsValue(bo, typ, val)
			}
		})

		if haveOff {
			if blob := sliceJPEG(data, jpegOff, jpegLen); blob != nil {
				found = append(found, blob)
			}
		}

		if blob := concatStrips(data, stripOffs, stripLens); isJPEG(blob) {
			found = append(found, blob)
		}

		for _, sub := range subIFDs {
			walk(sub)
		}
		if next := nextIFDOffset(data, bo, offset); next != 0 {
			walk(next)
		}
	}

	walk(bo.Uint32(data[4:8]))
	return found
}

func nextIFDOffset(tiff []byte, bo binary.ByteOrder, ifdOffset uint32) uint32 {
	if ifdOffset+2 > uint32(len(tiff)) {
		return 0
	}

	n := uint32(bo.Uint16(tiff[ifdOffset : ifdOffset+2]))
	pos := ifdOffset + 2 + n*12
	if pos+4 > uint32(len(tiff)) {
		return 0
	}

	return bo.Uint32(tiff[pos : pos+4])
}

func uintsValue(bo binary.ByteOrder, typ uint16, val []byte) []uint32 {
	switch typ {
	case 3, 8: // SHORT, SSHORT
		out := make([]uint32, 0, len(val)/2)
		for i := 0; i+2 <= len(val); i += 2 {
			out = append(out, uint32(bo.Uint16(val[i:i+2])))
		}
		return out
	case 4, 9, 13: // LONG, SLONG, IFD
		out := make([]uint32, 0, len(val)/4)
		for i := 0; i+4 <= len(val); i += 4 {
			out = append(out, bo.Uint32(val[i:i+4]))
		}
		return out
	}

	return nil
}

func sliceJPEG(data []byte, off, length uint32) []byte {
	if !haveRange(data, off, length) {
		if length == 0 {
			if uint64(off) >= uint64(len(data)) {
				return nil
			}
			n := jpegLength(data[off:])
			if n == 0 {
				return nil
			}
			return data[off : int(off)+n]
		}
		return nil
	}

	blob := data[off : off+length]
	if !isJPEG(blob) {
		return nil
	}
	return blob
}

func haveRange(data []byte, off, length uint32) bool {
	return length > 0 && uint64(off)+uint64(length) <= uint64(len(data))
}

func concatStrips(data []byte, offs, lens []uint32) []byte {
	if len(offs) == 0 {
		return nil
	}
	if len(lens) == 0 && len(offs) == 1 {
		return sliceJPEG(data, offs[0], 0)
	}
	if len(offs) != len(lens) {
		return nil
	}

	var total uint64
	for _, n := range lens {
		total += uint64(n)
		if total > 32<<20 {
			return nil
		}
	}

	out := make([]byte, 0, total)
	for i, off := range offs {
		if !haveRange(data, off, lens[i]) {
			return nil
		}
		out = append(out, data[off:off+lens[i]]...)
	}
	return out
}

// scanJPEGs finds SOI markers and takes each complete JPEG up to its EOI.
// Used for CR3 (ISOBMFF) and RAF, and as a TIFF fallback when no IFD points
// at a JPEG (some MakerNotes hide one).
func scanJPEGs(data []byte) [][]byte {
	var found [][]byte
	for i := 0; i+3 < len(data); i++ {
		if data[i] != 0xFF || data[i+1] != 0xD8 || data[i+2] != 0xFF {
			continue
		}

		n := jpegLength(data[i:])
		if n < 4 {
			continue
		}

		found = append(found, data[i:i+n])
		if len(found) >= 16 {
			break
		}
		i += n - 1
	}
	return found
}

// jpegLength returns the number of bytes from data[0] through the EOI of a
// JPEG that starts there, or 0 if the marker structure doesn't close.
func jpegLength(data []byte) int {
	if !isJPEG(data) {
		return 0
	}

	pos := 2
	for pos+1 < len(data) {
		if data[pos] != 0xFF {
			return 0
		}

		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			return 0
		}

		marker := data[pos]
		pos++

		if marker == 0xD9 { // EOI
			return pos
		}
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		if marker == 0xDA { // SOS: entropy-coded data until EOI
			return scanToEOI(data, pos)
		}

		if pos+1 >= len(data) {
			return 0
		}
		segLen := int(data[pos])<<8 | int(data[pos+1])
		if segLen < 2 || pos+segLen > len(data) {
			return 0
		}
		pos += segLen
	}

	return 0
}

func scanToEOI(data []byte, pos int) int {
	for pos+1 < len(data) {
		if data[pos] != 0xFF {
			pos++
			continue
		}

		marker := data[pos+1]
		if marker == 0x00 || marker == 0xFF {
			pos++
			continue
		}
		if marker == 0xD9 {
			return pos + 2
		}
		if marker >= 0xD0 && marker <= 0xD7 {
			pos += 2
			continue
		}
		pos++
	}
	return 0
}

func largestJPEG(blobs [][]byte) ([]byte, bool) {
	var best []byte
	var bestPixels int64

	for _, blob := range blobs {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(blob))
		if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
			continue
		}
		pixels := int64(cfg.Width) * int64(cfg.Height)
		if pixels > bestPixels || (pixels == bestPixels && len(blob) > len(best)) {
			best, bestPixels = blob, pixels
		}
	}

	if best == nil {
		return nil, false
	}
	return best, true
}

func previewOrientation(container, preview []byte) int {
	if o := readEXIFOrientation(container); o != 1 {
		return o
	}
	return readEXIFOrientation(preview)
}

// previewBounds is ReadAndProbe's fallback when image.DecodeConfig cannot
// read a RAW container: the embedded JPEG's display size, with the same
// orientation swap DecodeLoaded will apply.
func previewBounds(data []byte) (image.Rectangle, bool) {
	jpegBytes, ok := embeddedJPEGPreview(data)
	if !ok {
		return image.Rectangle{}, false
	}

	cfg, err := jpeg.DecodeConfig(bytes.NewReader(jpegBytes))
	if err != nil {
		return image.Rectangle{}, false
	}

	w, h := cfg.Width, cfg.Height
	if err := checkDimensions(w, h); err != nil {
		return image.Rectangle{}, false
	}

	if o := previewOrientation(data, jpegBytes); o >= 5 && o <= 8 {
		w, h = h, w
	}

	return image.Rect(0, 0, w, h), true
}

// decodeEmbeddedPreview finishes a load that image.Decode could not: the
// embedded JPEG, EXIF-oriented, flagged Preview so the UI can say so.
func decodeEmbeddedPreview(data []byte) (*LoadedImage, bool) {
	jpegBytes, ok := embeddedJPEGPreview(data)
	if !ok {
		return nil, false
	}

	decoded, err := jpeg.Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, false
	}

	return &LoadedImage{
		Frames:  []image.Image{ApplyOrientation(decoded, previewOrientation(data, jpegBytes))},
		Preview: true,
	}, true
}
