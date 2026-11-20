package destinations

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// filesConfig is the secret of a connection of the kind blog_files (SDD-020 section 42).
type filesConfig struct {
	Folder      string `json:"folder"`
	Template    string `json:"template,omitempty"`
	FrontMatter string `json:"front_matter,omitempty"`
	Zip         bool   `json:"zip,omitempty"`
}

func (c *filesConfig) template() string {
	if c.Template == "" {
		return writing.TemplateAstro
	}

	return c.Template
}

func parseFilesConfig(secret []byte) (filesConfig, error) {
	var c filesConfig
	if err := strictJSON(secret, &c); err != nil {
		return c, err
	}

	if !filepath.IsAbs(c.Folder) || strings.ContainsRune(c.Folder, 0) {
		return c, errors.New("the folder is an absolute path")
	}

	if err := writing.CheckTemplate(c.template(), c.FrontMatter); err != nil {
		return c, err
	}

	return c, nil
}

// strictJSON reads one JSON object and refuses a member the settings do not name.
func strictJSON(data []byte, into any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	if err := dec.Decode(into); err != nil {
		return errors.New("the settings are not a JSON object with the members the destination names")
	}

	if dec.More() {
		return errors.New("the settings have text after the JSON object")
	}

	return nil
}

type files struct{ cfg Config }

func newFiles(cfg Config) *files { return &files{cfg: cfg} }

func (f *files) Describe() Description {
	return Description{
		Kind: KindFiles, Name: "Blog: files", Outbound: false,
		Hints: hints(map[string]string{
			social.KindInvalid: "check that the folder of the connection exists and can be written, then send again",
		}),
	}
}

func (f *files) ValidateConfig(secret []byte, _ string) error {
	c, err := parseFilesConfig(secret)
	if err != nil {
		return err
	}

	if st, serr := os.Stat(c.Folder); serr != nil || !st.IsDir() {
		return errors.New("the folder does not exist")
	}

	return nil
}

func (f *files) Test(_ context.Context, c connections.Credential) (connections.TestResult, error) {
	if problem := f.problem(c.Secret); problem != "" {
		return connections.TestResult{Message: problem}, nil
	}

	return connections.TestResult{OK: true, Message: "the folder exists; nothing was written."}, nil
}

// problem says in a sentence what is wrong with the settings of a connection, or is empty.
func (f *files) problem(secret []byte) string {
	cfg, err := parseFilesConfig(secret)
	if err != nil {
		return "the settings of the connection are not valid."
	}

	if st, serr := os.Stat(cfg.Folder); serr != nil || !st.IsDir() {
		return "the folder of the connection does not exist."
	}

	return ""
}

// Send writes <folder>/<slug>/index.md and images/, or <folder>/<slug>.zip, replacing an earlier version.
func (f *files) Send(ctx context.Context, in *SendInput) (Result, error) {
	cfg, err := parseFilesConfig(in.Cred.Secret)
	if err != nil {
		return Result{}, fail(social.KindInvalid, KindFiles, "bad_settings", 0, err)
	}

	p := &in.Package
	if cerr := p.Check(); cerr != nil {
		return Result{}, fail(social.KindInvalid, KindFiles, "bad_package", 0, cerr)
	}

	if st, serr := os.Stat(cfg.Folder); serr != nil || !st.IsDir() {
		return Result{}, fail(social.KindInvalid, KindFiles, "folder_missing", 0, serr)
	}

	index, err := f.index(&cfg, p)
	if err != nil {
		return Result{}, fail(social.KindInvalid, KindFiles, "bad_template", 0, err)
	}

	target := filepath.Join(cfg.Folder, p.Slug)
	if cfg.Zip {
		target += ".zip"
	}

	if !within(cfg.Folder, target) {
		return Result{}, fail(social.KindInvalid, KindFiles, "outside_folder", 0, nil)
	}

	if st, lerr := os.Lstat(target); lerr == nil && (st.Mode()&os.ModeSymlink != 0 || st.IsDir() == cfg.Zip) {
		return Result{}, fail(social.KindInvalid, KindFiles, "not_a_plain_target", 0, nil)
	}

	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	if cfg.Zip {
		err = f.writeZip(cfg.Folder, target, index, in)
	} else {
		err = f.writeTree(cfg.Folder, target, index, in)
	}

	if err != nil {
		return Result{}, fail(social.KindInvalid, KindFiles, "write_failed", 0, err)
	}

	return Result{RemoteID: p.Slug, URL: target, State: p.Status}, nil
}

