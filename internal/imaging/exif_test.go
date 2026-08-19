package imaging

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// approx reports whether two decimal degrees are equal to within a
// millionth of a degree (about 10 cm) - the DMS-to-degrees conversion is
// exact only for whole seconds, so coordinate assertions compare with a
// tolerance rather than for equality.
func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// buildExifSegment builds the payload of an APP1 Exif segment (starting with
// the "Exif\0\0" marker) that declares a single orientation tag.
func buildExifSegment(t *testing.T, orientation uint16, bigEndian bool) []byte {
	t.Helper()

	var bo binary.ByteOrder
	var byteOrderMark []byte

	if bigEndian {
		bo = binary.BigEndian
		byteOrderMark = []byte("MM")
	} else {
		bo = binary.LittleEndian
		byteOrderMark = []byte("II")
	}

	buf := new(bytes.Buffer)
	buf.WriteString("Exif\x00\x00")
	buf.Write(byteOrderMark)

	u16 := func(v uint16) []byte {
		b := make([]byte, 2)
		bo.PutUint16(b, v)
		return b
	}
	u32 := func(v uint32) []byte {
		b := make([]byte, 4)
		bo.PutUint32(b, v)
		return b
	}

	buf.Write(u16(0x002A)) // TIFF magic
	buf.Write(u32(8))      // IFD0 offset, right after this header
	buf.Write(u16(1))      // one entry

	buf.Write(u16(0x0112)) // Orientation tag
	buf.Write(u16(3))      // type SHORT
	buf.Write(u32(1))      // count 1

	value := make([]byte, 4)
	bo.PutUint16(value, orientation)
	buf.Write(value)

	buf.Write(u32(0)) // next IFD offset

	return buf.Bytes()
}

// wrapAsAPP1 wraps a segment payload in an FF E1 marker with the correct
// length prefix.
func wrapAsAPP1(payload []byte) []byte {
	length := len(payload) + 2
	return append([]byte{0xFF, 0xE1, byte(length >> 8), byte(length)}, payload...)
}

func TestParseExifOrientation(t *testing.T) {
	t.Run("little endian", func(t *testing.T) {
		seg := buildExifSegment(t, 6, false)
		if got := parseExifOrientation(seg); got != 6 {
			t.Errorf("parseExifOrientation() = %d, want 6", got)
		}
	})

	t.Run("big endian", func(t *testing.T) {
		seg := buildExifSegment(t, 8, true)
		if got := parseExifOrientation(seg); got != 8 {
			t.Errorf("parseExifOrientation() = %d, want 8", got)
		}
	})

	t.Run("not an Exif segment", func(t *testing.T) {
		if got := parseExifOrientation([]byte("JFIF\x00\x00 not exif at all")); got != 0 {
			t.Errorf("parseExifOrientation() = %d, want 0", got)
		}
	})

	t.Run("too short", func(t *testing.T) {
		if got := parseExifOrientation([]byte("Exif")); got != 0 {
			t.Errorf("parseExifOrientation() = %d, want 0", got)
		}
	})

	t.Run("out of range value falls back to no correction", func(t *testing.T) {
		seg := buildExifSegment(t, 42, false)
		if got := parseExifOrientation(seg); got != 0 {
			t.Errorf("parseExifOrientation() = %d, want 0", got)
		}
	})
}

// fullExifFields is the set of tags buildFullExifTIFF writes, and the
// values TestReadMetadata_FullTagSet expects back out.
type fullExifFields struct {
	make, model, lensModel   string
	dateTimeOriginal         string
	exposureNum, exposureDen uint32
	fNumberNum, fNumberDen   uint32
	iso                      uint16
	focalNum, focalDen       uint32
}

