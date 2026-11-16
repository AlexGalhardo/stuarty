package shell

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"
)

// The handshake and launch routes of the server (SDD-007 sections 3.4 and 3.8).
// The proof format is HMAC-SHA256(token, "vukem-video-handshake-v1" + nonce).
const (
	handshakePath  = "/api/v1/handshake"
	launchPath     = "/api/v1/system/launch"
	handshakeLabel = "vukem-video-handshake-v1"
	launchTimeout  = 10 * time.Second
	launcherHeader = "Vukem-Video-Launcher"
)

// newLauncherSecret is 32 random bytes as hexadecimal: the proof that a launch request comes from
// the shell that started the server (SDD-007 section 5.3).
func newLauncherSecret() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// ErrImpostor means the server did not prove that it knows the workspace token.
var ErrImpostor = errors.New("the server did not prove that it knows the workspace token; the token was not sent")

// LaunchURL asks the server for a one-time launch address. The webview opens it
// and the server answers with the session cookie, so the window is signed in
// without the page ever seeing the local token (ADR-0019 item 2). The token is
// sent only after the server proved that it knows it.
func (s *Server) LaunchURL(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	client := &http.Client{Timeout: launchTimeout} //nolint:forbidigo // loopback only: the child server

	if err := s.verify(ctx, client); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL()+launchPath, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)
	req.Header.Set(launcherHeader, s.launcher)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		URL string `json:"url"`
	}
	if resp.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&out) != nil || out.URL == "" {
		return "", &StartupError{
			Reason:  ReasonRuntime,
			Message: "The server refused the launch request: " + resp.Status,
		}
	}
	// The address comes from the server: it must still be the server we started.
	if !Allowed(out.URL, s.Port) {
		return "", &StartupError{
			Reason:  ReasonRuntime,
			Message: "The server returned an address outside its own origin",
		}
	}
	return out.URL, nil
}

func (s *Server) verify(ctx context.Context, client *http.Client) error { //nolint:forbidigo // loopback only
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return err
	}
	nonce := hex.EncodeToString(raw[:])
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		s.URL()+handshakePath+"?nonce="+url.QueryEscape(nonce), http.NoBody)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Proof string `json:"proof"`
	}
	if resp.StatusCode != http.StatusOK ||
		json.NewDecoder(io.LimitReader(resp.Body, 1<<10)).Decode(&body) != nil {
		return ErrImpostor
	}
	m := hmac.New(sha256.New, []byte(s.Token))
	m.Write([]byte(handshakeLabel + nonce))
	if !hmac.Equal([]byte(hex.EncodeToString(m.Sum(nil))), []byte(body.Proof)) {
		return ErrImpostor
	}
	return nil
}
