package spiral

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"
)

// TestShaderUniformsDeclaredInBothSources guards the real hazard of a
// hand-maintained two-variant shader: a uniform key that's typo'd or
// removed from one of the GLSL sources isn't a compile error, it's a
// silently ignored uniform - the parameter it was meant to drive just stops
// doing anything, discovered later by eye rather than by a failing build.
func TestShaderUniformsDeclaredInBothSources(t *testing.T) {
	test.NewApp()
	sh := newShader(newState())

	for k := range sh.Uniforms {
		decl := "uniform float " + k + ";"
		if !strings.Contains(shaderSourceDesktop, decl) {
			t.Errorf("shaderSourceDesktop missing declaration %q for uniform key %q", decl, k)
		}
		if !strings.Contains(shaderSourceES, decl) {
			t.Errorf("shaderSourceES missing declaration %q for uniform key %q", decl, k)
		}
	}

	// frame isn't in the Uniforms map - it's one of the built-ins
	// canvas.Shader always supplies - but the nautilus preset reads it
	// directly, so its declaration matters just as much.
	for name, src := range map[string]string{"shaderSourceDesktop": shaderSourceDesktop, "shaderSourceES": shaderSourceES} {
		if !strings.Contains(src, "uniform vec2 frame;") {
			t.Errorf("%s missing `uniform vec2 frame;` declaration", name)
		}
	}
}

// TestShaderSourcesStructurallyComplete checks both GLSL variants define the
// pieces newShader and the preset switch in main() depend on, so a source
// gutted by a bad copy-paste fails here instead of at first paint.
func TestShaderSourcesStructurallyComplete(t *testing.T) {
	for name, src := range map[string]string{"shaderSourceDesktop": shaderSourceDesktop, "shaderSourceES": shaderSourceES} {
		for _, want := range []string{"void main", "gl_FragColor", "rippleSpiral", "nautilusSpiral"} {
			if !strings.Contains(src, want) {
				t.Errorf("%s missing %q", name, want)
			}
		}
	}
}

// TestShaderSourcesAgreeBelowPreamble is the test that fails when someone
// edits one preset's logic and forgets the other: shaderSourceES is only
// meant to differ from shaderSourceDesktop in its header (the #version line
// and the #ifdef GL_ES precision block). Everything from the first shared
// uniform declaration onward must be byte-for-byte identical.
func TestShaderSourcesAgreeBelowPreamble(t *testing.T) {
	const marker = "uniform vec2 frame;"

	di := strings.Index(shaderSourceDesktop, marker)
	if di < 0 {
		t.Fatalf("shaderSourceDesktop has no %q", marker)
	}
	ei := strings.Index(shaderSourceES, marker)
	if ei < 0 {
		t.Fatalf("shaderSourceES has no %q", marker)
	}

	desktopBody := shaderSourceDesktop[di:]
	esBody := shaderSourceES[ei:]
	if desktopBody != esBody {
		t.Errorf("shaderSourceDesktop and shaderSourceES diverge below their shared preamble marker %q; the two variants must stay in lockstep past the header", marker)
	}
}

// TestNewShaderSeedsFromState checks newShader reads live state rather than
// baking in the package defaults, so a state already adjusted before the
// shader is built (e.g. settings restored from a previous run) opens at its
// current values instead of snapping back to defaults.
func TestNewShaderSeedsFromState(t *testing.T) {
	test.NewApp()
	st := newState()
	st.adjustSpeed(1.0)
	st.togglePreset()

	sh := newShader(st)

	if got, want := sh.Uniforms["speed"], float32(st.speed()); got != want {
		t.Errorf("Uniforms[speed] = %f; want %f", got, want)
	}
	if got, want := sh.Uniforms["preset"], float32(1); got != want {
		t.Errorf("Uniforms[preset] = %f; want %f", got, want)
	}
}