func (f *files) index(cfg *filesConfig, p *Package) ([]byte, error) {
	v := writing.FrontValues{Date: f.cfg.now(), Draft: p.Status == StatusDraft}
	if c := p.Cover(); c != nil {
		v.Cover = "images/" + c.Name
	}

	fm, err := writing.FrontMatter(cfg.template(), cfg.FrontMatter, &p.Meta, v)
	if err != nil {
		return nil, err
	}

	md := p.Markdown
	if !strings.HasSuffix(md, "\n") {
		md += "\n"
	}

	return []byte(fm + "\n" + md), nil
}

// within says whether path is inside folder once both are cleaned.
func within(folder, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(folder), filepath.Clean(path))

	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}

func randomSuffix() string {
	var b [6]byte
	_, _ = rand.Read(b[:])

	return hex.EncodeToString(b[:])
}

// copyChecked copies an image to w and checks its size and digest against what the person approved.
func copyChecked(in *SendInput, im Image, w io.Writer) error {
	r, err := in.Open(im)
	if err != nil {
		return err
	}

	defer func() { _ = r.Close() }()

	h := sha256.New()

	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(r, im.Bytes+1))
	if err != nil {
		return err
	}

	if n != im.Bytes || hex.EncodeToString(h.Sum(nil)) != im.SHA256 {
		return errors.New("an image is not the file that was approved")
	}

	return nil
}

func (f *files) writeTree(folder, target string, index []byte, in *SendInput) error {
	tmp := filepath.Join(folder, ".stuarty-"+randomSuffix())
	if err := os.Mkdir(tmp, 0o750); err != nil {
		return err
	}

	defer func() { _ = os.RemoveAll(tmp) }()

	if err := os.WriteFile(filepath.Join(tmp, "index.md"), index, 0o600); err != nil {
		return err
	}

	if len(in.Package.Images) > 0 {
		if err := os.Mkdir(filepath.Join(tmp, "images"), 0o750); err != nil {
			return err
		}
	}

	for i, im := range in.Package.Images {
		if err := writeImage(filepath.Join(tmp, "images", im.Name), in, im); err != nil {
			return err
		}

		if in.Report != nil {
			in.Report(float64(i+1)/float64(len(in.Package.Images)+1), "writing the images")
		}
	}

	old := ""

	if _, err := os.Lstat(target); err == nil {
		old = filepath.Join(folder, ".stuarty-old-"+randomSuffix())
		if err := os.Rename(target, old); err != nil {
			return err
		}
	}

	if err := os.Rename(tmp, target); err != nil {
		if old != "" {
			_ = os.Rename(old, target)
		}

		return err
	}

	if old != "" {
		_ = os.RemoveAll(old)
	}

	return nil
}

func writeImage(path string, in *SendInput, im Image) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	if err := copyChecked(in, im, out); err != nil {
		_ = out.Close()

		return err
	}

	return out.Close()
}

func (f *files) writeZip(folder, target string, index []byte, in *SendInput) error {
	tmp := filepath.Join(folder, ".stuarty-"+randomSuffix()+".zip")

	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	defer func() { _ = os.Remove(tmp) }()

	zw := zip.NewWriter(out)
	when := f.cfg.now().UTC().Truncate(2 * time.Second)

	add := func(name string) (io.Writer, error) {
		return zw.CreateHeader(&zip.FileHeader{Name: in.Package.Slug + "/" + name, Method: zip.Deflate, Modified: when})
	}

	w, err := add("index.md")
	if err == nil {
		_, err = w.Write(index)
	}

	for _, im := range in.Package.Images {
		if err != nil {
			break
		}

		if w, err = add("images/" + im.Name); err == nil {
			err = copyChecked(in, im, w)
		}
	}

	if err == nil {
		err = zw.Close()
	}

	if cerr := out.Close(); err == nil {
		err = cerr
	}

	if err != nil {
		return err
	}

	_ = os.Remove(target)

	return os.Rename(tmp, target)
}
