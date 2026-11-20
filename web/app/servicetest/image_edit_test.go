package servicetest

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"net/http"
	"strings"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	"github.com/AlexGalhardo/stuarty/service"
)

// Epic 30 (SDD-016 section 6.4): the job image.edit removes the background of a layer, or generates an image
// into a document, with the provider of a connection. The provider is the fake server of generate_test.go, which
// answers POST /images/edits as editMode says.

// editPNG is a real PNG of w by h: opaque, or with its left half transparent, like a cut-out.
func editPNG(w, h int, transparent bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	for x := range w {
		for y := range h {
			a := uint8(255)
			if transparent && x < w/2 {
				a = 0
			}

			img.SetNRGBA(x, y, color.NRGBA{R: byte(x * 3), G: byte(y * 5), B: 120, A: a})
		}
	}

	var b bytes.Buffer

	_ = png.Encode(&b, img)

	return b.Bytes()
}

func (f *fakeMedia) setEdit(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.editMode = mode
}

// answerEdit answers POST /images/edits: a PNG of the size of the picture it was sent, transparent unless the
// test asks for another answer.
func (f *fakeMedia) answerEdit(w http.ResponseWriter, body []byte) {
	f.mu.Lock()
	mode, foreign := f.editMode, f.foreign
	f.mu.Unlock()

	var req struct {
		Images []struct {
			ImageURL string `json:"image_url"`
		} `json:"images"`
	}

	_ = json.Unmarshal(body, &req)

	width, height := 8, 8

	if len(req.Images) == 1 {
		_, b64, _ := strings.Cut(req.Images[0].ImageURL, ";base64,")
		if raw, err := base64.StdEncoding.DecodeString(b64); err == nil {
			if cfg, _, derr := image.DecodeConfig(bytes.NewReader(raw)); derr == nil {
				width, height = cfg.Width, cfg.Height
			}
		}
	}

	answer := func(data map[string]string) {
		raw, err := json.Marshal(struct {
			Created int                 `json:"created"`
			Data    []map[string]string `json:"data"`
		}{1, []map[string]string{data}})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		_, _ = w.Write(raw)
	}

	switch mode {
	case "refused":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"content_policy_violation","type":"invalid_request_error"}}`))
	case "opaque":
		answer(map[string]string{"b64_json": base64.StdEncoding.EncodeToString(editPNG(width, height, false))})
	case "square":
		answer(map[string]string{"b64_json": base64.StdEncoding.EncodeToString(editPNG(40, 40, true))})
	case "notimage":
		answer(map[string]string{"b64_json": base64.StdEncoding.EncodeToString([]byte("not a picture"))})
	case "foreign":
		answer(map[string]string{"url": foreign + "/files/x.png"})
	case "echo":
		answer(map[string]string{
			"b64_json":       base64.StdEncoding.EncodeToString(editPNG(width, height, true)),
			"revised_prompt": "the key is " + apiKey,
		})
	default:
		answer(map[string]string{"b64_json": base64.StdEncoding.EncodeToString(editPNG(width, height, true))})
	}
}

// editEnv is a generation environment with the real ingest (a layer needs a ready image), a connection that
// accepts pictures, an image document with one photo layer, and a price per image.
type editEnv struct {
	*genEnv

	imageConn string
	image     string
	photo     string // the content hash of the picture of the layer
	layer     string
}

const photoLayer = "lyr_0000000099"

func newEditEnv(tb testing.TB) *editEnv {
	tb.Helper()

	g := newGenEnv(tb, mediaCfg{realExport: true, ingest: ingest.Run})
	g.setPrice(imageModel, "image", 40_000)

	c, err := g.svc.ConnectionCreate(g.as(ui), service.ConnectionCreateRequest{
		Kind: "openai", Label: "my openai with pictures", Endpoint: g.api.srv.URL + "/v1",
		Accepts: []string{"text", "image"}, Secret: apiKey,
	})
	if err != nil {
		tb.Fatal(err)
	}

	e := &editEnv{genEnv: g, imageConn: c.ID, layer: photoLayer}

	imp := g.importOne(g.prj, g.write("photo.png", string(editPNG(64, 48, false))))
	if len(imp.Assets) != 1 || imp.Assets[0].State != "ready" || imp.Assets[0].Hash == "" {
		tb.Fatalf("the photo did not become a ready image: %+v", imp)
	}

	e.photo = imp.Assets[0].Hash

	created, err := g.svc.ImageCreate(g.as(token), service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Thumb"})
	if err != nil {
		tb.Fatal(err)
	}

	e.image = created.ImageID

	e.edit(1, op("layer.add_image", `{"asset":"`+e.photo+`","id":"`+photoLayer+
		`","x":100,"y":50,"w":600,"h":450,"name":"Photo"}`))

	return e
}

