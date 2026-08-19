package exifwin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/frathe/picfetch/internal/imaging"
)

const (
	// osmTiles is the tile URL the map is drawn from, in the
	// zoom/x/y order fyne.io/x/fyne's Map formats its arguments in.
	osmTiles = "https://tile.openstreetmap.org/%d/%d/%d.png"

	// userAgent identifies this app to the tile server, as
	// OpenStreetMap's tile usage policy requires - the map widget's own
	// generic "Fyne-X Map Widget" would not.
	userAgent = "PicFetch/1.0 (+https://github.com/frathe/picfetch)"

	// tileBudget bounds the decoded-tile cache. An OSM tile is a PNG of
	// roughly 10-50 KB, so this holds several hundred of them - far more
	// than the handful of screens' worth a session's EXIF windows look at.
	tileBudget = 16 << 20

	// maxTileBytes is the most this will read from one tile response, so a
	// server answering with something enormous can't grow the cache
	// unbounded before the budget above ever sees it.
	maxTileBytes = 4 << 20

	// tileTimeout bounds a single tile request; tileWorkers bounds how many
	// of them a prefetch runs at once, both to stay a good citizen of a
	// donated tile server and to keep a stalled request from pinning the
	// whole prefetch - which is also how long the "loading" indicator can
	// linger on a network that accepts connections and then says nothing.
	tileTimeout = 10 * time.Second
	tileWorkers = 4

	// tileRetryAfter is how long a failed tile is left alone before it is
	// tried again. Without it an offline session would re-request every
	// missing tile on every single repaint.
	tileRetryAfter = 30 * time.Second

	// prefetchRadius is how many tiles beyond the center one a prefetch
	// warms in each direction: a 5x5 block, comfortably more than the
	// panel-sized map draws at once.
	prefetchRadius = 2
)

// errTilePending is what the map widget's HTTP client returns for a tile
// that isn't cached yet. It is the whole point of this file: the widget
// fetches tiles from *inside* its raster draw function, which runs on the
// UI goroutine, so letting that call reach the network freezes the app for
// as long as the download takes. Failing instantly instead - while the
// real download runs in the background, and the map is refreshed once it
// lands - keeps the draw non-blocking. The widget logs the failure and
// skips that tile for this frame, and, crucially, does not cache the
// failure, so the refresh redraws it for real.
var errTilePending = errors.New("tile not downloaded yet")

// The map widget calls fyne.LogError("tile fetch error", err) for every
// tile it doesn't get, on every frame it doesn't get it - so a zoom or a
// pan onto tiles that are still downloading writes a three-line block per
// missing tile, dozens at a time, for a condition that is this file's
// normal operation rather than a fault. quietPendingTiles drops exactly
// that block from the log.
//
// Suppressing it loses nothing: the widget only ever sees a cached tile or
// errTilePending, because a real download failure is handled here (backed
// off in claim, never passed on), so "tile fetch error" caused by
// errTilePending carries no information the log doesn't already have. A
// "tile fetch error" from any other cause - a corrupt tile failing to
// decode, say - still prints its cause and location.
const tileFetchError = "tile fetch error"

var quietOnce sync.Once

// quietPendingTiles installs the filter over the standard logger's current
// output, once per process. It is a process-wide side effect for want of
// anywhere narrower to put it: the log call is inside the widget, made
// from a draw this package doesn't drive.
func quietPendingTiles() {
	quietOnce.Do(func() {
		log.SetOutput(&tileLogFilter{out: log.Writer()})
	})
}

// tileLogFilter passes writes through except for the three lines
// fyne.LogError emits for a tile that hasn't downloaded yet. Each of those
// lines arrives as its own Write, hence the state: the header only counts
// as ours once the cause behind it turns out to be errTilePending.
type tileLogFilter struct {
	out io.Writer

	mu sync.Mutex

	// stage is how far into a suppressed block the last line got: 0 none,
	// 1 the "Fyne error" header, 2 its cause.
	stage int
}

func (t *tileLogFilter) Write(p []byte) (int, error) {
	t.mu.Lock()

	stage := t.stage
	t.stage = 0

	switch {
	case bytes.Contains(p, []byte(tileFetchError)):
		t.stage = 1
	case stage == 1 && bytes.Contains(p, []byte(errTilePending.Error())):
		t.stage = 2
	case stage == 2 && bytes.Contains(p, []byte("  At:")):
	default:
		t.mu.Unlock()

		return t.out.Write(p)
	}

	t.mu.Unlock()

	return len(p), nil
}

// tileFetcher is the map's tile source: an HTTP client whose transport
// never blocks (see errTilePending), a byte-bounded cache of the tiles it
// has, and the bookkeeping that lets the window show a spinner while
// anything is still on its way.
//
// It is a field on Window rather than package-level state so tests can
// point one at an httptest server instead of the real tile service.
type tileFetcher struct {
	template string
	base     http.RoundTripper
	cache    *imaging.ByteCache[[]byte]

	mu       sync.Mutex
	inflight map[string]bool
	failed   map[string]time.Time
	pending  int
	warming  bool
	onChange func(pending int)

	// now is time.Now, replaced in tests that need the retry backoff to
	// pass without sleeping.
	now func() time.Time
}

// newTileFetcher returns a fetcher for tiles named by template (the
// zoom/x/y URL format string), downloading through base - nil for the
// default transport, which is what production uses.
func newTileFetcher(template string, base http.RoundTripper) *tileFetcher {
	if base == nil {
		base = http.DefaultTransport
	}

	// The widget logs a fault for every tile this fetcher answers with
	// errTilePending, which is most of them on a first view - see
	// quietPendingTiles.
	quietPendingTiles()

	return &tileFetcher{
		template: template,
		base:     base,
		cache:    imaging.NewByteCache(int64(tileBudget), func(b []byte) int64 { return int64(len(b)) }),
		inflight: make(map[string]bool),
		failed:   make(map[string]time.Time),
		now:      time.Now,
	}
}

