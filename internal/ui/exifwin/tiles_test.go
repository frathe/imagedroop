package exifwin

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

// tilePNG is a one-pixel PNG - the smallest thing the map widget's decoder
// accepts as a tile, and all these tests need it to be.
func tilePNG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode tile: %v", err)
	}

	return buf.Bytes()
}

// tileServer is a stand-in tile service: it counts what was asked for, and
// can be made to hang until released, so a test can look at the world while
// a download is still in flight.
type tileServer struct {
	*httptest.Server

	mu       sync.Mutex
	requests []string

	block chan struct{}
	fail  bool
}

func newTileServer(t *testing.T) *tileServer {
	t.Helper()

	body := tilePNG(t)
	s := &tileServer{}

	s.Server = httptest.NewServer(http.HandlerFunc(func(wr http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, r.URL.Path)
		block, fail := s.block, s.fail
		s.mu.Unlock()

		if block != nil {
			<-block
		}

		if fail {
			wr.WriteHeader(http.StatusInternalServerError)
			return
		}

		wr.Header().Set("Content-Type", "image/png")
		_, _ = wr.Write(body)
	}))

	t.Cleanup(s.Close)

	return s
}

func (s *tileServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.requests)
}

func (s *tileServer) paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.requests...)
}

// hold makes every later request hang until the returned func is called.
func (s *tileServer) hold() func() {
	block := make(chan struct{})

	s.mu.Lock()
	s.block = block
	s.mu.Unlock()

	var once sync.Once

	return func() {
		once.Do(func() {
			s.mu.Lock()
			s.block = nil
			s.mu.Unlock()

			close(block)
		})
	}
}

func (s *tileServer) breakIt() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.fail = true
}

// fetcherFor returns a tileFetcher pointed at s instead of the real tile
// service - the swap every test in this package makes, and the reason the
// fetcher is a field on Window rather than package-level state.
func fetcherFor(s *tileServer) *tileFetcher {
	return newTileFetcher(s.URL+"/%d/%d/%d.png", http.DefaultTransport)
}

// waitForPending blocks until the fetcher has nothing outstanding.
func waitForPending(t *testing.T, f *tileFetcher) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if f.Pending() == 0 {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out with %d tiles still pending", f.Pending())
}

func TestRoundTrip_AnswersAMissImmediatelyAndFetchesInTheBackground(t *testing.T) {
	s := newTileServer(t)
	release := s.hold()
	t.Cleanup(release)

	f := fetcherFor(s)

	req, err := http.NewRequest(http.MethodGet, s.URL+"/15/0/0.png", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	start := time.Now()
	res, err := f.RoundTrip(req)
	elapsed := time.Since(start)

	if res != nil {
		t.Error("RoundTrip returned a response for an uncached tile, want none")
	}

	if err != errTilePending {
		t.Fatalf("RoundTrip() err = %v, want errTilePending", err)
	}

	// The map widget calls this from inside its raster draw, on the UI
	// goroutine: whatever the network is doing, it has to come straight
	// back or the app freezes.
	if elapsed > time.Second {
		t.Errorf("RoundTrip blocked for %v on a hanging server, want an immediate answer", elapsed)
	}

	release()
	waitForPending(t, f)

	res, err = f.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() after the download err = %v, want a cached response", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("cached response status = %d, want 200", res.StatusCode)
	}

	body := make([]byte, 8)
	if _, err := res.Body.Read(body); err != nil {
		t.Fatalf("read cached body: %v", err)
	}

	if !bytes.Equal(body, tilePNG(t)[:8]) {
		t.Error("cached response body is not the tile the server sent")
	}
}

func TestRoundTrip_DownloadsATileOnlyOnce(t *testing.T) {
	s := newTileServer(t)

	// Hold the server so the tile is genuinely still on its way for all the
	// repaints below. Without this the background download can land between
	// two of them - a loopback fetch takes microseconds - and the cache
	// starts answering them for real, which is correct behaviour failing an
	// assertion about a state the test no longer has.
	release := s.hold()
	t.Cleanup(release)

	f := fetcherFor(s)

	req, err := http.NewRequest(http.MethodGet, s.URL+"/15/1/1.png", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	// Every repaint re-asks for the same tile while it is on its way; only
	// the first of those may turn into a request.
	for range 5 {
		if _, err := f.RoundTrip(req); err != errTilePending {
			t.Fatalf("RoundTrip() err = %v, want errTilePending", err)
		}
	}

	release()
	waitForPending(t, f)

	if _, err := f.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() after the download err = %v, want a cached response", err)
	}

	if got := s.count(); got != 1 {
		t.Errorf("server saw %d requests, want exactly 1", got)
	}
}