// buildFullExifTIFF builds a little-endian TIFF payload (everything after
// the "Exif\x00\x00" marker) with an IFD0 (Make, Model, the Exif SubIFD
// pointer) and an Exif SubIFD (ExposureTime, FNumber, ISO, FocalLength,
// DateTimeOriginal, LensModel) - the tag set ReadMetadata looks for. Unlike
// buildExifSegment (a single inline SHORT tag), several of these values are
// too large to fit in an entry's 4-byte value field and need a trailing
// "value area" that later entries' offsets point into; this builder lays
// that out by hand, appending each oversized value's bytes and offset as it
// goes.
func buildFullExifTIFF(t *testing.T, f fullExifFields) []byte {
	t.Helper()

	bo := binary.LittleEndian

	u16 := func(v uint16) []byte { b := make([]byte, 2); bo.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); bo.PutUint32(b, v); return b }
	asciiZ := func(s string) []byte { return append([]byte(s), 0) }
	rational := func(num, den uint32) []byte { return append(u32(num), u32(den)...) }

	const (
		ifd0EntryCount = 3
		ifd1EntryCount = 6
	)

	ifd0Size := 2 + ifd0EntryCount*12 + 4
	ifd1Size := 2 + ifd1EntryCount*12 + 4

	const headerSize = 8 // "II" + magic + IFD0 offset
	ifd0Offset := uint32(headerSize)
	ifd1Offset := ifd0Offset + uint32(ifd0Size)
	valueAreaStart := ifd1Offset + uint32(ifd1Size)

	// Lay out each oversized value in the value area, in the order it's
	// referenced below, tracking each one's absolute offset as it's placed.
	var valueArea []byte
	place := func(b []byte) uint32 {
		offset := valueAreaStart + uint32(len(valueArea))
		valueArea = append(valueArea, b...)
		return offset
	}

	makeBytes := asciiZ(f.make)
	modelBytes := asciiZ(f.model)
	makeOffset := place(makeBytes)
	modelOffset := place(modelBytes)

	exposureOffset := place(rational(f.exposureNum, f.exposureDen))
	fNumberOffset := place(rational(f.fNumberNum, f.fNumberDen))
	focalOffset := place(rational(f.focalNum, f.focalDen))
	dateTimeBytes := asciiZ(f.dateTimeOriginal)
	dateTimeOffset := place(dateTimeBytes)
	lensBytes := asciiZ(f.lensModel)
	lensOffset := place(lensBytes)

	buf := new(bytes.Buffer)
	buf.WriteString("II")
	buf.Write(u16(0x002A))
	buf.Write(u32(ifd0Offset))

	// IFD0: Make, Model, ExifIFDPointer (LONG, fits inline).
	buf.Write(u16(ifd0EntryCount))

	buf.Write(u16(0x010F)) // Make
	buf.Write(u16(2))      // ASCII
	buf.Write(u32(uint32(len(makeBytes))))
	buf.Write(u32(makeOffset))

	buf.Write(u16(0x0110)) // Model
	buf.Write(u16(2))
	buf.Write(u32(uint32(len(modelBytes))))
	buf.Write(u32(modelOffset))

	buf.Write(u16(0x8769)) // ExifIFDPointer
	buf.Write(u16(4))      // LONG
	buf.Write(u32(1))
	buf.Write(u32(ifd1Offset))

	buf.Write(u32(0)) // IFD0 next-IFD offset

	if buf.Len() != int(ifd1Offset) {
		t.Fatalf("IFD0 layout mismatch: wrote %d bytes, want %d", buf.Len(), ifd1Offset)
	}

	// Exif SubIFD: ExposureTime, FNumber, ISO, FocalLength,
	// DateTimeOriginal, LensModel.
	buf.Write(u16(ifd1EntryCount))

	buf.Write(u16(0x829A)) // ExposureTime
	buf.Write(u16(5))      // RATIONAL
	buf.Write(u32(1))
	buf.Write(u32(exposureOffset))

	buf.Write(u16(0x829D)) // FNumber
	buf.Write(u16(5))
	buf.Write(u32(1))
	buf.Write(u32(fNumberOffset))

	buf.Write(u16(0x8827)) // ISO (PhotographicSensitivity)
	buf.Write(u16(3))      // SHORT
	buf.Write(u32(1))
	value := make([]byte, 4)
	bo.PutUint16(value, f.iso)
	buf.Write(value)

	buf.Write(u16(0x920A)) // FocalLength
	buf.Write(u16(5))
	buf.Write(u32(1))
	buf.Write(u32(focalOffset))

	buf.Write(u16(0x9003)) // DateTimeOriginal
	buf.Write(u16(2))
	buf.Write(u32(uint32(len(dateTimeBytes))))
	buf.Write(u32(dateTimeOffset))

	buf.Write(u16(0xA434)) // LensModel
	buf.Write(u16(2))
	buf.Write(u32(uint32(len(lensBytes))))
	buf.Write(u32(lensOffset))

	buf.Write(u32(0)) // Exif SubIFD next-IFD offset

	if buf.Len() != int(valueAreaStart) {
		t.Fatalf("Exif SubIFD layout mismatch: wrote %d bytes, want %d", buf.Len(), valueAreaStart)
	}

	buf.Write(valueArea)

	return buf.Bytes()
}

