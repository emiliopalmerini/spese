package spa

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerCacheAndFallbackBoundaries(t *testing.T) {
	root := fstest.MapFS{
		"dist/index.html":             {Data: []byte("<main>spa</main>")},
		"dist/assets/index-abc123.js": {Data: []byte("console.log('spa')")},
	}
	handler, err := New(fs.FS(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path, cache string
		status      int
	}{
		{"/movimenti", "no-cache, no-store, must-revalidate", http.StatusOK},
		{"/assets/index-abc123.js", "public, max-age=31536000, immutable", http.StatusOK},
		{"/api/v1/missing", "", http.StatusNotFound},
		{"/healthz/missing", "", http.StatusNotFound},
		{"/typo", "", http.StatusNotFound},
	} {
		t.Run(test.path, func(t *testing.T) {
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, test.path, nil))
			if res.Code != test.status {
				t.Fatalf("status = %d, want %d", res.Code, test.status)
			}
			if got := res.Header().Get("Cache-Control"); got != test.cache {
				t.Fatalf("cache = %q, want %q", got, test.cache)
			}
		})
	}
}
