package destinations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// maxWPAnswer is how much of an answer of a site is read (SDD-020 section 45).
const maxWPAnswer = 1 << 20

// wpConfig is the secret of a connection of the kind blog_wordpress.
type wpConfig struct {
	User                string `json:"user"`
	ApplicationPassword string `json:"application_password"`
}

func parseWPConfig(secret []byte) (wpConfig, error) {
	var c wpConfig
	if err := strictJSON(secret, &c); err != nil {
		return c, err
	}

	bad := func(s string) bool { return strings.ContainsFunc(s, unicode.IsControl) }

	if n := len(c.User); n < 1 || n > 60 || bad(c.User) {
		return c, errors.New("the user name is 1 to 60 characters")
	}

	if n := len(c.ApplicationPassword); n < 8 || n > 200 || bad(c.ApplicationPassword) {
		return c, errors.New("the application password is 8 to 200 characters")
	}

	return c, nil
}

type wordpress struct{ cfg Config }

func newWordPress(cfg Config) *wordpress { return &wordpress{cfg: cfg} }

func (w *wordpress) Describe() Description {
	return Description{
		Kind: KindWordPress, Name: "Blog: WordPress", Outbound: true,
		Hints: hints(map[string]string{
			social.KindAuth: "WordPress refused the user or the application password: make a new application password " +
				"in the profile of the user in wp-admin and put it in the connection",
			social.KindInvalid: "WordPress refused a field of the post, or the address of the site is not its REST API: " +
				"check the address, turn on pretty permalinks, then send again",
		}),
	}
}

func (w *wordpress) ValidateConfig(secret []byte, _ string) error {
	_, err := parseWPConfig(secret)

	return err
}

// wpCall is one conversation with a site.
type wpCall struct {
	base   string
	auth   string
	client connections.Doer
}

func newWPCall(cred *connections.Credential, cfg *wpConfig) *wpCall {
	return &wpCall{
		base:   strings.TrimRight(cred.Connection.Endpoint, "/") + "/wp-json/wp/v2",
		auth:   "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.User+":"+cfg.ApplicationPassword)),
		client: cred.Client,
	}
}

// do sends one request and reads the answer into out when it is a 2xx. A status that is not is returned with the
// error that classifies it, so a caller can look at the status first.
func (c *wpCall) do(
	ctx context.Context, method, path string, q url.Values, body io.Reader, hdr map[string]string, out any,
) (int, error) {
	target := c.base + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return 0, fail(social.KindInvalid, KindWordPress, "bad_request", 0, err)
	}

	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Accept", "application/json")

	for k, v := range hdr {
		req.Header.Set(k, v)
	}

	res, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, outbound.ErrNoConnection) || errors.Is(err, outbound.ErrDestination) ||
			errors.Is(err, outbound.ErrNotPublic) || errors.Is(err, context.Canceled) {
			return 0, err
		}

		return 0, fail(social.KindUnavailable, KindWordPress, "network", 0, errors.New("the site could not be reached"))
	}

	defer func() { _ = res.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, maxWPAnswer))

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return res.StatusCode, wpStatusError(res.StatusCode, raw)
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return res.StatusCode, fail(social.KindInvalid, KindWordPress, "bad_answer", res.StatusCode,
				errors.New("the answer is not JSON: the address may not be the REST API of a WordPress"))
		}
	}

	return res.StatusCode, nil
}

// wpStatusError classifies a status. The text of the site is never kept: only a code that is a plain code.
func wpStatusError(status int, raw []byte) error {
	var body struct {
		Code string `json:"code"`
	}

	_ = json.Unmarshal(raw, &body)

	reason := body.Code
	if reason == "" {
		reason = "http_" + strconv.Itoa(status)
	}

	cause := errors.New("wordpress answered " + strconv.Itoa(status))

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fail(social.KindAuth, KindWordPress, reason, status, cause)
	case status == http.StatusTooManyRequests || status == http.StatusRequestTimeout || status >= 500:
		return fail(social.KindUnavailable, KindWordPress, reason, status, cause)
	case status >= 300 && status < 400:
		return fail(social.KindInvalid, KindWordPress, "redirect", status,
			errors.New("the site redirected the request: use its final address, with https, as the endpoint"))
	}

	return fail(social.KindInvalid, KindWordPress, reason, status, cause)
}