func TestReadMetadata_FullTagSet(t *testing.T) {
	tiff := buildFullExifTIFF(t, fullExifFields{
		make:             "Canon",
		model:            "EOS 90D",
		lensModel:        "EF50mm f/1.8",
		dateTimeOriginal: "2024:08:12 14:33:02",
		exposureNum:      1, exposureDen: 200,
		fNumberNum: 28, fNumberDen: 10,
		iso:      400,
		focalNum: 50, focalDen: 1,
	})

	seg := append([]byte("Exif\x00\x00"), tiff...)
	data := append([]byte{0xFF, 0xD8}, wrapAsAPP1(seg)...)

	m := ReadMetadata(data)

	want := Metadata{
		Make:          "Canon",
		Model:         "EOS 90D",
		LensModel:     "EF50mm f/1.8",
		ExposureTime:  "1/200 s",
		FNumber:       "f/2.8",
		ISO:           "ISO 400",
		FocalLength:   "50 mm",
		DateTaken:     "2024-08-12 14:33:02",
		DateTakenTime: time.Date(2024, 8, 12, 14, 33, 2, 0, time.Local),
	}

	if m != want {
		t.Errorf("ReadMetadata() = %+v, want %+v", m, want)
	}
}

// gpsFields is the raw GPS sub-IFD content buildGPSExifTIFF writes: the two
// hemisphere reference strings and the two degrees/minutes/seconds triples,
// each component a numerator/denominator pair so a test can write a
// fractional (or deliberately broken, zero-denominator) value.
type gpsFields struct {
	latRef, lonRef string
	lat, lon       [3][2]uint32

	// omitLatRef and omitLon drop a tag entirely, for the partial-IFD cases
	// where the coordinate can't be resolved.
	omitLatRef, omitLon bool
}