// edit commits a batch of the person's own to the image, at the revision it is at.
func (e *editEnv) edit(base int64, ops ...command.Operation) {
	e.t.Helper()

	if _, err := e.svc.ImageEdit(
		e.as(token),
		service.ImageEditRequest{ImageID: e.image, BaseRevision: base, Operations: ops},
	); err != nil {
		e.t.Fatal(err)
	}
}

func (e *editEnv) describe() *service.ImageDescribeResult {
	e.t.Helper()

	d, err := e.svc.ImageDescribe(e.as(token), service.ImageDescribeRequest{ImageID: e.image})
	if err != nil {
		e.t.Fatal(err)
	}

	return d
}

func (e *editEnv) layerOf(id string) coreimage.Layer {
	e.t.Helper()

	layers := e.describe().Layers
	for i := range layers {
		if layers[i].ID == id {
			return layers[i]
		}
	}

	e.t.Fatalf("the image has no layer %s", id)

	return coreimage.Layer{}
}

func (e *editEnv) removeBackground() (*service.Job, error) {
	return e.svc.ImageModelEdit(e.as(token), service.ImageModelEditRequest{
		ProjectID:    e.prj,
		ConnectionID: e.imageConn, ImageID: e.image, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	})
}

func (e *editEnv) generate(layer, prompt string) (*service.Job, error) {
	return e.svc.ImageModelEdit(e.as(token), service.ImageModelEditRequest{
		ProjectID:    e.prj,
		ConnectionID: e.conn, ImageID: e.image, Operation: provider.OperationGenerate, Model: imageModel,
		LayerID: layer, Prompt: prompt,
	})
}

func sha(b []byte) string {
	s := sha256.Sum256(b)

	return "sha256:" + hex.EncodeToString(s[:])
}