func TestWarm_DownloadsTheBlockAroundTheLocationOnce(t *testing.T) {
	s := newTileServer(t)
	f := fetcherFor(s)

	f.Warm(48.858222, 2.2945, mapZoom)

	want := (2*prefetchRadius + 1) * (2*prefetchRadius + 1)
	if got := s.count(); got != want {
		t.Fatalf("server saw %d requests, want the %dx%d block (%d)", got, 2*prefetchRadius+1, 2*prefetchRadius+1, want)
	}

	for _, p := range s.paths() {
		if !strings.HasPrefix(p, "/15/") {
			t.Errorf("prefetched %q, want a tile at zoom %d", p, mapZoom)
		}
	}

	// Re-expanding the section must not re-download what is already cached.
	f.Warm(48.858222, 2.2945, mapZoom)

	if got := s.count(); got != want {
		t.Errorf("server saw %d requests after a second warm, want the cache to serve it (%d)", got, want)
	}

	if f.Pending() != 0 {
		t.Errorf("Pending() = %d after Warm returned, want 0", f.Pending())
	}
}

func TestOnChange_ReportsABackgroundBatchButNotAPrefetch(t *testing.T) {
	s := newTileServer(t)
	f := fetcherFor(s)

	var mu sync.Mutex
	var calls, last int

	f.SetOnChange(func(pending int) {
		mu.Lock()
		defer mu.Unlock()

		calls++
		last = pending
	})

	f.Warm(48.858222, 2.2945, mapZoom)

	mu.Lock()
	during := calls
	mu.Unlock()

	// Warm returning *is* the prefetch's completion report, so letting
	// every tile in the block report as well would only queue redraws of a
	// map the caller is about to redraw anyway.
	if during != 0 {
		t.Errorf("onChange fired %d times during a prefetch, want none", during)
	}

	// A tile the user pans onto is a different matter: nobody is waiting
	// on it, so its arrival is the only thing that can trigger the redraw.
	req, err := http.NewRequest(http.MethodGet, s.URL+"/15/900/900.png", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if _, err := f.RoundTrip(req); err != errTilePending {
		t.Fatalf("RoundTrip() err = %v, want errTilePending", err)
	}

	waitForPending(t, f)

	mu.Lock()
	defer mu.Unlock()

	if calls != 1 {
		t.Errorf("onChange fired %d times for one background tile, want 1", calls)
	}

	if last != 0 {
		t.Errorf("reported pending = %d, want 0 once the tile is in", last)
	}
}

func TestFetch_FailedTileIsRetriedOnlyAfterTheBackoff(t *testing.T) {
	s := newTileServer(t)
	s.breakIt()

	f := fetcherFor(s)

	now := time.Now()
	f.now = func() time.Time { return now }

	url := s.URL + "/15/2/2.png"

	if !f.claim(url) {
		t.Fatal("claim() = false for a tile nobody has asked for, want true")
	}

	f.fetch(url)

	if f.claim(url) {
		t.Error("claim() = true straight after a failure, want the backoff to hold it off")
	}

	now = now.Add(tileRetryAfter + time.Second)

	if !f.claim(url) {
		t.Error("claim() = false after the backoff elapsed, want a retry")
	}
}

func TestNeighborhood_ClampsToTheEdgeOfTheWorld(t *testing.T) {
	f := newTileFetcher("%d/%d/%d", http.DefaultTransport)

	// Zoom 1 is a 2x2 world, so a radius-2 block around any tile in it is
	// almost entirely off the map.
	got := f.neighborhood(0, 0, 1)

	if len(got) != 4 {
		t.Errorf("neighborhood at zoom 1 = %v (%d tiles), want the whole 2x2 world", got, len(got))
	}

	for _, url := range got {
		if strings.Contains(url, "-") {
			t.Errorf("neighborhood produced a negative tile index: %q", url)
		}
	}
}

func TestTileXY(t *testing.T) {
	cases := []struct {
		name     string
		lat, lon float64
		zoom     int
		x, y     int
	}{
		{"whole world at zoom 0", 48.858222, 2.2945, 0, 0, 0},
		{"origin sits at the seam", 0, 0, 1, 1, 1},
		{"north-west corner", 85.05, -180, 1, 0, 0},
		{"the Eiffel Tower", 48.858222, 2.2945, 15, 16592, 11272},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, y := tileXY(c.lat, c.lon, c.zoom)

			if x != c.x || y != c.y {
				t.Errorf("tileXY(%v, %v, %d) = (%d, %d), want (%d, %d)", c.lat, c.lon, c.zoom, x, y, c.x, c.y)
			}
		})
	}
}