// client is the http.Client to hand the map widget: this fetcher itself,
// acting as the transport.
func (f *tileFetcher) client() *http.Client {
	return &http.Client{Transport: f}
}

// SetOnChange registers what to call after every background tile finishes,
// with the number still outstanding. Called from the fetching goroutine,
// so the callback is responsible for marshalling onto the UI goroutine.
func (f *tileFetcher) SetOnChange(fn func(pending int)) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.onChange = fn
}

// Pending is how many tiles are being downloaded right now - what the
// window's loading indicator follows.
func (f *tileFetcher) Pending() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.pending
}

// RoundTrip serves the map widget's tile requests from cache, and answers
// anything it doesn't have with errTilePending after starting the real
// download in the background.
func (f *tileFetcher) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()

	if b, ok := f.cache.Get(url); ok {
		return tileResponse(req, b), nil
	}

	if f.claim(url) {
		go f.fetch(url)
	}

	return nil, errTilePending
}

// tileResponse wraps cached tile bytes as the 200 response the widget's
// image decoder expects.
func tileResponse(req *http.Request, b []byte) *http.Response {
	return &http.Response{
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		Proto:         "HTTP/1.1",
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(bytes.NewReader(b)),
		ContentLength: int64(len(b)),
		Request:       req,
	}
}

// claim reports whether the caller should download url, and counts it as
// outstanding if so. It says no for a tile that is already cached, already
// being downloaded, or that failed within the last tileRetryAfter.
func (f *tileFetcher) claim(url string) bool {
	if f.cache.Contains(url) {
		return false
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.inflight[url] {
		return false
	}

	if at, ok := f.failed[url]; ok && f.now().Sub(at) < tileRetryAfter {
		return false
	}

	f.inflight[url] = true
	f.pending++

	return true
}

// release records a claimed tile's outcome - its bytes, or an error worth
// backing off from - and reports the outstanding count to onChange.
func (f *tileFetcher) release(url string, data []byte, err error) {
	if err == nil {
		f.cache.Add(url, data)
	}

	f.mu.Lock()

	delete(f.inflight, url)
	f.pending--

	if err != nil {
		f.failed[url] = f.now()
	} else {
		delete(f.failed, url)
	}

	pending, onChange := f.pending, f.onChange

	// A prefetch reports its own completion once, when the whole block is
	// in - one redraw for the batch instead of one per tile, and one
	// goroutine touching the map instead of two racing to.
	if f.warming {
		onChange = nil
	}

	f.mu.Unlock()

	if onChange != nil {
		onChange(pending)
	}
}

// fetch downloads one claimed tile. Errors are deliberately not reported
// anywhere: a tile that doesn't arrive is a gap in the map, the widget has
// already logged its own failure for that frame, and the backoff in claim
// is what stops a dead network turning into a request storm.
func (f *tileFetcher) fetch(url string) {
	data, err := f.get(url)

	f.release(url, data, err)
}

func (f *tileFetcher) get(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Transport: f.base, Timeout: tileTimeout}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tile server returned %s", res.Status)
	}

	data, err := io.ReadAll(io.LimitReader(res.Body, maxTileBytes))
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Warm downloads the block of tiles around lat/lon at zoom and returns
// once they have all arrived (or failed), so the window can keep a
// "loading" indicator up for exactly as long as the first view of the map
// is still missing pieces. Tiles already cached, already in flight, or in
// backoff are skipped, which is what makes re-expanding the section
// instant.
func (f *tileFetcher) Warm(lat, lon float64, zoom int) {
	f.mu.Lock()
	f.warming = true
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.warming = false
		f.mu.Unlock()
	}()

	urls := f.neighborhood(lat, lon, zoom)

	sem := make(chan struct{}, tileWorkers)
	var wg sync.WaitGroup

	for _, url := range urls {
		if !f.claim(url) {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}

		go func(url string) {
			defer wg.Done()
			defer func() { <-sem }()

			f.fetch(url)
		}(url)
	}

	wg.Wait()
}

// neighborhood is the tile URLs within prefetchRadius of the tile holding
// lat/lon at zoom, skipping the ones off the edge of the world.
func (f *tileFetcher) neighborhood(lat, lon float64, zoom int) []string {
	centerX, centerY := tileXY(lat, lon, zoom)
	n := 1 << zoom

	var urls []string

	for x := centerX - prefetchRadius; x <= centerX+prefetchRadius; x++ {
		for y := centerY - prefetchRadius; y <= centerY+prefetchRadius; y++ {
			if x < 0 || y < 0 || x >= n || y >= n {
				continue
			}

			urls = append(urls, fmt.Sprintf(f.template, zoom, x, y))
		}
	}

	return urls
}

// tileXY converts a position to the slippy-map tile holding it, the same
// arithmetic the map widget uses to decide which tiles to draw:
// https://wiki.openstreetmap.org/wiki/Slippy_map_tilenames#Mathematics
func tileXY(lat, lon float64, zoom int) (x, y int) {
	n := float64(int(1) << zoom)
	latRad := lat * math.Pi / 180

	x = int(math.Floor((lon + 180) / 360 * n))
	y = int(math.Floor((1 - math.Log(math.Tan(latRad)+1/math.Cos(latRad))/math.Pi) / 2 * n))

	return x, y
}
