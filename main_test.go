package main

import (
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestArgsToURIs_ResolvesRelativeToAbsolute(t *testing.T) {
	uris := argsToURIs([]string{"one.jpg", filepath.Join("sub", "two.png")})

	if len(uris) != 2 {
		t.Fatalf("len(uris) = %d, want 2", len(uris))
	}

	wantOne, err := filepath.Abs("one.jpg")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if uris[0].Path() != wantOne {
		t.Errorf("uris[0].Path() = %q, want %q", uris[0].Path(), wantOne)
	}
}

func TestArgsToURIs_EmptyArgsReturnsEmpty(t *testing.T) {
	if uris := argsToURIs(nil); len(uris) != 0 {
		t.Errorf("len(uris) = %d, want 0", len(uris))
	}
}

func TestArgsToURIs_AbsoluteArgUnchanged(t *testing.T) {
	uris := argsToURIs([]string{"/already/absolute.jpg"})

	if len(uris) != 1 {
		t.Fatalf("len(uris) = %d, want 1", len(uris))
	}
	if uris[0].Path() != "/already/absolute.jpg" {
		t.Errorf("uris[0].Path() = %q, want %q", uris[0].Path(), "/already/absolute.jpg")
	}
}

// --- translations -----------------------------------------------------------

// loadBundle reads one embedded translation bundle, failing the test if it
// isn't valid JSON - which is the failure mode worth catching early:
// lang.AddTranslationsFS only logs a load error, so a malformed bundle
// ships as an app that silently falls back to the untranslated keys.
func loadBundle(t *testing.T, name string) map[string]string {
	t.Helper()

	data, err := fs.ReadFile(translationsFS, "translations/"+name)
	if err != nil {
		t.Fatalf("reading the embedded %s: %v", name, err)
	}

	var bundle map[string]string
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	if len(bundle) == 0 {
		t.Fatalf("%s is empty", name)
	}

	return bundle
}

// TestTranslations_EveryLocaleCoversEnglish is the guard against the way
// this actually goes wrong: a new lang.L string gets added to en.json and
// nowhere else, and nothing complains until a German user sees an English
// word in the middle of their UI. English is the source locale, so it
// defines the key set every other bundle has to cover exactly - a key only
// de.json has is just as broken, since it means an English string was
// renamed and its translation left stranded.
func TestTranslations_EveryLocaleCoversEnglish(t *testing.T) {
	en := loadBundle(t, "en.json")

	for _, locale := range []string{"de.json"} {
		other := loadBundle(t, locale)

		for key := range en {
			if _, ok := other[key]; !ok {
				t.Errorf("%s is missing a translation for %q", locale, key)
			}
		}
		for key := range other {
			if _, ok := en[key]; !ok {
				t.Errorf("%s translates %q, which no longer exists in en.json", locale, key)
			}
		}
	}
}

// TestTranslations_EnglishMapsEachKeyToItself pins the convention this app
// uses: the lang.L argument in the source *is* the English text, so en.json
// is an identity mapping. A value that differs from its key means the
// English UI silently says something other than what the source reads,
// which here is always a typo rather than an intent.
func TestTranslations_EnglishMapsEachKeyToItself(t *testing.T) {
	for key, value := range loadBundle(t, "en.json") {
		if key != value {
			t.Errorf("en.json maps %q to %q, want the key itself", key, value)
		}
	}
}