// logLines drives lines through a filter the way log.Logger does - one
// Write per line, timestamp prefix and all - and returns what got through.
func logLines(t *testing.T, lines ...string) string {
	t.Helper()

	var out bytes.Buffer
	f := &tileLogFilter{out: &out}

	for _, line := range lines {
		p := []byte("2026/08/19 15:39:17 " + line + "\n")

		n, err := f.Write(p)
		if err != nil {
			t.Fatalf("Write(%q) returned %v", line, err)
		}

		// A filtered write still has to claim the whole line, or the
		// standard logger treats the difference as a short write.
		if n != len(p) {
			t.Errorf("Write(%q) = %d, want %d", line, n, len(p))
		}
	}

	return out.String()
}

func TestTileLogFilter(t *testing.T) {
	const (
		header  = "Fyne error:  tile fetch error"
		pending = `  Cause: Get "https://tile.openstreetmap.org/14/8650/5412.png": tile not downloaded yet`
		at      = "  At: /Users/x/go/pkg/mod/fyne.io/x/fyne@v0/widget/map.go:389"
	)

	t.Run("drops a whole pending-tile block", func(t *testing.T) {
		if got := logLines(t, header, pending, at); got != "" {
			t.Errorf("filter passed %q, want nothing", got)
		}
	})

	t.Run("drops every block of a burst", func(t *testing.T) {
		if got := logLines(t, header, pending, at, header, pending, at); got != "" {
			t.Errorf("filter passed %q, want nothing", got)
		}
	})

	t.Run("passes everything else through", func(t *testing.T) {
		got := logLines(t, "Fyne error:  could not read file", "  Cause: no such file", at)

		for _, want := range []string{"could not read file", "no such file", "map.go:389"} {
			if !strings.Contains(got, want) {
				t.Errorf("filter dropped %q from %q", want, got)
			}
		}
	})

	// Only errTilePending is this package's own noise. A "tile fetch error"
	// from anything else is a real fault, and its cause and location have
	// to survive.
	t.Run("keeps a tile error with another cause", func(t *testing.T) {
		got := logLines(t, header, "  Cause: png: invalid format", at)

		if !strings.Contains(got, "png: invalid format") || !strings.Contains(got, "map.go:389") {
			t.Errorf("filter passed %q, want the cause and location kept", got)
		}
	})

	t.Run("does not swallow a line following a partial block", func(t *testing.T) {
		got := logLines(t, header, pending, "Fyne error:  something else")

		if !strings.Contains(got, "something else") {
			t.Errorf("filter passed %q, want the unrelated error kept", got)
		}
	})

	t.Run("is safe to write to from several goroutines", func(t *testing.T) {
		f := &tileLogFilter{out: &bytes.Buffer{}}

		var wg sync.WaitGroup
		for range 8 {

			wg.Go(func() {

				for range 50 {
					_, _ = f.Write([]byte("Fyne error:  tile fetch error\n"))
				}
			})
		}

		wg.Wait()
	})
}

// The filter's whole design rests on the shape fyne.LogError writes, so
// pin it against the real thing rather than a hand-written imitation: a
// future Fyne that logs a pending tile differently has to be noticed here.
func TestTileLogFilter_SwallowsARealLogErrorCall(t *testing.T) {
	var out bytes.Buffer

	restore := log.Writer()
	log.SetOutput(&tileLogFilter{out: &out})
	t.Cleanup(func() { log.SetOutput(restore) })

	fyne.LogError(tileFetchError, errTilePending)

	if got := out.String(); got != "" {
		t.Errorf("a pending tile logged %q, want nothing", got)
	}

	fyne.LogError("something real", errors.New("boom"))

	if got := out.String(); !strings.Contains(got, "boom") {
		t.Errorf("a real error logged %q, want it kept", got)
	}
}

func TestNewTileFetcher_InstallsTheLogFilter(t *testing.T) {
	newTileFetcher(osmTiles, nil)

	if _, ok := log.Writer().(*tileLogFilter); !ok {
		t.Errorf("log.Writer() is %T, want the tile log filter installed", log.Writer())
	}
}