// Acceptance criteria 1 and 3 of epic 30: the same layer, the same place, another file with a transparent
// background, and one undo gives the original back; nothing leaves before the estimate was shown and approved.
func TestImageEdit_RemoveBackgroundReplacesTheLayerAndOneUndoRestoresIt(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)
	before := e.layerOf(e.layer)
	revision := e.describe().Revision

	job, err := e.removeBackground()
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobWaitingApproval || job.Kind != "image.edit" || job.Estimate == nil ||
		*job.Estimate != (core.Money{Micros: 40_000, Currency: "USD"}) {
		t.Fatalf("job = %+v (estimate %+v)", job, job.Estimate)
	}

	if e.api.edits.Load() != 0 {
		t.Fatal("the picture left before a person approved")
	}

	req, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || !strings.Contains(req.Summary, "will be sent to OpenAI") ||
		!strings.Contains(req.Summary, `"Photo"`) || !strings.Contains(req.Summary, "64 by 48") {
		t.Fatalf("the person is told what leaves: %+v, %v", req, err)
	}

	if aerr := e.approve(job.ApprovalID); aerr != nil {
		t.Fatal(aerr)
	}

	done := e.jobOf(job.JobID)
	if done.State != service.JobSucceeded || done.Result == nil || done.Result.Generated == nil {
		t.Fatalf("job after approval = %+v", done)
	}

	res := done.Result.Generated
	if res.Delivered != 1 || len(res.Items) != 1 || res.Image == nil || !res.Image.Applied ||
		res.Image.LayerID != e.layer || res.Image.ImageID != e.image || res.Image.Revision != revision+1 ||
		res.Cost != (core.Money{Micros: 40_000, Currency: "USD"}) || len(res.Notes) != 0 {
		t.Fatalf("result = %+v (image %+v)", res, res.Image)
	}

	after := e.layerOf(e.layer)
	if after.ID != before.ID || after.X != before.X || after.Y != before.Y || after.W != before.W ||
		after.H != before.H || after.Name != before.Name || after.Asset == before.Asset || after.Asset != res.Items[0].Hash {
		t.Fatalf("the layer was %+v and is %+v", before, after)
	}

	// The file is a cut-out: a PNG with a transparent pixel, an asset with a complete origin.
	got := e.asset(res.Items[0].Hash)
	o := got.Origin

	if got.Kind != "image" || o.Kind != "generated" || o.Provider != "openai" || o.Model != imageModel ||
		o.Prompt != "remove background" || o.CostMicros == nil || *o.CostMicros != 40_000 ||
		o.Source != "job "+job.JobID+" of "+e.photo || got.CreatedBy != token {
		t.Fatalf("origin = %+v, kind %s, by %s", o, got.Kind, got.CreatedBy)
	}

	// What the provider received is the file of the layer, under the key, with a transparent background asked.
	e.api.mu.Lock()
	sent := e.api.bodies[len(e.api.bodies)-1]
	auth := e.api.auths[len(e.api.auths)-1]
	e.api.mu.Unlock()

	var body struct {
		Model      string `json:"model"`
		Background string `json:"background"`
		Format     string `json:"output_format"`
		N          int    `json:"n"`
		Images     []struct {
			URL string `json:"image_url"`
		} `json:"images"`
	}

	if err := json.Unmarshal(sent, &body); err != nil || body.Model != imageModel || body.Background != "transparent" ||
		body.Format != "png" || body.N != 1 || len(body.Images) != 1 {
		t.Fatalf("the provider received %.300s (%v)", sent, err)
	}

	_, b64, _ := strings.Cut(body.Images[0].URL, "base64,")

	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || sha(raw) != e.photo || auth != "Bearer "+apiKey {
		t.Fatalf("the picture sent is not the file of the layer (%v), or the key is wrong: %q", err, auth)
	}

	if n := e.spentToday(); n != 40_000 {
		t.Errorf("spent today = %d, want 40000", n)
	}

	// One undo of the person who asked puts the original file back, and the original file was never changed.
	undone, err := e.svc.ImageUndo(e.as(token), service.ImageStepRequest{ImageID: e.image})
	if err != nil || !undone.Applied {
		t.Fatalf("undo: %+v, %v", undone, err)
	}

	back := e.layerOf(e.layer)
	if back.Asset != e.photo || back.X != before.X || back.W != before.W || back.H != before.H {
		t.Fatalf("after the undo the layer is %+v, want the first one", back)
	}

	if e.asset(e.photo).Origin.Kind == "generated" {
		t.Error("the original file became a generated one")
	}
}

// A file of another proportion is contained in the old box: the layer is not stretched (SDD-021 section 3.2).
func TestImageEdit_AResultOfAnotherShapeIsNotStretched(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)
	e.api.setEdit("square")

	job := e.approved(e.removeBackground())
	if job.State != service.JobSucceeded || job.Result.Generated.Image == nil || !job.Result.Generated.Image.Applied {
		t.Fatalf("job = %+v", job)
	}

	l := e.layerOf(e.layer)
	// the box was 100,50 600x450; a square fits as 450x450, centred on it
	if l.W != 450 || l.H != 450 || l.X != 175 || l.Y != 50 {
		t.Fatalf("box %d,%d %dx%d", l.X, l.Y, l.W, l.H)
	}
}

