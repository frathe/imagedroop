package imaging

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/heic"
)

// readEXIFOrientation scans the raw JPEG bytes for an APP1 Exif segment and
// returns the orientation tag's value (1-8), or 1 (no correction needed) if
// the file has no Exif data, no orientation tag, or is malformed in a way
// that makes the tag unreadable.
func readEXIFOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}

	pos := 2

	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			break
		}

		marker := data[pos+1]

		// Markers with no payload: SOI (already consumed), restart markers,
		// and TEM. Skip straight past them.
		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD9) {
			pos += 2
			continue
		}

		if marker == 0xDA { // start of scan: header segments are over
			break
		}

		segLen := int(data[pos+2])<<8 | int(data[pos+3])

		if segLen < 2 || pos+2+segLen > len(data) {
			break
		}

		segStart := pos + 4
		segEnd := pos + 2 + segLen

		if marker == 0xE1 { // APP1
			if o := parseExifOrientation(data[segStart:segEnd]); o != 0 {
				return o
			}
		}

		pos = segEnd
	}

	return 1
}

// parseExifOrientation reads the orientation tag (0x0112) out of an APP1
// segment's payload, which starts with the "Exif\0\0" marker followed by a
// TIFF header. It returns 0 if the segment isn't Exif data or has no valid
// orientation tag.
func parseExifOrientation(seg []byte) int {
	if len(seg) < 8 || string(seg[:6]) != "Exif\x00\x00" {
		return 0
	}

	tiff := seg[6:]

	if len(tiff) < 8 {
		return 0
	}

	var bo binary.ByteOrder

	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0
	}

	if bo.Uint16(tiff[2:4]) != 0x002A {
		return 0
	}

	ifdOffset := bo.Uint32(tiff[4:8])

	if ifdOffset+2 > uint32(len(tiff)) {
		return 0
	}

	numEntries := bo.Uint16(tiff[ifdOffset : ifdOffset+2])
	entriesStart := ifdOffset + 2

	for i := uint32(0); i < uint32(numEntries); i++ {
		entryOffset := entriesStart + i*12

		if entryOffset+12 > uint32(len(tiff)) {
			break
		}

		tag := bo.Uint16(tiff[entryOffset : entryOffset+2])

		if tag != 0x0112 {
			continue
		}

		valType := bo.Uint16(tiff[entryOffset+2 : entryOffset+4])

		if valType != 3 { // SHORT
			return 0
		}

		v := bo.Uint16(tiff[entryOffset+8 : entryOffset+10])

		if v < 1 || v > 8 {
			return 0
		}

		return int(v)
	}

	return 0
}

// Metadata is the subset of a photo's Exif tags the EXIF window (see the
// root package's exif.go) displays: camera make/model, lens, exposure,
// aperture, ISO, focal length, and capture date. GPS is deliberately not
// read at all - it lives in its own sub-IFD (pointer tag 0x8825) that
// ReadMetadata never follows - so location data never even reaches this
// struct, let alone the UI. A zero Metadata (every field "") means either
// the file has no Exif data or none of these particular tags.
type Metadata struct {
	Make         string
	Model        string
	LensModel    string
	ExposureTime string
	FNumber      string
	ISO          string
	FocalLength  string
	DateTaken    string

	// DateTakenTime is DateTaken's underlying value, parsed from the same
	// raw Exif tag - for callers that need to compare or sort capture
	// dates (see the root package's captureOrModTime/CaptureDate below)
	// rather than just display DateTaken's already-formatted string. Zero
	// when DateTaken is empty, or set from a raw value that didn't parse.
	DateTakenTime time.Time
}

// Empty reports whether none of m's fields were populated - either the file
// carried no Exif segment at all, or it did but none of the tags
// ReadMetadata looks for were present.
func (m Metadata) Empty() bool {
	return m == Metadata{}
}

