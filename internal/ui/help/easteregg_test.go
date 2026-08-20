package help

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestManualView_SecretPhraseFiresCallbackOnce(t *testing.T) {
	var calls int
	v := newManualView(searchFixture, func() { calls++ })

	v.entry.SetText(secretPhrase)
	v.submit(v.entry.Text)

	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestManualView_SecretPhraseCaseAndWhitespaceInsensitive(t *testing.T) {
	cases := []string{
		"PLEASE HYPNOTIZE ME",
		"Please Hypnotize Me",
		"  please hypnotize me  ",
	}

	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			var calls int
			v := newManualView(searchFixture, func() { calls++ })

			v.entry.SetText(q)
			v.submit(q)

			if calls != 1 {
				t.Errorf("calls = %d, want 1 for %q", calls, q)
			}
		})
	}
}

func TestManualView_SubstringOfSecretDoesNotFireIt(t *testing.T) {
	var calls int
	v := newManualView(searchFixture, func() { calls++ })

	q := "say " + secretPhrase + " now"
	v.entry.SetText(q)
	v.submit(q)

	if calls != 0 {
		t.Errorf("calls = %d, want 0 for a mere substring match", calls)
	}
}

func TestManualView_SecretPhraseClearsEntryAndSearchState(t *testing.T) {
	v := newManualView(searchFixture, func() {})

	v.entry.SetText(secretPhrase)
	v.submit(v.entry.Text)

	if v.entry.Text != "" {
		t.Errorf("entry text = %q, want empty", v.entry.Text)
	}
	if got := hitTexts(v.text.Segments); len(got) != 0 {
		t.Errorf("highlighted segments = %v, want none", got)
	}
	if v.current != nil {
		t.Errorf("current = %+v, want nil", v.current)
	}
	if v.state != (searchState{}) {
		t.Errorf("state = %+v, want zero value", v.state)
	}
}

func TestManualView_OrdinaryQueryDoesNotFireSecretAndStillHighlights(t *testing.T) {
	var calls int
	v := newManualView(searchFixture, func() { calls++ })

	v.entry.SetText("alpha")
	v.submit(v.entry.Text)

	if calls != 0 {
		t.Errorf("calls = %d, want 0 for an ordinary query", calls)
	}
	if got := hitTexts(v.text.Segments); len(got) != 2 {
		t.Errorf("highlighted = %v, want two alphas", got)
	}
}

func TestHelp_SecretPhraseInManualOpensSpiral(t *testing.T) {
	a := test.NewApp()
	t.Cleanup(a.Quit)

	h := New(a, "PicFetch", nil)
	orig := currentManual
	t.Cleanup(func() { currentManual = orig })
	currentManual = func() string { return "just words" }

	h.ShowManual()
	t.Cleanup(func() {
		h.spiral.Close()
		h.spiral.Settle()
	})

	h.manual.entry.SetText(secretPhrase)
	h.manual.submit(h.manual.entry.Text)

	if !h.spiral.Open() {
		t.Fatal("submitting the secret phrase did not open the spiral window")
	}
}
