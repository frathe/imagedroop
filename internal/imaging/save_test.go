package imaging

import (
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"fyne.io/fyne/v2/storage"
)

func TestCanEncode(t *testing.T) {
	cases := []struct {
		name string
		u    fakeURI
		want bool
	}{
		{"jpg", fakeURI{name: "a.jpg", ext: ".jpg"}, true},
		{"jpeg uppercase extension", fakeURI{name: "A.JPEG", ext: ".JPEG"}, true},
		{"png", fakeURI{name: "a.png", ext: ".png"}, true},
		{"gif", fakeURI{name: "a.gif", ext: ".gif"}, true},
		{"bmp", fakeURI{name: "a.bmp", ext: ".bmp"}, true},
		{"tif", fakeURI{name: "a.tif", ext: ".tif"}, true},
		{"tiff", fakeURI{name: "a.tiff", ext: ".tiff"}, true},
		{"avif", fakeURI{name: "a.avif", ext: ".avif"}, true},
		{"webp is decode-only, no encoder", fakeURI{name: "a.webp", ext: ".webp"}, false},
		{"heic is decode-only, no encoder", fakeURI{name: "a.heic", ext: ".heic"}, false},
		{"ico unsupported", fakeURI{name: "a.ico", ext: ".ico"}, false},
		{"xpm unsupported", fakeURI{name: "a.xpm", ext: ".xpm"}, false},
		{"no extension", fakeURI{name: "a", ext: ""}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CanEncode(c.u); got != c.want {
				t.Errorf("CanEncode(%+v) = %v, want %v", c.u, got, c.want)
			}
		})
	}
}

func TestSaveRotated(t *testing.T) {
	t.Run("overwrites the file with the given pixels, exactly for a lossless format", func(t *testing.T) {
		path := writeTempFile(t, "photo.png", []byte("placeholder, never read back"))
		u := storage.NewFileURI(path)

		const w, h = 3, 2
		if err := SaveRotated(u, markedImage(w, h)); err != nil {
			t.Fatalf("SaveRotated: %v", err)
		}

		got, err := LoadImage(u, DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("reload after save: %v", err)
		}

		b := got.Frames[0].Bounds()
		if b.Dx() != w || b.Dy() != h {
			t.Fatalf("bounds after save = %v, want %dx%d", b, w, h)
		}

		for y := range h {
			for x := range w {
				c := got.Frames[0].At(x, y).(color.RGBA)
				if int(c.R) != x || int(c.G) != y {
					t.Errorf("(%d,%d) = (%d,%d), want (%d,%d)", x, y, c.R, c.G, x, y)
				}
			}
		}
	})

	t.Run("unsupported format returns an error and leaves the file untouched", func(t *testing.T) {
		original := []byte("not a real webp, but SaveRotated should never get far enough to care")
		path := writeTempFile(t, "photo.webp", original)
		u := storage.NewFileURI(path)

		if err := SaveRotated(u, markedImage(2, 2)); err == nil {
			t.Fatal("SaveRotated: want error for unsupported format, got nil")
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != string(original) {
			t.Error("SaveRotated modified the file despite returning an error")
		}
	})

	t.Run("preserves the original file permissions", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows does not expose Unix permission bits")
		}

		path := writeTempFile(t, "private.png", []byte("placeholder"))
		if err := os.Chmod(path, 0o640); err != nil {
			t.Fatalf("chmod fixture: %v", err)
		}

		if err := SaveRotated(storage.NewFileURI(path), markedImage(2, 2)); err != nil {
			t.Fatalf("SaveRotated: %v", err)
		}

		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat saved file: %v", err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o640); got != want {
			t.Errorf("saved file permissions = %o, want %o", got, want)
		}
	})

	t.Run("updates a symlink target without replacing the link", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target.png")
		if err := os.WriteFile(target, []byte("placeholder"), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}
		link := filepath.Join(dir, "photo.webp")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}

		u := storage.NewFileURI(link)
		if !CanEncode(u) {
			t.Fatal("CanEncode returned false for a link to an encodable PNG target")
		}
		if err := SaveRotated(u, markedImage(3, 2)); err != nil {
			t.Fatalf("SaveRotated: %v", err)
		}

		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("lstat link: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("SaveRotated replaced the symlink instead of updating its target")
		}

		loaded, err := LoadImage(storage.NewFileURI(target), DefaultImgCacheBytes)
		if err != nil {
			t.Fatalf("load saved target: %v", err)
		}
		if got := loaded.Frames[0].Bounds(); got.Dx() != 3 || got.Dy() != 2 {
			t.Errorf("saved target bounds = %v, want 3x2", got)
		}
	})

	t.Run("every broadened format round-trips to the new dimensions", func(t *testing.T) {
		for _, ext := range []string{".jpg", ".png", ".gif", ".bmp", ".tiff", ".avif"} {
			t.Run(ext, func(t *testing.T) {
				path := writeTempFile(t, "photo"+ext, nil)
				u := storage.NewFileURI(path)

				const w, h = 4, 3
				if err := SaveRotated(u, markedImage(w, h)); err != nil {
					t.Fatalf("SaveRotated: %v", err)
				}

				loaded, err := LoadImage(u, DefaultImgCacheBytes)
				if err != nil {
					t.Fatalf("reload after save: %v", err)
				}
				if b := loaded.Frames[0].Bounds(); b.Dx() != w || b.Dy() != h {
					t.Errorf("bounds after save = %v, want %dx%d", b, w, h)
				}
			})
		}
	})
}