// ReadMetadata scans data (a whole image file's raw bytes) for an Exif APP1
// segment and extracts Metadata from it. Like readEXIFOrientation, it is
// deliberately failure-tolerant throughout: a malformed tag, a truncated
// value, or a file with no Exif data at all just leaves the corresponding
// field (or all of them) blank rather than returning an error - there is no
// error to report, only "nothing to show".
func ReadMetadata(data []byte) Metadata {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return isobmffMetadata(data)
	}

	pos := 2

	for pos+4 <= len(data) {
		if data[pos] != 0xFF {
			break
		}

		marker := data[pos+1]

		if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD9) {
			pos += 2
			continue
		}

		if marker == 0xDA {
			break
		}

		segLen := int(data[pos+2])<<8 | int(data[pos+3])

		if segLen < 2 || pos+2+segLen > len(data) {
			break
		}

		segStart := pos + 4
		segEnd := pos + 2 + segLen

		if marker == 0xE1 {
			seg := data[segStart:segEnd]
			if len(seg) >= 8 && string(seg[:6]) == "Exif\x00\x00" {
				if m := parseExifMetadata(seg[6:]); !m.Empty() {
					return m
				}
			}
		}

		pos = segEnd
	}

	return Metadata{}
}

// isobmffMetadata reads Exif metadata out of an ISOBMFF-boxed file (HEIC or
// AVIF) - readEXIFOrientation and the JPEG walk above only understand the
// APP1 container, so anything that fails that signature check lands here
// instead. Both DecodeExif calls fail fast (pure box-walking, no wasm/cgo
// invocation) on a file that isn't theirs, so trying heic then avif costs
// nothing extra for the common case of a format with no Exif at all (PNG,
// GIF, WebP, BMP, TIFF, ICO, XPM).
func isobmffMetadata(data []byte) Metadata {
	if ex, err := heic.DecodeExif(bytes.NewReader(data)); err == nil {
		return metadataFromISOBMFFExif(ex.Make, ex.Model, ex.ExposureTime, ex.FNumber, ex.ISOSpeed, ex.FocalLength, ex.DateTimeOriginal, ex.DateTime)
	}

	if ex, err := avif.DecodeExif(bytes.NewReader(data)); err == nil {
		return metadataFromISOBMFFExif(ex.Make, ex.Model, ex.ExposureTime, ex.FNumber, ex.ISOSpeed, ex.FocalLength, ex.DateTimeOriginal, ex.DateTime)
	}

	return Metadata{}
}

// metadataFromISOBMFFExif adapts the fields heic.Exif and avif.Exif share
// (the two packages expose identically-shaped structs) into Metadata,
// reusing the same formatting helpers the JPEG APP1 walk uses so a HEIC/AVIF
// photo's EXIF window reads the same as a JPEG's. LensModel has no
// equivalent in either struct, so it's left unset, same as a JPEG missing
// that tag.
func metadataFromISOBMFFExif(cameraMake, model string, exposureTime, fNumber float64, iso int, focalLength float64, dateTimeOriginal, dateTime string) Metadata {
	m := Metadata{Make: cameraMake, Model: model}

	if exposureTime > 0 {
		m.ExposureTime = formatExposureTime(exposureTime)
	}
	if fNumber > 0 {
		m.FNumber = fmt.Sprintf("f/%.1f", fNumber)
	}
	if iso > 0 {
		m.ISO = fmt.Sprintf("ISO %d", iso)
	}
	if focalLength > 0 {
		m.FocalLength = formatFocalLength(focalLength)
	}

	if dateTimeOriginal != "" {
		m.DateTaken = formatExifDate(dateTimeOriginal)
		if t, ok := parseExifDateTime(dateTimeOriginal); ok {
			m.DateTakenTime = t
		}
	} else if dateTime != "" {
		m.DateTaken = formatExifDate(dateTime)
		if t, ok := parseExifDateTime(dateTime); ok {
			m.DateTakenTime = t
		}
	}

	return m
}

// exifIFDPointer (0x8769) locates the Exif SubIFD, which - unlike IFD0's
// camera make/model/date - holds the shooting parameters (exposure,
// aperture, ISO, focal length, lens, and the more specific
// DateTimeOriginal).
const exifIFDPointer = 0x8769