// buildGPSExifTIFF builds a little-endian TIFF payload whose IFD0 holds
// nothing but the GPS sub-IFD pointer (0x8825), plus that sub-IFD. Refs are
// two bytes and so live inline in their entries; the DMS triples are 24
// bytes each and need the trailing value area.
func buildGPSExifTIFF(t *testing.T, f gpsFields) []byte {
	t.Helper()

	bo := binary.LittleEndian

	u16 := func(v uint16) []byte { b := make([]byte, 2); bo.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); bo.PutUint32(b, v); return b }

	dms := func(v [3][2]uint32) []byte {
		var b []byte
		for _, pair := range v {
			b = append(b, u32(pair[0])...)
			b = append(b, u32(pair[1])...)
		}
		return b
	}

	gpsEntryCount := 4
	if f.omitLatRef {
		gpsEntryCount--
	}
	if f.omitLon {
		gpsEntryCount -= 2
	}

	const headerSize = 8
	ifd0Offset := uint32(headerSize)
	ifd0Size := 2 + 1*12 + 4
	gpsOffset := ifd0Offset + uint32(ifd0Size)
	gpsSize := 2 + gpsEntryCount*12 + 4
	valueAreaStart := gpsOffset + uint32(gpsSize)

	var valueArea []byte
	place := func(b []byte) uint32 {
		offset := valueAreaStart + uint32(len(valueArea))
		valueArea = append(valueArea, b...)
		return offset
	}

	latOffset := place(dms(f.lat))
	lonOffset := place(dms(f.lon))

	inlineASCII := func(s string) []byte {
		b := make([]byte, 4)
		copy(b, s)
		return b
	}

	buf := new(bytes.Buffer)
	buf.WriteString("II")
	buf.Write(u16(0x002A))
	buf.Write(u32(ifd0Offset))

	buf.Write(u16(1))
	buf.Write(u16(0x8825)) // GPSIFDPointer
	buf.Write(u16(4))      // LONG
	buf.Write(u32(1))
	buf.Write(u32(gpsOffset))
	buf.Write(u32(0))

	if buf.Len() != int(gpsOffset) {
		t.Fatalf("IFD0 layout mismatch: wrote %d bytes, want %d", buf.Len(), gpsOffset)
	}

	buf.Write(u16(uint16(gpsEntryCount)))

	if !f.omitLatRef {
		buf.Write(u16(0x0001)) // GPSLatitudeRef
		buf.Write(u16(2))      // ASCII
		buf.Write(u32(2))
		buf.Write(inlineASCII(f.latRef))
	}

	buf.Write(u16(0x0002)) // GPSLatitude
	buf.Write(u16(5))      // RATIONAL
	buf.Write(u32(3))
	buf.Write(u32(latOffset))

	if !f.omitLon {
		buf.Write(u16(0x0003)) // GPSLongitudeRef
		buf.Write(u16(2))
		buf.Write(u32(2))
		buf.Write(inlineASCII(f.lonRef))

		buf.Write(u16(0x0004)) // GPSLongitude
		buf.Write(u16(5))
		buf.Write(u32(3))
		buf.Write(u32(lonOffset))
	}

	buf.Write(u32(0))

	if buf.Len() != int(valueAreaStart) {
		t.Fatalf("GPS IFD layout mismatch: wrote %d bytes, want %d", buf.Len(), valueAreaStart)
	}

	buf.Write(valueArea)

	return buf.Bytes()
}

// gpsJPEG wraps a GPS TIFF payload in the APP1 segment and JPEG SOI marker
// ReadMetadata expects to walk.
func gpsJPEG(t *testing.T, f gpsFields) []byte {
	t.Helper()

	seg := append([]byte("Exif\x00\x00"), buildGPSExifTIFF(t, f)...)

	return append([]byte{0xFF, 0xD8}, wrapAsAPP1(seg)...)
}

func TestReadMetadata_GPS(t *testing.T) {
	// 48° 51' 29.6" N, 2° 17' 40.2" E - the Eiffel Tower, with the seconds
	// written as hundredths so the rational denominators aren't all 1.
	m := ReadMetadata(gpsJPEG(t, gpsFields{
		latRef: "N", lat: [3][2]uint32{{48, 1}, {51, 1}, {2960, 100}},
		lonRef: "E", lon: [3][2]uint32{{2, 1}, {17, 1}, {4020, 100}},
	}))

	if !m.HasGPS {
		t.Fatalf("ReadMetadata() = %+v, want a GPS position", m)
	}

	if !approx(m.Latitude, 48.858222) || !approx(m.Longitude, 2.294500) {
		t.Errorf("position = (%v, %v), want approximately (48.858222, 2.294500)", m.Latitude, m.Longitude)
	}
}