func wpSettings(secret []byte) (wpConfig, bool) {
	c, err := parseWPConfig(secret)

	return c, err == nil
}

func (w *wordpress) Test(ctx context.Context, c connections.Credential) (connections.TestResult, error) {
	cfg, valid := wpSettings(c.Secret)
	if !valid {
		return connections.TestResult{Message: "the settings of the connection are not valid."}, nil
	}

	status, err := newWPCall(
		&c,
		&cfg,
	).do(ctx, http.MethodGet, "/users/me", url.Values{"_fields": {"id"}}, nil, nil, nil)

	switch se, ok := social.AsError(err); {
	case err == nil:
		return connections.TestResult{
			OK:      true,
			Status:  status,
			Message: "the site accepted the user and the application password.",
		}, nil
	case !ok:
		return connections.TestResult{}, err
	case se.Kind == social.KindAuth:
		return connections.TestResult{
			Status:  status,
			Message: "the site refused the user name or the application password.",
		}, nil
	default:
		return connections.TestResult{
			Status:  status,
			Message: "the address did not answer as the REST API of a WordPress.",
		}, nil
	}
}

type wpIDLink struct {
	ID   int64  `json:"id"`
	Link string `json:"link"`
}

type wpMedia struct {
	ID        int64  `json:"id"`
	SourceURL string `json:"source_url"`
}

// Send creates the post at WordPress, or updates the one an earlier send made (SDD-020 section 45).
func (w *wordpress) Send(ctx context.Context, in *SendInput) (Result, error) {
	cfg, err := parseWPConfig(in.Cred.Secret)
	if err != nil {
		return Result{}, fail(social.KindInvalid, KindWordPress, "bad_settings", 0, err)
	}

	p := &in.Package
	if cerr := p.Check(); cerr != nil {
		return Result{}, fail(social.KindInvalid, KindWordPress, "bad_package", 0, cerr)
	}

	c := newWPCall(&in.Cred, &cfg)
	report := in.Report

	if report == nil {
		report = func(float64, string) {}
	}

	content := p.HTML
	featured := int64(0)

	for i, im := range p.Images {
		m, uerr := w.upload(ctx, c, in, im)
		if uerr != nil {
			return Result{}, uerr
		}

		src, serr := safeMediaURL(m.SourceURL)
		if serr != nil {
			return Result{}, serr
		}

		content = strings.ReplaceAll(content, `src="images/`+im.Name+`"`, `src="`+html.EscapeString(src)+`"`)

		if im.Role == "cover" {
			featured = m.ID
		}

		report(float64(i+1)/float64(len(p.Images)+2), "uploading the images")
	}

	tags, err := w.tags(ctx, c, p.Meta.Tags)
	if err != nil {
		return Result{}, err
	}

	report(float64(len(p.Images)+1)/float64(len(p.Images)+2), "sending the post")

	status := "draft"
	if p.Status == StatusPublished {
		status = "publish"
	}

	body := map[string]any{
		"title": p.Meta.Title, "slug": p.Slug, "content": content, "excerpt": p.Meta.Summary, "status": status,
		"tags": tags,
	}
	if featured != 0 {
		body["featured_media"] = featured
	}

	raw, merr := json.Marshal(body)
	if merr != nil {
		return Result{}, fail(social.KindInvalid, KindWordPress, "bad_package", 0, merr)
	}

	post, err := w.save(ctx, c, in.Prev, raw)
	if err != nil {
		return Result{}, err
	}

	link, _ := safeMediaURL(post.Link)

	return Result{RemoteID: strconv.FormatInt(post.ID, 10), URL: link, State: p.Status}, nil
}

