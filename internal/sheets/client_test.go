package sheets

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/option"
	sheetsapi "google.golang.org/api/sheets/v4"
)

func TestReadRangeUsesETagValidation(t *testing.T) {
	t.Parallel()

	var calls int
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++

		switch calls {
		case 1:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Fatalf("first request If-None-Match = %q, want empty", got)
			}
			return jsonResponse(r, http.StatusOK, `"v1"`, `{"range":"accounts","values":[["Name"],["Checking"]]}`), nil
		case 2:
			if got := r.Header.Get("If-None-Match"); got != `"v1"` {
				t.Fatalf("second request If-None-Match = %q, want %q", got, `"v1"`)
			}
			return jsonResponse(r, http.StatusNotModified, "", ""), nil
		case 3:
			if got := r.Header.Get("If-None-Match"); got != `"v1"` {
				t.Fatalf("third request If-None-Match = %q, want %q", got, `"v1"`)
			}
			return jsonResponse(r, http.StatusOK, `"v2"`, `{"range":"accounts","values":[["Name"],["Savings"]]}`), nil
		default:
			t.Fatalf("unexpected request %d", calls)
			return nil, nil
		}
	})}

	client := newTestClient(t, httpClient)

	got, err := client.ReadRange(context.Background(), "accounts", false)
	if err != nil {
		t.Fatalf("first ReadRange: %v", err)
	}
	if got[1][0] != "Checking" {
		t.Fatalf("first ReadRange row = %v, want Checking", got[1])
	}

	got, err = client.ReadRange(context.Background(), "accounts", false)
	if err != nil {
		t.Fatalf("second ReadRange: %v", err)
	}
	if got[1][0] != "Checking" {
		t.Fatalf("second ReadRange row = %v, want cached Checking", got[1])
	}

	got, err = client.ReadRange(context.Background(), "accounts", false)
	if err != nil {
		t.Fatalf("third ReadRange: %v", err)
	}
	if got[1][0] != "Savings" {
		t.Fatalf("third ReadRange row = %v, want refreshed Savings", got[1])
	}
}

func TestReadRangeForceBypassesETagValidation(t *testing.T) {
	t.Parallel()

	var calls int
	httpClient := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++

		switch calls {
		case 1:
			return jsonResponse(r, http.StatusOK, `"v1"`, `{"range":"accounts","values":[["Name"],["Checking"]]}`), nil
		case 2:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Fatalf("forced request If-None-Match = %q, want empty", got)
			}
			return jsonResponse(r, http.StatusOK, `"v2"`, `{"range":"accounts","values":[["Name"],["Savings"]]}`), nil
		default:
			t.Fatalf("unexpected request %d", calls)
			return nil, nil
		}
	})}

	client := newTestClient(t, httpClient)

	if _, err := client.ReadRange(context.Background(), "accounts", false); err != nil {
		t.Fatalf("first ReadRange: %v", err)
	}
	got, err := client.ReadRange(context.Background(), "accounts", true)
	if err != nil {
		t.Fatalf("forced ReadRange: %v", err)
	}
	if got[1][0] != "Savings" {
		t.Fatalf("forced ReadRange row = %v, want refreshed Savings", got[1])
	}
}

func newTestClient(t *testing.T, httpClient *http.Client) *Client {
	t.Helper()

	svc, err := sheetsapi.NewService(
		context.Background(),
		option.WithHTTPClient(httpClient),
	)
	if err != nil {
		t.Fatalf("new sheets service: %v", err)
	}

	return &Client{
		svc:           svc,
		spreadsheetID: "spreadsheet-id",
		cache:         make(map[string]cacheEntry),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(req *http.Request, status int, etag, body string) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	if etag != "" {
		header.Set("ETag", etag)
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