func TestReadMetadata_GPSSouthWestIsNegative(t *testing.T) {
	m := ReadMetadata(gpsJPEG(t, gpsFields{
		latRef: "S", lat: [3][2]uint32{{33, 1}, {51, 1}, {31, 1}},
		lonRef: "W", lon: [3][2]uint32{{70, 1}, {39, 1}, {0, 1}},
	}))

	if !m.HasGPS {
		t.Fatalf("ReadMetadata() = %+v, want a GPS position", m)
	}

	if m.Latitude >= 0 || m.Longitude >= 0 {
		t.Errorf("position = (%v, %v), want both negative for S/W", m.Latitude, m.Longitude)
	}

	if !approx(m.Latitude, -33.858611) || !approx(m.Longitude, -70.650000) {
		t.Errorf("position = (%v, %v), want approximately (-33.858611, -70.650000)", m.Latitude, m.Longitude)
	}
}

func TestReadMetadata_GPSLowercaseRef(t *testing.T) {
	// Some writers emit a lowercase hemisphere ref; it means the same thing.
	m := ReadMetadata(gpsJPEG(t, gpsFields{
		latRef: "s", lat: [3][2]uint32{{10, 1}, {0, 1}, {0, 1}},
		lonRef: "w", lon: [3][2]uint32{{20, 1}, {0, 1}, {0, 1}},
	}))

	if !m.HasGPS || m.Latitude != -10 || m.Longitude != -20 {
		t.Errorf("ReadMetadata() = %+v, want (-10, -20) with HasGPS", m)
	}
}

func TestReadMetadata_GPSZeroIslandIsStillAPosition(t *testing.T) {
	// Unlike the HEIC/AVIF path, which can't tell (0, 0) from "no tags", an
	// explicit all-zero JPEG GPS IFD is a real - if unlikely - position.
	m := ReadMetadata(gpsJPEG(t, gpsFields{
		latRef: "N", lat: [3][2]uint32{{0, 1}, {0, 1}, {0, 1}},
		lonRef: "E", lon: [3][2]uint32{{0, 1}, {0, 1}, {0, 1}},
	}))

	if !m.HasGPS || m.Latitude != 0 || m.Longitude != 0 {
		t.Errorf("ReadMetadata() = %+v, want (0, 0) with HasGPS", m)
	}
}

func TestReadMetadata_GPSRejected(t *testing.T) {
	cases := []struct {
		name string
		f    gpsFields
	}{
		{"missing latitude ref", gpsFields{
			lonRef: "E", lon: [3][2]uint32{{2, 1}, {0, 1}, {0, 1}},
			lat: [3][2]uint32{{48, 1}, {0, 1}, {0, 1}}, omitLatRef: true,
		}},
		{"missing longitude", gpsFields{
			latRef: "N", lat: [3][2]uint32{{48, 1}, {0, 1}, {0, 1}}, omitLon: true,
		}},
		{"unknown hemisphere ref", gpsFields{
			latRef: "X", lat: [3][2]uint32{{48, 1}, {0, 1}, {0, 1}},
			lonRef: "E", lon: [3][2]uint32{{2, 1}, {0, 1}, {0, 1}},
		}},
		{"latitude ref on the longitude axis", gpsFields{
			latRef: "N", lat: [3][2]uint32{{48, 1}, {0, 1}, {0, 1}},
			lonRef: "N", lon: [3][2]uint32{{2, 1}, {0, 1}, {0, 1}},
		}},
		{"zero denominator", gpsFields{
			latRef: "N", lat: [3][2]uint32{{48, 0}, {0, 1}, {0, 1}},
			lonRef: "E", lon: [3][2]uint32{{2, 1}, {0, 1}, {0, 1}},
		}},
		{"latitude out of range", gpsFields{
			latRef: "N", lat: [3][2]uint32{{91, 1}, {0, 1}, {0, 1}},
			lonRef: "E", lon: [3][2]uint32{{2, 1}, {0, 1}, {0, 1}},
		}},
		{"longitude out of range", gpsFields{
			latRef: "N", lat: [3][2]uint32{{48, 1}, {0, 1}, {0, 1}},
			lonRef: "E", lon: [3][2]uint32{{181, 1}, {0, 1}, {0, 1}},
		}},
		{"minutes push latitude past the pole", gpsFields{
			latRef: "N", lat: [3][2]uint32{{90, 1}, {1, 1}, {0, 1}},
			lonRef: "E", lon: [3][2]uint32{{2, 1}, {0, 1}, {0, 1}},
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := ReadMetadata(gpsJPEG(t, c.f))

			if m.HasGPS {
				t.Errorf("ReadMetadata() = %+v, want no GPS position", m)
			}
		})
	}
}