// Acceptance criterion 2: Generate adds a new image layer whose asset has a complete origin; it covers the layer
// the person named.
func TestImageEdit_GenerateAddsALayerWithACompleteOrigin(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)
	revision := e.describe().Revision

	job, err := e.generate(e.layer, "A snowy forest, soft light")
	if err != nil {
		t.Fatal(err)
	}

	if job.State != service.JobWaitingApproval || job.Estimate == nil || job.Estimate.Micros != 40_000 {
		t.Fatalf("job = %+v", job)
	}

	req, err := e.svc.ApprovalGet(e.as(token), service.ApprovalRef{ID: job.ApprovalID})
	if err != nil || !strings.Contains(req.Summary, "Only the text below is sent") ||
		!strings.Contains(req.Summary, "A snowy forest") {
		t.Fatalf("summary = %+v, %v", req, err)
	}

	if e.providerCalls() != 0 {
		t.Fatal("the provider was called before approval")
	}

	done := e.approved(job, nil)
	res := done.Result.Generated

	if done.State != service.JobSucceeded || res.Image == nil || !res.Image.Applied || res.Image.LayerID == "" ||
		res.Image.Revision != revision+1 || len(res.Items) != 1 {
		t.Fatalf("job = %+v", done)
	}

	l := e.layerOf(res.Image.LayerID)
	if l.Kind != coreimage.KindImage || l.Asset != res.Items[0].Hash || l.Name != "Generated" {
		t.Fatalf("layer = %+v", l)
	}

	// the request for a picture that covers a 600 by 450 box asks for the nearest shape of the model
	e.api.mu.Lock()
	sent := e.api.bodies[len(e.api.bodies)-1]
	e.api.mu.Unlock()

	var body struct {
		Prompt string `json:"prompt"`
		Size   string `json:"size"`
		N      int    `json:"n"`
	}

	if err := json.Unmarshal(sent, &body); err != nil || body.Prompt != "A snowy forest, soft light" ||
		body.Size != "1536x1024" || body.N != 1 {
		t.Fatalf("the provider received %s (%v)", sent, err)
	}

	o := e.asset(res.Items[0].Hash).Origin
	if o.Kind != "generated" || o.Provider != "openai" || o.Model != imageModel ||
		o.Prompt != "A snowy forest, soft light" || o.CostMicros == nil || *o.CostMicros != 40_000 ||
		!strings.HasPrefix(o.Source, "job ") {
		t.Fatalf("origin = %+v", o)
	}

	// One undo takes the new layer away again.
	if _, err := e.svc.ImageUndo(e.as(token), service.ImageStepRequest{ImageID: e.image}); err != nil {
		t.Fatal(err)
	}

	for _, l := range e.describe().Layers {
		if l.ID == res.Image.LayerID {
			t.Fatal("the generated layer is still there after the undo")
		}
	}
}

func TestImageEdit_GenerateCoversTheCanvasWhenNoLayerIsNamed(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)

	done := e.approved(e.generate("", "A red fox"))
	if done.State != service.JobSucceeded || done.Result.Generated.Image == nil ||
		!done.Result.Generated.Image.Applied {
		t.Fatalf("job = %+v", done)
	}

	l := e.layerOf(done.Result.Generated.Image.LayerID)
	d := e.describe()

	if l.W < d.Width || l.H < d.Height && l.W < d.Width {
		t.Fatalf("a layer of %dx%d does not cover the canvas %dx%d", l.W, l.H, d.Width, d.Height)
	}
}

// A dry run answers the job it would create and the estimate, and sends nothing.
func TestImageEdit_DryRunShowsTheEstimateAndSendsNothing(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)

	job, err := e.svc.ImageModelEdit(e.as(token), service.ImageModelEditRequest{
		ProjectID: e.prj, DryRun: true,
		ConnectionID: e.imageConn, ImageID: e.image, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	})
	if err != nil || job.Estimate == nil || job.Estimate.Micros != 40_000 {
		t.Fatalf("job = %+v, %v", job, err)
	}

	if e.api.edits.Load() != 0 {
		t.Fatal("a dry run sent the picture")
	}

	if list, lerr := e.svc.ApprovalList(e.as(token), service.ApprovalListRequest{State: "all"}); lerr != nil ||
		list.Total != 0 {
		t.Errorf("a dry run left requests: %+v, %v", list, lerr)
	}
}