// parseExifMetadata reads tiff - the TIFF header and IFDs following the
// "Exif\x00\x00" marker, same payload parseExifOrientation works from - and
// walks IFD0 plus the Exif SubIFD it points to for the tags Metadata cares
// about.
func parseExifMetadata(tiff []byte) Metadata {
	var m Metadata

	if len(tiff) < 8 {
		return m
	}

	var bo binary.ByteOrder

	switch string(tiff[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return m
	}

	if bo.Uint16(tiff[2:4]) != 0x002A {
		return m
	}

	ifd0Offset := bo.Uint32(tiff[4:8])

	var exifIFDOffset uint32
	haveExifIFD := false

	walkIFD(tiff, bo, ifd0Offset, func(tag, typ uint16, val []byte) {
		switch tag {
		case 0x010F: // Make
			if s, ok := asciiValue(typ, val); ok {
				m.Make = s
			}
		case 0x0110: // Model
			if s, ok := asciiValue(typ, val); ok {
				m.Model = s
			}
		case 0x0132: // DateTime - fallback if the Exif SubIFD has no
			// DateTimeOriginal (0x9003), which is preferred where present.
			if s, ok := asciiValue(typ, val); ok {
				m.DateTaken = formatExifDate(s)
				if t, ok := parseExifDateTime(s); ok {
					m.DateTakenTime = t
				}
			}
		case exifIFDPointer:
			if v, ok := uintValue(bo, typ, val); ok {
				exifIFDOffset = v
				haveExifIFD = true
			}
		}
	})

	if !haveExifIFD {
		return m
	}

	walkIFD(tiff, bo, exifIFDOffset, func(tag, typ uint16, val []byte) {
		switch tag {
		case 0x829A: // ExposureTime
			if r, ok := rationalValue(bo, typ, val); ok && r > 0 {
				m.ExposureTime = formatExposureTime(r)
			}
		case 0x829D: // FNumber
			if r, ok := rationalValue(bo, typ, val); ok && r > 0 {
				m.FNumber = fmt.Sprintf("f/%.1f", r)
			}
		case 0x8827: // PhotographicSensitivity (ISO)
			if v, ok := uintValue(bo, typ, val); ok {
				m.ISO = fmt.Sprintf("ISO %d", v)
			}
		case 0x920A: // FocalLength
			if r, ok := rationalValue(bo, typ, val); ok && r > 0 {
				m.FocalLength = formatFocalLength(r)
			}
		case 0x9003: // DateTimeOriginal - takes priority over IFD0's DateTime
			if s, ok := asciiValue(typ, val); ok {
				m.DateTaken = formatExifDate(s)
				if t, ok := parseExifDateTime(s); ok {
					m.DateTakenTime = t
				}
			}
		case 0xA434: // LensModel
			if s, ok := asciiValue(typ, val); ok {
				m.LensModel = s
			}
		}
	})

	return m
}

// walkIFD calls fn once per readable entry in the IFD at ifdOffset within
// tiff. Entries with an unrecognized type, an implausible count, or a
// value/offset that doesn't fit inside tiff are silently skipped rather
// than reported - see ReadMetadata's comment on why that's the right
// failure mode here.
func walkIFD(tiff []byte, bo binary.ByteOrder, ifdOffset uint32, fn func(tag, typ uint16, val []byte)) {
	if ifdOffset+2 > uint32(len(tiff)) {
		return
	}

	numEntries := bo.Uint16(tiff[ifdOffset : ifdOffset+2])
	entriesStart := ifdOffset + 2

	for i := uint32(0); i < uint32(numEntries); i++ {
		entryOffset := entriesStart + i*12

		if entryOffset+12 > uint32(len(tiff)) {
			break
		}

		tag := bo.Uint16(tiff[entryOffset : entryOffset+2])
		typ := bo.Uint16(tiff[entryOffset+2 : entryOffset+4])
		count := bo.Uint32(tiff[entryOffset+4 : entryOffset+8])

		size := tagComponentSize(typ)
		// A count this large is either a corrupt file or a hostile one -
		// either way the tags this reader looks for are all single values
		// or short strings, so anything past a generous cap is skipped
		// rather than trusted enough to compute a byte length from.
		if size == 0 || count == 0 || count > 1<<16 {
			continue
		}

		total := uint64(size) * uint64(count)
		if total > 1<<20 {
			continue
		}

		var val []byte
		if total <= 4 {
			val = tiff[entryOffset+8 : uint64(entryOffset)+8+total]
		} else {
			offset := bo.Uint32(tiff[entryOffset+8 : entryOffset+12])
			if uint64(offset)+total > uint64(len(tiff)) {
				continue
			}
			val = tiff[offset : uint64(offset)+total]
		}

		fn(tag, typ, val)
	}
}

// tagComponentSize returns the byte size of one component of Exif type typ,
// or 0 for a type this reader doesn't know how to decode.
func tagComponentSize(typ uint16) int {
	switch typ {
	case 1, 2, 6, 7: // BYTE, ASCII, SBYTE, UNDEFINED
		return 1
	case 3, 8: // SHORT, SSHORT
		return 2
	case 4, 9: // LONG, SLONG
		return 4
	case 5, 10: // RATIONAL, SRATIONAL
		return 8
	default:
		return 0
	}
}

// asciiValue decodes val as an Exif ASCII value (type 2): NUL-terminated,
// often with trailing padding. Returns ok=false for a wrong type or a
// value that's empty once trimmed, so callers can just skip setting the
// field.
func asciiValue(typ uint16, val []byte) (string, bool) {
	if typ != 2 {
		return "", false
	}

	s := strings.TrimRight(string(val), "\x00")
	s = strings.TrimSpace(s)

	if s == "" {
		return "", false
	}

	return s, true
}

// uintValue decodes val as an unsigned integer from a SHORT or LONG entry
// (the two Exif types this reader treats as plain counts: ISO and the
// Exif SubIFD pointer).
func uintValue(bo binary.ByteOrder, typ uint16, val []byte) (uint32, bool) {
	switch typ {
	case 3: // SHORT
		if len(val) < 2 {
			return 0, false
		}
		return uint32(bo.Uint16(val[:2])), true
	case 4: // LONG
		if len(val) < 4 {
			return 0, false
		}
		return bo.Uint32(val[:4]), true
	}

	return 0, false
}

// rationalValue decodes val as an unsigned RATIONAL (type 5: a numerator and
// denominator, each a LONG) - the type Exif uses for exposure time,
// aperture, and focal length. ok is false for a wrong type, a truncated
// value, or a zero denominator.
func rationalValue(bo binary.ByteOrder, typ uint16, val []byte) (float64, bool) {
	if typ != 5 || len(val) < 8 {
		return 0, false
	}

	num := bo.Uint32(val[0:4])
	den := bo.Uint32(val[4:8])

	if den == 0 {
		return 0, false
	}

	return float64(num) / float64(den), true
}

// formatExposureTime renders a shutter speed in seconds as Exif-style
// display text: "1/200 s" for anything faster than a second (the common
// case), or "2.5 s" for a full second or slower (long exposures).
func formatExposureTime(seconds float64) string {
	if seconds >= 1 {
		return fmt.Sprintf("%.1f s", seconds)
	}

	denominator := math.Round(1 / seconds)

	return fmt.Sprintf("1/%d s", int64(denominator))
}

// formatFocalLength renders a focal length in millimeters, dropping the
// decimal point for the common whole-number case.
func formatFocalLength(mm float64) string {
	if mm == math.Trunc(mm) {
		return fmt.Sprintf("%.0f mm", mm)
	}

	return fmt.Sprintf("%.1f mm", mm)
}

// formatExifDate reformats Exif's "YYYY:MM:DD HH:MM:SS" date/time encoding
// (colons instead of dashes in the date, so it doubles as a valid bare
// filename component on every OS) into the more readable
// "YYYY-MM-DD HH:MM:SS". Anything not matching that exact shape is passed
// through unchanged rather than discarded - still useful to show even if
// this reader doesn't recognize its layout.
func formatExifDate(raw string) string {
	if len(raw) == 19 && raw[4] == ':' && raw[7] == ':' {
		return raw[:4] + "-" + raw[5:7] + "-" + raw[8:]
	}

	return raw
}

// parseExifDateTime parses raw - the same "YYYY:MM:DD HH:MM:SS" Exif
// encoding formatExifDate reformats for display - into a time.Time. ok is
// false for anything not matching that exact layout, mirroring
// formatExifDate's own tolerant fallback (pass the raw string through
// unchanged rather than erroring). Interpreted in the local zone: Exif
// carries no timezone offset in the tags this reader looks at, so that's
// the best available guess, same as most photo software assumes.
func parseExifDateTime(raw string) (time.Time, bool) {
	t, err := time.ParseInLocation("2006:01:02 15:04:05", raw, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