func TestReadMetadata_GPSExactlyAtTheRangeEdges(t *testing.T) {
	m := ReadMetadata(gpsJPEG(t, gpsFields{
		latRef: "S", lat: [3][2]uint32{{90, 1}, {0, 1}, {0, 1}},
		lonRef: "W", lon: [3][2]uint32{{180, 1}, {0, 1}, {0, 1}},
	}))

	if !m.HasGPS || m.Latitude != -90 || m.Longitude != -180 {
		t.Errorf("ReadMetadata() = %+v, want the in-range edge (-90, -180)", m)
	}
}

func TestReadMetadata_NoGPSIFDLeavesThePositionUnset(t *testing.T) {
	tiff := buildFullExifTIFF(t, fullExifFields{
		make: "Canon", model: "EOS 90D", lensModel: "EF50mm f/1.8",
		dateTimeOriginal: "2024:08:12 14:33:02",
		exposureNum:      1, exposureDen: 200,
		fNumberNum: 28, fNumberDen: 10,
		iso:      400,
		focalNum: 50, focalDen: 1,
	})

	seg := append([]byte("Exif\x00\x00"), tiff...)

	if m := ReadMetadata(append([]byte{0xFF, 0xD8}, wrapAsAPP1(seg)...)); m.HasGPS {
		t.Errorf("ReadMetadata() = %+v, want no GPS position", m)
	}
}