// A provider that answers a picture with no transparent pixel did not remove the background: the layer is not
// changed, the file is kept and paid for, and the job says so.
func TestImageEdit_AnOpaqueResultIsKeptAndNotApplied(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)
	e.api.setEdit("opaque")

	revision := e.describe().Revision

	done := e.approved(e.removeBackground())
	res := done.Result.Generated

	if done.State != service.JobSucceeded || res.Image == nil || res.Image.Applied || len(res.Items) != 1 ||
		len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "no transparent pixel") ||
		!strings.Contains(res.Notes[0], res.Items[0].AssetID) {
		t.Fatalf("result = %+v", res)
	}

	if e.layerOf(e.layer).Asset != e.photo || e.describe().Revision != revision {
		t.Fatal("the layer or the revision changed")
	}

	if n := e.spentToday(); n != 40_000 {
		t.Errorf("the provider billed the edit: spent %d", n)
	}
}

// A layer that was changed by another actor while the job waited is a conflict: nothing is overwritten, the file
// stays in the library, and the job succeeds with the reason.
func TestImageEdit_ALayerChangedMeanwhileIsAConflictNotAnOverwrite(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)

	job, err := e.removeBackground()
	if err != nil {
		t.Fatal(err)
	}

	// another actor moves the layer while the person decides
	if _, err := e.svc.ImageEdit(e.as(ui), service.ImageEditRequest{
		ImageID: e.image, BaseRevision: e.describe().Revision,
		Operations: []command.Operation{op("layer.transform", `{"layer_id":"`+e.layer+`","x":300}`)},
	}); err != nil {
		t.Fatal(err)
	}

	if aerr := e.approve(job.ApprovalID); aerr != nil {
		t.Fatal(aerr)
	}

	done := e.jobOf(job.JobID)
	res := done.Result.Generated

	if done.State != service.JobSucceeded || res.Image == nil || res.Image.Applied || len(res.Items) != 1 ||
		len(res.Notes) != 1 || !strings.Contains(res.Notes[0], command.CodeRevisionConflict) {
		t.Fatalf("job = %+v, notes %v", done, res.Notes)
	}

	if l := e.layerOf(e.layer); l.Asset != e.photo || l.X != 300 {
		t.Fatalf("the layer was overwritten: %+v", l)
	}
}

func TestImageEdit_ArgumentsAreChecked(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)

	base := service.ImageModelArgs{
		ConnectionID: e.imageConn, ImageID: e.image, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	}

	for name, tc := range map[string]struct {
		mutate func(a *service.ImageModelArgs)
		path   string
	}{
		"an unknown operation":       {func(a *service.ImageModelArgs) { a.Operation = "upscale" }, "args.operation"},
		"no layer":                   {func(a *service.ImageModelArgs) { a.LayerID = "" }, "args.layer_id"},
		"a layer that is not there":  {func(a *service.ImageModelArgs) { a.LayerID = "lyr_0000000001" }, "args.layer_id"},
		"a prompt for a removal":     {func(a *service.ImageModelArgs) { a.Prompt = "x" }, "args.prompt"},
		"a bad image id":             {func(a *service.ImageModelArgs) { a.ImageID = "nope" }, "args.image_id"},
		"an image that is not there": {func(a *service.ImageModelArgs) { a.ImageID = "img_0000000001" }, ""},
		"a connection that takes text only": {
			func(a *service.ImageModelArgs) { a.ConnectionID = e.conn }, "args.connection_id",
		},
		"no connection": {func(a *service.ImageModelArgs) { a.ConnectionID = "" }, "args.connection_id"},
	} {
		a := base
		tc.mutate(&a)

		_, err := e.svc.ImageModelEdit(e.as(token), service.ImageModelEditRequest{ProjectID: e.prj, ImageModelArgs: a})

		se, ok := svcErr(err)
		if !ok || (tc.path != "" && se.Path != tc.path) {
			t.Errorf("%s: got %v", name, err)
		}
	}

	// generate needs a prompt
	_, err := e.generate("", "")
	if se, ok := svcErr(err); !ok || se.Path != "args.prompt" {
		t.Errorf("generate with no prompt: %v", err)
	}

	if e.api.edits.Load() != 0 || e.providerCalls() != 0 {
		t.Fatal("a refused request reached the provider")
	}
}

