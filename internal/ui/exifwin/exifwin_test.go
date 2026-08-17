package exifwin

import (
	"testing"

	"github.com/frathe/picfetch/internal/imaging"
)

func TestFormatExifMetadata(t *testing.T) {
	t.Run("every field set", func(t *testing.T) {
		m := imaging.Metadata{
			Make: "Canon", Model: "EOS 90D",
			LensModel:    "EF50mm f/1.8",
			ExposureTime: "1/200 s",
			FNumber:      "f/2.8",
			ISO:          "ISO 400",
			FocalLength:  "50 mm",
			DateTaken:    "2024-08-12 14:33:02",
		}

		want := "Camera: Canon EOS 90D\n" +
			"Lens: EF50mm f/1.8\n" +
			"Exposure: 1/200 s\n" +
			"Aperture: f/2.8\n" +
			"ISO: ISO 400\n" +
			"Focal length: 50 mm\n" +
			"Date taken: 2024-08-12 14:33:02"

		if got := formatExifMetadata(m); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	t.Run("only some fields set", func(t *testing.T) {
		m := imaging.Metadata{Make: "Canon", ISO: "ISO 400"}

		want := "Camera: Canon\nISO: ISO 400"
		if got := formatExifMetadata(m); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})

	t.Run("nothing set", func(t *testing.T) {
		want := "No EXIF metadata found in this file."
		if got := formatExifMetadata(imaging.Metadata{}); got != want {
			t.Errorf("formatExifMetadata() = %q, want %q", got, want)
		}
	})
}