func TestDegreesFromDMS(t *testing.T) {
	cases := []struct {
		name   string
		dms    []float64
		ref    string
		want   float64
		wantOK bool
	}{
		{name: "north", dms: []float64{48, 30, 36}, ref: "N", want: 48.51, wantOK: true},
		{name: "south negates", dms: []float64{48, 30, 36}, ref: "S", want: -48.51, wantOK: true},
		{name: "zero", dms: []float64{0, 0, 0}, ref: "N", want: 0, wantOK: true},
		{name: "nil triple", dms: nil, ref: "N"},
		{name: "short triple", dms: []float64{48, 30}, ref: "N"},
		{name: "long triple", dms: []float64{48, 30, 36, 1}, ref: "N"},
		{name: "empty ref", dms: []float64{48, 30, 36}, ref: ""},
		{name: "wrong axis ref", dms: []float64{48, 30, 36}, ref: "E"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := degreesFromDMS(c.dms, c.ref, "N", "S")

			if ok != c.wantOK {
				t.Fatalf("degreesFromDMS() ok = %v, want %v", ok, c.wantOK)
			}

			if ok && !approx(got, c.want) {
				t.Errorf("degreesFromDMS() = %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidCoordinates(t *testing.T) {
	cases := []struct {
		name     string
		lat, lon float64
		want     bool
	}{
		{"origin", 0, 0, true},
		{"north-east edge", 90, 180, true},
		{"south-west edge", -90, -180, true},
		{"just past the pole", 90.000001, 0, false},
		{"just past the antimeridian", 0, 180.000001, false},
		{"just inside the pole", 89.999999, 0, true},
		{"NaN latitude", math.NaN(), 0, false},
		{"NaN longitude", 0, math.NaN(), false},
		{"infinite latitude", math.Inf(1), 0, false},
		{"negative infinite longitude", 0, math.Inf(-1), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := validCoordinates(c.lat, c.lon); got != c.want {
				t.Errorf("validCoordinates(%v, %v) = %v, want %v", c.lat, c.lon, got, c.want)
			}
		})
	}
}

func TestRationalsValue(t *testing.T) {
	bo := binary.LittleEndian

	rationals := func(pairs ...uint32) []byte {
		b := make([]byte, 0, len(pairs)*4)
		for _, v := range pairs {
			b = binary.LittleEndian.AppendUint32(b, v)
		}
		return b
	}

	t.Run("three rationals", func(t *testing.T) {
		got, ok := rationalsValue(bo, 5, rationals(1, 2, 3, 4, 10, 4), 3)

		if !ok || len(got) != 3 || got[0] != 0.5 || got[1] != 0.75 || got[2] != 2.5 {
			t.Errorf("rationalsValue() = %v, %v, want [0.5 0.75 2.5], true", got, ok)
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		if _, ok := rationalsValue(bo, 4, rationals(1, 2, 3, 4, 5, 6), 3); ok {
			t.Error("rationalsValue() ok = true for a LONG entry, want false")
		}
	})

	t.Run("truncated", func(t *testing.T) {
		if _, ok := rationalsValue(bo, 5, rationals(1, 2, 3, 4), 3); ok {
			t.Error("rationalsValue() ok = true for two rationals, want false")
		}
	})

	t.Run("zero denominator", func(t *testing.T) {
		if _, ok := rationalsValue(bo, 5, rationals(1, 0, 3, 4, 5, 6), 3); ok {
			t.Error("rationalsValue() ok = true for a zero denominator, want false")
		}
	})
}

func TestReadMetadata_NoExifData(t *testing.T) {
	data := encodeJPEG(t, 4, 4, color.White)

	m := ReadMetadata(data)

	if !m.Empty() {
		t.Errorf("ReadMetadata() = %+v, want empty", m)
	}
}

func TestReadMetadata_NotAJPEG(t *testing.T) {
	m := ReadMetadata([]byte("not a jpeg"))

	if !m.Empty() {
		t.Errorf("ReadMetadata() = %+v, want empty", m)
	}
}

func TestReadMetadata_OrientationOnlySegmentYieldsEmpty(t *testing.T) {
	// A real-world file whose only Exif tag is orientation (buildExifSegment,
	// used throughout this file) has no Make/Model/etc to find - ReadMetadata
	// should come back empty, not error or panic.
	seg := wrapAsAPP1(buildExifSegment(t, 6, false))
	data := append([]byte{0xFF, 0xD8}, seg...)

	m := ReadMetadata(data)

	if !m.Empty() {
		t.Errorf("ReadMetadata() = %+v, want empty", m)
	}
}

// wantISOBMFFTestFixtureMetadata is what both testdata/test_exif.heic and
// testdata/test_exif.avif carry - the two files were built (by the
// gen2brain/heic and gen2brain/avif projects, whose testdata this repo's
// fixtures are copied from) with the same Make/Model/FNumber/ISO values, no
// ExposureTime, FocalLength, or date, so ReadMetadata's ISOBMFF fallback
// should decode both to the same Metadata.
var wantISOBMFFTestFixtureMetadata = Metadata{
	Make:    "TestCam",
	Model:   "Model123",
	FNumber: "f/5.6",
	ISO:     "ISO 800",
}

func TestReadMetadata_HEICFallback(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "test_exif.heic"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	if m := ReadMetadata(data); m != wantISOBMFFTestFixtureMetadata {
		t.Errorf("ReadMetadata() = %+v, want %+v", m, wantISOBMFFTestFixtureMetadata)
	}
}

func TestReadMetadata_AVIFFallback(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "test_exif.avif"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	if m := ReadMetadata(data); m != wantISOBMFFTestFixtureMetadata {
		t.Errorf("ReadMetadata() = %+v, want %+v", m, wantISOBMFFTestFixtureMetadata)
	}
}

func TestReadMetadata_HEICWithNoExifYieldsEmpty(t *testing.T) {
	// A synthesized AVIF (this package's encodeAVIF helper, loader_test.go)
	// has no Exif box at all - the ISOBMFF fallback should come back empty,
	// not error or panic, same as a JPEG with no Exif segment.
	data := encodeAVIF(t, 4, 4, color.White)

	if m := ReadMetadata(data); !m.Empty() {
		t.Errorf("ReadMetadata() = %+v, want empty", m)
	}
}

func TestFormatExposureTime(t *testing.T) {
	cases := []struct {
		seconds float64
		want    string
	}{
		{1.0 / 200, "1/200 s"},
		{1.0 / 4000, "1/4000 s"},
		{2.5, "2.5 s"},
		{1, "1.0 s"},
	}

	for _, c := range cases {
		if got := formatExposureTime(c.seconds); got != c.want {
			t.Errorf("formatExposureTime(%v) = %q, want %q", c.seconds, got, c.want)
		}
	}
}

func TestFormatFocalLength(t *testing.T) {
	cases := []struct {
		mm   float64
		want string
	}{
		{50, "50 mm"},
		{18.5, "18.5 mm"},
	}

	for _, c := range cases {
		if got := formatFocalLength(c.mm); got != c.want {
			t.Errorf("formatFocalLength(%v) = %q, want %q", c.mm, got, c.want)
		}
	}
}

func TestFormatExifDate(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"2024:08:12 14:33:02", "2024-08-12 14:33:02"},
		{"garbage", "garbage"},
	}

	for _, c := range cases {
		if got := formatExifDate(c.raw); got != c.want {
			t.Errorf("formatExifDate(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestParseExifDateTime(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, ok := parseExifDateTime("2024:08:12 14:33:02")
		if !ok {
			t.Fatal("ok = false, want true")
		}
		want := time.Date(2024, 8, 12, 14, 33, 2, 0, time.Local)
		if !got.Equal(want) {
			t.Errorf("parseExifDateTime() = %v, want %v", got, want)
		}
	})

	t.Run("malformed", func(t *testing.T) {
		if _, ok := parseExifDateTime("garbage"); ok {
			t.Error("ok = true, want false for a malformed date")
		}
	})
}

func TestReadEXIFOrientation(t *testing.T) {
	t.Run("no Exif data at all", func(t *testing.T) {
		data := encodeJPEG(t, 4, 4, color.White)
		if got := readEXIFOrientation(data); got != 1 {
			t.Errorf("readEXIFOrientation() = %d, want 1 (no correction)", got)
		}
	})

	t.Run("Exif segment after another APP marker", func(t *testing.T) {
		jfif := wrapAsAPP1([]byte("JFIF\x00\x00padding")) // pretend APP1, not Exif
		exif := wrapAsAPP1(buildExifSegment(t, 6, false))

		data := append([]byte{0xFF, 0xD8}, jfif...)
		data = append(data, exif...)

		if got := readEXIFOrientation(data); got != 6 {
			t.Errorf("readEXIFOrientation() = %d, want 6", got)
		}
	})

	t.Run("truncated file", func(t *testing.T) {
		if got := readEXIFOrientation([]byte{0xFF, 0xD8}); got != 1 {
			t.Errorf("readEXIFOrientation() = %d, want 1", got)
		}
	})

	t.Run("not a JPEG", func(t *testing.T) {
		if got := readEXIFOrientation([]byte("not a jpeg")); got != 1 {
			t.Errorf("readEXIFOrientation() = %d, want 1", got)
		}
	})
}