// A picture the ingest takes and the provider does not (a GIF) is refused at the estimate, before a person is
// asked to approve a job that could not send it.
func TestImageEdit_APictureTheProviderCannotTakeIsRefusedFirst(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)

	var b bytes.Buffer

	frame := image.NewPaletted(image.Rect(0, 0, 32, 24), color.Palette{color.Black, color.White})
	if err := gif.Encode(&b, frame, nil); err != nil {
		t.Fatal(err)
	}

	imp := e.importOne(e.prj, e.write("still.gif", b.String()))
	if len(imp.Assets) != 1 || imp.Assets[0].State != "ready" {
		t.Fatalf("the GIF did not become a ready image: %+v", imp)
	}

	e.edit(e.describe().Revision, op("layer.add_image", `{"asset":"`+imp.Assets[0].Hash+
		`","id":"lyr_0000000098","x":0,"y":0,"w":320,"h":240,"name":"Gif"}`))

	_, err := e.svc.ImageModelEdit(e.as(token), service.ImageModelEditRequest{
		ProjectID:    e.prj,
		ConnectionID: e.imageConn, ImageID: e.image, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: "lyr_0000000098",
	})
	if se, ok := svcErr(err); !ok || se.Code != "E_VALIDATION" || se.Path != "args.layer_id" ||
		!strings.Contains(se.Message, "not a PNG, a JPEG or a WebP") {
		t.Fatalf("a GIF layer = %v, want E_VALIDATION at args.layer_id", err)
	}

	if list, _ := e.svc.ApprovalList(e.as(token), service.ApprovalListRequest{State: "all"}); list.Total != 0 {
		t.Errorf("a picture that cannot be sent left %d approval requests", list.Total)
	}

	if e.api.edits.Load() != 0 {
		t.Fatal("a picture that cannot be sent reached the provider")
	}
}

// SDD-016 section 12.2: a sign-in grants text only, so neither operation of image.edit runs through a ChatGPT
// connection, even with a price set for the model: refused before a job or an approval exists, with the hint
// to use an API key.
func TestImageEdit_ASignInConnectionIsRefused(t *testing.T) {
	t.Parallel()

	se := newSignInEnv(t)
	done := se.signIn("openai", "")

	micros := int64(40_000)
	if _, err := se.svc.ProviderPriceSet(se.asPerson(), service.ProviderPriceSetRequest{
		Provider: "openai", Model: imageModel, Currency: "USD", Unit: "image", UnitMicros: &micros,
	}); err != nil {
		t.Fatal(err)
	}

	created, err := se.svc.ImageCreate(se.as(token), service.ImageCreateRequest{Preset: "video_thumbnail"})
	if err != nil {
		t.Fatal(err)
	}

	for _, a := range []service.ImageModelArgs{
		{Operation: provider.OperationRemoveBackground, LayerID: photoLayer},
		{Operation: provider.OperationGenerate, Prompt: "a beach"},
	} {
		a.ConnectionID, a.ImageID, a.Model = done.ConnectionID, created.ImageID, imageModel

		_, err := se.svc.ImageModelEdit(se.as(token), service.ImageModelEditRequest{
			ProjectID: se.prj, ImageModelArgs: a,
		})

		e, ok := svcErr(err)
		if !ok || e.Code != "E_VALIDATION" || e.Path != "args.connection_id" || !strings.Contains(e.Hint, "API key") {
			t.Errorf("%s through a sign-in = %v, want E_VALIDATION at args.connection_id naming an API key",
				a.Operation, err)
		}
	}

	approvals, _ := se.svc.ApprovalList(se.as(token), service.ApprovalListRequest{State: "all"})
	if len(approvals.Requests) != 0 {
		t.Errorf("a refused image.edit left %d approval requests", len(approvals.Requests))
	}
}
