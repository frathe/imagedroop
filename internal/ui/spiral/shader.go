package spiral

import "fyne.io/fyne/v2/canvas"

// shaderSourceDesktop targets desktop OpenGL (GLSL 110), matching the
// legacy compatibility-profile context Fyne creates on desktop platforms.
const shaderSourceDesktop = `#version 110
uniform vec2 frame;
uniform vec4 bounds;
uniform float time;
uniform float arms;
uniform float twistBase;
uniform float speed;
uniform float hueSpeed;
uniform float centerOffsetX;
uniform float centerOffsetY;
uniform float density;
uniform float preset;

const float TAU = 6.28318530718;
const int NUM_LAYERS = 4;

vec3 hsv2rgb(vec3 c) {
    vec4 K = vec4(1.0, 2.0/3.0, 1.0/3.0, 3.0);
    vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
    return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
}

vec3 rippleSpiral(vec2 frag, vec2 center) {
    vec2 d = frag - center;
    float radius = length(d) / twistBase;
    float angle = atan(d.y, d.x) * arms;
    float phase = angle + radius - time * speed;
    float v = sin(phase);
    float hue = fract((v + 1.0) * 0.5 + time * hueSpeed);
    float val = 0.5 + 0.5 * v;
    return hsv2rgb(vec3(hue, 0.9, val));
}

// nautilusSpiral draws several counter-winding, power-law spiral arms
// (radius = u^(1/curve), so they bunch tightly near the centre and fan out
// toward the edge like a nautilus shell) and combines them with a lighten
// blend (componentwise max), echoing the layered, screen-blended look of a
// reference multi-layer canvas spiral this preset is modelled after.
vec3 nautilusSpiral(vec2 frag, vec2 center) {
    vec2 d = frag - center;
    float r = length(d);
    float theta = atan(d.y, d.x);
    float rMax = max(length(frame) * 0.5, 1.0);
    float u = clamp(r / rMax, 0.0, 1.0);
    float turns = twistBase / 10.0;

    vec3 result = vec3(0.02, 0.01, 0.04);
    for (int i = 0; i < NUM_LAYERS; i++) {
        float layerT = float(i) / float(NUM_LAYERS - 1);
        float curve = 2.0 + layerT * 1.2;
        float spiralAngle = pow(u, 1.0 / curve) * (turns + layerT) * TAU;
        // Outer layers wind slightly slower, and the outermost reverses
        // direction, giving the layers visible relative motion.
        float layerSpeed = speed * (0.75 - layerT);
        float phase = theta - spiralAngle - time * layerSpeed;
        float armPos = fract(phase * arms / TAU);

        float duty = 0.32;
        float edge = 0.05;
        float band = smoothstep(0.0, edge, armPos) - smoothstep(duty, duty + edge, armPos);
        band = clamp(band, 0.0, 1.0);

        float hue = fract(u * 0.5 + layerT * 0.18 + time * hueSpeed);
        vec3 layerColor = hsv2rgb(vec3(hue, 0.85, 1.0)) * band;
        result = max(result, layerColor);
    }
    return result;
}

void main() {
    vec2 frag = gl_FragCoord.xy;
    // density is in [0.25, 1.0]; map it to a pixel block size from 1px
    // (native, density = 1.0) up to 30px (heavily pixelated, density = 0.25)
    // so the slider's effect is clearly visible rather than a 1-4px change
    // that would be imperceptible on a high-DPI display.
    float blockSize = max(1.0, 40.0 * (1.0 - density));
    frag = (floor(frag / blockSize) + 0.5) * blockSize;
    vec2 center = vec2(frame.x * 0.5, frame.y * 0.5) + vec2(centerOffsetX, centerOffsetY);

    vec3 rgb;
    if (preset < 0.5) {
        rgb = rippleSpiral(frag, center);
    } else {
        rgb = nautilusSpiral(frag, center);
    }
    gl_FragColor = vec4(rgb, 1.0);
}
`