// save creates the post, or updates the remembered one; when the site no longer has it (404) the post is made again.
func (w *wordpress) save(ctx context.Context, c *wpCall, prev *Remote, raw []byte) (wpIDLink, error) {
	hdr := map[string]string{"Content-Type": "application/json"}

	var post wpIDLink

	path := "/posts"

	if prev != nil {
		if id, perr := strconv.ParseInt(prev.RemoteID, 10, 64); perr == nil && id > 0 {
			path += "/" + strconv.FormatInt(id, 10)
		}
	}

	status, err := c.do(ctx, http.MethodPost, path, nil, bytes.NewReader(raw), hdr, &post)
	if err != nil && status == http.StatusNotFound && path != "/posts" {
		status, err = c.do(ctx, http.MethodPost, "/posts", nil, bytes.NewReader(raw), hdr, &post)
	}

	if err != nil {
		return post, err
	}

	if post.ID <= 0 {
		return post, fail(social.KindInvalid, KindWordPress, "bad_answer", status,
			errors.New("the answer has no id: the address may not be the REST API of a WordPress"))
	}

	return post, nil
}

func (w *wordpress) upload(ctx context.Context, c *wpCall, in *SendInput, im Image) (wpMedia, error) {
	r, err := in.Open(im)
	if err != nil {
		return wpMedia{}, fail(social.KindInvalid, KindWordPress, "image_missing", 0, err)
	}

	data, err := io.ReadAll(io.LimitReader(r, im.Bytes+1))
	_ = r.Close()

	if err != nil || int64(len(data)) != im.Bytes {
		return wpMedia{}, fail(
			social.KindInvalid,
			KindWordPress,
			"image_changed",
			0,
			errors.New("an image is not the approved file"),
		)
	}

	var m wpMedia

	hdr := map[string]string{
		"Content-Type":        im.ContentType,
		"Content-Disposition": `attachment; filename="` + im.Name + `"`,
	}

	if _, err := c.do(ctx, http.MethodPost, "/media", nil, bytes.NewReader(data), hdr, &m); err != nil {
		return wpMedia{}, err
	}

	if m.ID <= 0 {
		return wpMedia{}, fail(
			social.KindInvalid,
			KindWordPress,
			"bad_answer",
			0,
			errors.New("the media answer has no id"),
		)
	}

	return m, nil
}

// tags finds each tag by name, or makes it, and returns the ids.
func (w *wordpress) tags(ctx context.Context, c *wpCall, names []string) ([]int64, error) {
	ids := []int64{}

	for _, name := range names {
		var found []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		}

		q := url.Values{"search": {name}, "per_page": {"100"}, "_fields": {"id,name"}}
		if _, err := c.do(ctx, http.MethodGet, "/tags", q, nil, nil, &found); err != nil {
			return nil, err
		}

		id := int64(0)

		for _, t := range found {
			if strings.EqualFold(html.UnescapeString(t.Name), name) {
				id = t.ID

				break
			}
		}

		if id == 0 {
			var made struct {
				ID int64 `json:"id"`
			}

			raw, merr := json.Marshal(map[string]string{"name": name})
			if merr != nil {
				return nil, fail(social.KindInvalid, KindWordPress, "bad_package", 0, merr)
			}

			if _, err := c.do(ctx, http.MethodPost, "/tags", nil, bytes.NewReader(raw),
				map[string]string{"Content-Type": "application/json"}, &made); err != nil {
				return nil, err
			}

			id = made.ID
		}

		if id <= 0 {
			return nil, fail(social.KindInvalid, KindWordPress, "bad_answer", 0, errors.New("the tag answer has no id"))
		}

		ids = append(ids, id)
	}

	return ids, nil
}

// safeMediaURL accepts an address the site gave only when it is an http or https URL with no white space or control
// character, so nothing the site says can become markup or another scheme in the post.
func safeMediaURL(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" ||
		strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return "", fail(
			social.KindInvalid,
			KindWordPress,
			"bad_media_url",
			0,
			errors.New("the site gave an address that is not http or https"),
		)
	}

	return s, nil
}
