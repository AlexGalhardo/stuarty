package providertest

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestMain(m *testing.M) { Main(m) }

func TestServeReturnsBinaryBodyWithTheDeclaredHeaders(t *testing.T) {
	t.Parallel()

	data := []byte{0, 1, 2, 255}
	recorder := Serve(t, t.TempDir(), Exchange{
		Method: http.MethodGet, Path: "/file", Status: http.StatusPartialContent,
		Body: data, ContentType: "application/octet-stream",
		ResponseHeader: map[string]string{"Content-Range": "bytes 0-3/8"},
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, recorder.Endpoint("/file"), http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	res, err := recorder.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	got, err := io.ReadAll(res.Body)
	if err != nil || !bytes.Equal(got, data) || res.StatusCode != http.StatusPartialContent ||
		res.Header.Get(
			"Content-Type",
		) != "application/octet-stream" || res.Header.Get("Content-Range") != "bytes 0-3/8" {
		t.Fatalf("response = %d %v %v, %v", res.StatusCode, res.Header, got, err)
	}
}

func TestOnlyLoopbackAddressesAreAllowed(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"localhost", "127.0.0.1:80", "[::1]:80"} {
		if err := allowLoopback("", address); err != nil {
			t.Errorf("%q: %v", address, err)
		}
	}
	for _, address := range []string{"example.test:80", "192.0.2.1", "[2001:db8::1]:80"} {
		if err := allowLoopback("", address); err == nil {
			t.Errorf("%q was allowed", address)
		}
	}
}