// shaderSourceES is the OpenGL ES / mobile / web variant of shaderSourceDesktop.
const shaderSourceES = `#version 100

#ifdef GL_ES
# ifdef GL_FRAGMENT_PRECISION_HIGH
precision highp float;
# else
precision mediump float;
#endif
precision mediump int;
#endif

uniform vec2 frame;
uniform vec4 bounds;
uniform float time;
uniform float arms;
uniform float twistBase;
uniform float speed;
uniform float hueSpeed;
uniform float centerOffsetX;
uniform float centerOffsetY;
uniform float density;
uniform float preset;

const float TAU = 6.28318530718;
const int NUM_LAYERS = 4;

vec3 hsv2rgb(vec3 c) {
    vec4 K = vec4(1.0, 2.0/3.0, 1.0/3.0, 3.0);
    vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
    return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
}

vec3 rippleSpiral(vec2 frag, vec2 center) {
    vec2 d = frag - center;
    float radius = length(d) / twistBase;
    float angle = atan(d.y, d.x) * arms;
    float phase = angle + radius - time * speed;
    float v = sin(phase);
    float hue = fract((v + 1.0) * 0.5 + time * hueSpeed);
    float val = 0.5 + 0.5 * v;
    return hsv2rgb(vec3(hue, 0.9, val));
}

// nautilusSpiral draws several counter-winding, power-law spiral arms
// (radius = u^(1/curve), so they bunch tightly near the centre and fan out
// toward the edge like a nautilus shell) and combines them with a lighten
// blend (componentwise max), echoing the layered, screen-blended look of a
// reference multi-layer canvas spiral this preset is modelled after.
vec3 nautilusSpiral(vec2 frag, vec2 center) {
    vec2 d = frag - center;
    float r = length(d);
    float theta = atan(d.y, d.x);
    float rMax = max(length(frame) * 0.5, 1.0);
    float u = clamp(r / rMax, 0.0, 1.0);
    float turns = twistBase / 10.0;

    vec3 result = vec3(0.02, 0.01, 0.04);
    for (int i = 0; i < NUM_LAYERS; i++) {
        float layerT = float(i) / float(NUM_LAYERS - 1);
        float curve = 2.0 + layerT * 1.2;
        float spiralAngle = pow(u, 1.0 / curve) * (turns + layerT) * TAU;
        // Outer layers wind slightly slower, and the outermost reverses
        // direction, giving the layers visible relative motion.
        float layerSpeed = speed * (0.75 - layerT);
        float phase = theta - spiralAngle - time * layerSpeed;
        float armPos = fract(phase * arms / TAU);

        float duty = 0.32;
        float edge = 0.05;
        float band = smoothstep(0.0, edge, armPos) - smoothstep(duty, duty + edge, armPos);
        band = clamp(band, 0.0, 1.0);

        float hue = fract(u * 0.5 + layerT * 0.18 + time * hueSpeed);
        vec3 layerColor = hsv2rgb(vec3(hue, 0.85, 1.0)) * band;
        result = max(result, layerColor);
    }
    return result;
}

void main() {
    vec2 frag = gl_FragCoord.xy;
    // density is in [0.25, 1.0]; map it to a pixel block size from 1px
    // (native, density = 1.0) up to 30px (heavily pixelated, density = 0.25)
    // so the slider's effect is clearly visible rather than a 1-4px change
    // that would be imperceptible on a high-DPI display.
    float blockSize = max(1.0, 40.0 * (1.0 - density));
    frag = (floor(frag / blockSize) + 0.5) * blockSize;
    vec2 center = vec2(frame.x * 0.5, frame.y * 0.5) + vec2(centerOffsetX, centerOffsetY);

    vec3 rgb;
    if (preset < 0.5) {
        rgb = rippleSpiral(frag, center);
    } else {
        rgb = nautilusSpiral(frag, center);
    }
    gl_FragColor = vec4(rgb, 1.0);
}
`

// newShader builds the shader canvas object and seeds its Uniforms from st's
// current values (rather than the package defaults), so a state that has
// already been adjusted - e.g. settings restored from a previous run -
// opens the spiral at its current values instead of snapping back to
// defaults.
//
// "time" is deliberately absent from the seeded map: canvas.NewShaderAnimation
// writes that entry itself on every animated frame, so setting it here would
// just be immediately overwritten.
func newShader(st *state) *canvas.Shader {
	sh := canvas.NewShader("hypno-spiral", []byte(shaderSourceDesktop), []byte(shaderSourceES))

	centerOffsetX, centerOffsetY := st.centerOffset()
	preset := float32(0)
	if st.preset() {
		preset = 1
	}

	sh.Uniforms = map[string]float32{
		"arms":          float32(st.arms),
		"twistBase":     float32(st.twist),
		"speed":         float32(st.speed()),
		"hueSpeed":      float32(st.hueSpeed()),
		"centerOffsetX": float32(centerOffsetX),
		"centerOffsetY": float32(centerOffsetY),
		"density":       float32(st.density),
		"preset":        preset,
	}

	return sh
}
