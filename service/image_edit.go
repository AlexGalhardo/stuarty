package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"slices"
	"strings"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// JobImageEdit is the job kind that removes the background of a layer, or generates an image into a document,
// with the provider of a connection (SDD-016 section 6.4).
const JobImageEdit = "image.edit"

const (
	// maxEditPixels bounds the picture a result of remove_background is decoded at, to look for transparency:
	// the largest canvas of an image document (SDD-021 section 12).
	maxEditPixels = coreimage.MaxCanvas * coreimage.MaxCanvas
	// editedLayerName is the name of the layer a generation adds.
	editedLayerName = "Generated"
	// removedBackgroundPrompt is the prompt the origin of a cut-out records: the adapter writes the real one.
	removedBackgroundPrompt = "remove background"
)

// ImageModelArgs are the arguments of an image.edit job (SDD-016 section 6.4).
type ImageModelArgs struct {
	ConnectionID string `json:"connection_id"`
	ImageID      string `json:"image_id"`
	// Operation is provider.OperationRemoveBackground or provider.OperationGenerate.
	Operation string `json:"operation"`
	Model     string `json:"model"`
	// LayerID is the layer whose background is removed, or the layer a generated picture must cover.
	LayerID string `json:"layer_id,omitempty"`
	// Prompt and Size are for generate only.
	Prompt string `json:"prompt,omitempty"`
	Size   string `json:"size,omitempty"`
}

// ImageModelEditRequest starts an image.edit job in a project: the arguments of the job kind, and the wait and
// the dry run of JobStart.
type ImageModelEditRequest struct {
	ProjectID string `json:"project_id"`
	ImageModelArgs
	WaitMS int  `json:"wait_ms,omitempty"`
	DryRun bool `json:"dry_run,omitempty"`
}

// ImageApplied says what the job did to the document (SDD-016 section 6.4): the layer it replaced or added, the
// revision of the batch, and whether the batch was applied.
type ImageApplied struct {
	ImageID  string `json:"image_id"`
	LayerID  string `json:"layer_id,omitempty"`
	Revision int64  `json:"revision,omitempty"`
	Applied  bool   `json:"applied"`
}

// imageEditSource is the file of the layer that goes to the provider, as Prepare read it.
type imageEditSource struct {
	Hash  string `json:"hash"`
	Bytes int64  `json:"bytes"`
}

// imageEditInput is the input column of an image.edit job: what was asked, what the document was, and the price
// the person saw.
type imageEditInput struct {
	ConnectionID string `json:"connection_id"`
	Provider     string `json:"provider"`
	ImageID      string `json:"image_id"`
	Operation    string `json:"operation"`
	LayerID      string `json:"layer_id,omitempty"`
	// Revision is the revision of the document Prepare read: the batch is applied against it, so a change made
	// meanwhile to the layer is a conflict.
	Revision int64 `json:"revision"`
	// Owner is the user of the request in network mode; the batch is committed as that user.
	Owner       string                 `json:"owner,omitempty"`
	Edit        *provider.EditRequest  `json:"edit,omitempty"`
	Image       *provider.ImageRequest `json:"image,omitempty"`
	Source      *imageEditSource       `json:"source,omitempty"`
	Price       provider.Price         `json:"price"`
	PriceSource string                 `json:"price_source"`
	Estimate    core.Money             `json:"estimate"`
}

func (s *svc) registerImageEditKind() {
	s.kinds[JobImageEdit] = &ApprovableKind{
		Name: JobImageEdit, Scope: ScopeEdit, Prepare: s.prepareImageEdit, Run: s.runImageEdit,
	}
}

// ImageModelEdit starts an image.edit job in a project and returns it. The estimate is on the job from the
// start; the job waits for a person, or for a rule, as every costly generation does (SDD-015 section 5).
func (s *svc) ImageModelEdit(ctx context.Context, req ImageModelEditRequest) (*Job, error) {
	args, err := json.Marshal(req.ImageModelArgs)
	if err != nil {
		return nil, newError(command.CodeInternal, "the request cannot be encoded.")
	}

	return s.JobStart(ctx, JobStartRequest{
		ProjectID: req.ProjectID, Kind: JobImageEdit, Args: args, WaitMS: req.WaitMS, DryRun: req.DryRun,
	})
}

// editConnection finds the connection and checks that its provider has the capability and that the connection
// accepts the kind of data the job sends (SDD-016 section 2.3).
func (s *svc) editConnection(
	ctx context.Context,
	id, capability, accept, members string,
) (connections.Connection, error) {
	if id == "" {
		return connections.Connection{}, newError(command.CodeValidation, "connection_id is required.").
			withPath("args.connection_id").
			withHint("args are connection_id, %s; provider:list names the connections", members)
	}

	conn, cerr := s.conns.Get(ctx, id)
	if cerr != nil {
		return conn, connectionError(cerr)
	}

	p, ok := s.providers.Provider(conn.Kind)
	if !ok || !slices.Contains(p.Descriptor().Capabilities, capability) || !slices.Contains(conn.Accepts, accept) {
		return conn, newError(
			command.CodeValidation,
			"the connection is not a connection to a provider with the capability %s that accepts %s.",
			capability,
			accept,
		).
			withPath("args.connection_id").
			withHint("list the providers with provider:list and connect one with connection:add")
	}

	return conn, nil
}

func (s *svc) prepareImageEdit(ctx context.Context, req JobStartRequest) (Prepared, error) {
	const members = "image_id, operation, model and, by operation, layer_id, prompt and size"

	args, aerr := decodeGenerateArgs[ImageModelArgs](req.Args, "connection_id, "+members)
	if aerr != nil {
		return Prepared{}, aerr
	}

	c, ok := CallerFrom(ctx)
	if !ok {
		return Prepared{}, newError(CodeUnauthorized, "the request carries no credentials.")
	}

	// An image belongs to no project, so a key limited to one project reaches none (SDD-021 section 10).
	if c.ProjectID != "" {
		return Prepared{}, newError(CodeForbidden, "the key is limited to one project and cannot use images.").
			withHint("use a key without a project limit")
	}

	if !validImageID(args.ImageID) {
		return Prepared{}, newError(command.CodeValidation, "image_id is not an image id.").
			withPath("args.image_id").withHint("an image id is img_ and 10 characters; image:list names them")
	}

	row, doc, lerr := s.loadImage(ctx, s.store.Images(), c, args.ImageID)
	if lerr != nil {
		return Prepared{}, lerr
	}

	in := imageEditInput{
		ImageID:   row.ID,
		Operation: args.Operation,
		LayerID:   args.LayerID,
		Revision:  row.Revision,
		Owner:     c.OwnerUserID,
	}

	var (
		prep Prepared
		err  error
	)

	switch args.Operation {
	case provider.OperationRemoveBackground:
		prep, err = s.prepareRemoveBackground(ctx, &args, doc, &in, members)
	case provider.OperationGenerate:
		prep, err = s.prepareGenerateInto(ctx, &args, doc, &in, members)
	default:
		return Prepared{}, newError(command.CodeValidation, "operation must be %s or %s.",
			provider.OperationRemoveBackground, provider.OperationGenerate).withPath("args.operation")
	}

	if err != nil {
		return Prepared{}, err
	}

	raw, merr := json.Marshal(in)
	if merr != nil {
		return Prepared{}, merr
	}

	prep.Input = raw

	return prep, nil
}

func (s *svc) prepareRemoveBackground(
	ctx context.Context, args *ImageModelArgs, doc *coreimage.Document, in *imageEditInput, members string,
) (Prepared, error) {
	if args.Prompt != "" || args.Size != "" {
		return Prepared{}, newError(command.CodeValidation, "remove_background takes no prompt and no size.").
			withPath("args.prompt").withHint("the prompt of a removal is the adapter's own")
	}

	conn, err := s.editConnection(
		ctx,
		args.ConnectionID,
		provider.CapabilityImageEdit,
		connections.AcceptImage,
		members,
	)
	if err != nil {
		return Prepared{}, err
	}

	ed, _ := s.providers.Editor(conn.Kind)

	if args.LayerID == "" {
		return Prepared{}, newError(command.CodeValidation, "layer_id is required for remove_background.").
			withPath("args.layer_id").withHint("image:show lists the layers of the image")
	}

	layer, rerr := coreimage.CheckReplaceable(doc, args.LayerID)
	if rerr != nil {
		return Prepared{}, fromCommand(rerr)
	}

	ref, ok := doc.Assets[layer.Asset]
	if layer.Asset == "" || !ok {
		return Prepared{}, newError(command.CodeValidation, "the layer %s has no picture to send yet.", layer.ID).
			withPath("args.layer_id").withHint("fill the slot with an image first")
	}

	ar, gerr := s.store.Assets().Get(ctx, layer.Asset)
	if gerr != nil || ar.Kind != "image" || ar.IngestState != IngestReady {
		return Prepared{}, newError(
			command.CodeValidation,
			"the picture of the layer %s is not a ready image.",
			layer.ID,
		).
			withPath("args.layer_id").
			withHint("wait for its ingest, or import it again")
	}

	limits := ed.EditLimits(args.Model)
	if ar.SizeBytes > limits.SourceLimit() || ar.SizeBytes > s.generationLimit(maxGeneratedImage) {
		return Prepared{}, newError(command.CodeValidation,
			"the picture of the layer %s is %d bytes; the provider takes at most %d.",
			layer.ID, ar.SizeBytes, min(limits.SourceLimit(), s.generationLimit(maxGeneratedImage))).
			withPath("args.layer_id").withHint("use a smaller picture").
			withLimit("source_bytes", ar.SizeBytes, limits.SourceLimit())
	}

	size := ""
	if len(limits.Sizes) > 0 {
		size = provider.ClosestSize(ref.Width, ref.Height, limits.Sizes)
	}

	r := provider.NormalizeEdit(provider.EditRequest{Model: args.Model, Operation: args.Operation, Size: size})
	if ferr := provider.ValidateEdit(r, limits); ferr != nil {
		return Prepared{}, newError(command.CodeValidation, "%s", ferr.Message).withPath("args." + ferr.Path)
	}

	price, source, perr := s.generationPrice(&conn, r.Model, provider.UnitImage)
	if perr != nil {
		return Prepared{}, perr
	}

	cost := provider.ImageCost(1, price)
	estimate := core.Money{Micros: cost.Micros, Currency: cost.Currency}

	in.ConnectionID, in.Provider, in.Edit = conn.ID, conn.Kind, &r
	in.Source = &imageEditSource{Hash: layer.Asset, Bytes: ar.SizeBytes}
	in.Price, in.PriceSource, in.Estimate = price, source, estimate

	name := ed.Descriptor().Name
	summary := fmt.Sprintf(
		"Remove the background of the layer %q of the image %q with %s, model %s. "+
			"The picture of the layer, %d by %d px, will be sent to %s.",
		excerpt(layer.Name, 40), excerpt(doc.Name, 40), name, r.Model, ref.Width, ref.Height, name)

	return Prepared{Summary: summary, ConnectionID: conn.ID, Estimate: estimate}, nil
}

func (s *svc) prepareGenerateInto(
	ctx context.Context, args *ImageModelArgs, doc *coreimage.Document, in *imageEditInput, members string,
) (Prepared, error) {
	conn, err := s.mediaConnection(ctx, args.ConnectionID, provider.CapabilityImage, members)
	if err != nil {
		return Prepared{}, err
	}

	gen, _ := s.providers.Image(conn.Kind)

	// The picture must cover the canvas, or the box of the layer the person named.
	width, height := doc.Width, doc.Height

	if args.LayerID != "" {
		l, ok := coreimage.Find(doc, args.LayerID)
		if !ok {
			return Prepared{}, newError(command.CodeNotFound, "Layer %s does not exist.", args.LayerID).
				withPath("args.layer_id").withHint("image:show lists the layers of the image")
		}

		if l.W < 1 || l.H < 1 {
			return Prepared{}, newError(command.CodeValidation, "the layer %s has no box to cover.", l.ID).
				withPath("args.layer_id").withHint("name a layer that is not a group, or none for the canvas")
		}

		width, height = l.W, l.H
	}

	limits := gen.ImageLimits(args.Model)

	size := args.Size
	if size == "" {
		size = provider.ClosestSize(width, height, limits.Sizes)
	}

	r := provider.NormalizeImage(provider.ImageRequest{Model: args.Model, Prompt: args.Prompt, Size: size, Count: 1})
	if ferr := provider.ValidateImage(r, limits); ferr != nil {
		return Prepared{}, newError(command.CodeValidation, "%s", ferr.Message).withPath("args." + ferr.Path)
	}

	price, source, perr := s.generationPrice(&conn, r.Model, provider.UnitImage)
	if perr != nil {
		return Prepared{}, perr
	}

	cost := provider.EstimateImage(r, price)
	estimate := core.Money{Micros: cost.Micros, Currency: cost.Currency}

	in.ConnectionID, in.Provider, in.Image = conn.ID, conn.Kind, &r
	in.Price, in.PriceSource, in.Estimate = price, source, estimate

	name := gen.Descriptor().Name
	head := fmt.Sprintf(
		"Generate an image of %s into the image %q with %s, model %s. Only the text below is sent to %s.",
		r.Size, excerpt(doc.Name, 40), name, r.Model, name)

	return Prepared{Summary: summaryWithExcerpt(head, r.Prompt), ConnectionID: conn.ID, Estimate: estimate}, nil
}

// readEditSource reads the file of the layer from the content store, to the bound the provider takes, and names
// its format by its signature. It is read only once the job is approved.
func (s *svc) readEditSource(src *imageEditSource, limit int64) (providers.SourceImage, *Error) {
	refuse := func(why string) (providers.SourceImage, *Error) {
		return providers.SourceImage{}, newError(command.CodeValidation, "the picture of the layer %s", why).
			withHint("use a PNG, a JPEG or a WebP picture of at most %d bytes", limit)
	}

	if s.media.Content == nil {
		return providers.SourceImage{}, internalError()
	}

	f, err := s.media.Content.Open(strings.TrimPrefix(src.Hash, "sha256:"))
	if err != nil {
		return refuse("is no longer in the workspace.")
	}

	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return refuse("could not be read.")
	}

	if int64(len(data)) > limit {
		return refuse("is larger than the provider takes.")
	}

	format, valid := provider.CheckImage(data)
	if !valid {
		return refuse("is not a PNG, a JPEG or a WebP file.")
	}

	return providers.SourceImage{Data: data, Format: format}, nil
}

// hasTransparency reports whether data is a PNG that has a pixel that is not opaque. The picture is untrusted:
// its size is read first and refused above maxEditPixels, and only then is it decoded.
func hasTransparency(data []byte) bool {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > maxEditPixels {
		return false
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}

	b := img.Bounds()

	if p, ok := img.(*image.NRGBA); ok {
		for y := range b.Dy() {
			row := p.Pix[y*p.Stride : y*p.Stride+b.Dx()*4]
			for x := 3; x < len(row); x += 4 {
				if row[x] != 0xFF {
					return true
				}
			}
		}

		return false
	}

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xFFFF {
				return true
			}
		}
	}

	return false
}

func (s *svc) readImageEditInput(run *JobRun) (imageEditInput, error) {
	var in imageEditInput
	if err := json.Unmarshal(run.Input, &in); err != nil {
		return in, newError(command.CodeInternal, "the input of the job is not readable.")
	}

	return in, nil
}

// runImageEdit is the job once approved: one request through the connection, the file checked and stored as an
// asset with the origin generated, one batch committed to the document as the actor that asked, and the real
// cost, the price the person saw times the images that arrived.
func (s *svc) runImageEdit(ctx context.Context, run JobRun) (JobOutcome, error) {
	in, err := s.readImageEditInput(&run)
	if err != nil {
		return JobOutcome{}, err
	}

	limit := s.generationLimit(maxGeneratedImage)

	if lerr := s.checkFreeSpace(limit); lerr != nil {
		return JobOutcome{}, lerr
	}

	res, err := s.askForTheImage(ctx, &run, &in, limit)
	if err != nil {
		return JobOutcome{}, err
	}

	// The provider billed what it delivered, whatever the product makes of the file.
	spent := moneyOf(provider.ImageCost(len(res.Files), in.Price))
	model, prompt, source := in.describe(run.ID)

	out := &GeneratedResult{
		Provider: in.Provider, Model: model, Requested: 1, Delivered: len(res.Files), Cost: spent,
		Items: []GeneratedItem{},
	}

	data := res.Files[0].Data

	format, valid := provider.CheckImage(data)
	if !valid {
		out.Notes = append(out.Notes, "the image is not a PNG, a JPEG or a WebP and was not kept")

		return s.finishGenerated(ctx, run.ProjectID, out, &spent)
	}

	item, serr := s.storeGenerated(
		ctx,
		&run,
		"image",
		fmt.Sprintf("image-%s-1.%s", shortJob(run.ID), provider.ImageExt(format)),
		data,
		&asset.Origin{
			Kind:     asset.Generated,
			Provider: cut(in.Provider, asset.MaxProvider),
			Model:    cut(model, asset.MaxModel),
			Prompt: cut(
				prompt,
				asset.MaxPrompt,
			),
			CostMicros: originCost(provider.Cost{Micros: in.Price.UnitMicros, Currency: in.Price.Currency}),
			Source:     cut(source, asset.MaxSource),
		},
	)
	if serr != nil {
		out.Notes = append(out.Notes, "the image was not stored: "+serr.Message)

		return s.finishGenerated(ctx, run.ProjectID, out, &spent)
	}

	item.RevisedPrompt = res.Files[0].RevisedPrompt
	out.Items = append(out.Items, item)
	s.awaitIngest(ctx, run.ProjectID, out)

	s.applyImageEdit(ctx, &run, &in, &out.Items[0], out, in.Edit != nil && format == "png" && hasTransparency(data))

	raw, merr := json.Marshal(JobResult{Generated: out})
	if merr != nil {
		return JobOutcome{Cost: &spent}, merr
	}

	return JobOutcome{Result: raw, Cost: &spent}, nil
}

// describe is what the origin of the file records of the job: the model, the prompt (the fixed words of a removal,
// or the prompt of a generation) and the source, which for a removal names the picture that was sent.
func (in *imageEditInput) describe(jobID string) (model, prompt, source string) {
	source = "job " + jobID

	if in.Edit != nil {
		return in.Edit.Model, removedBackgroundPrompt, source + " of " + in.Source.Hash
	}

	return in.Image.Model, in.Image.Prompt, source
}

// askForTheImage makes the one request of the job through the connection. Nothing has been read or sent before
// this: the picture is read from the store here, and a picture that cannot be sent ends the job with no request.
func (s *svc) askForTheImage(
	ctx context.Context,
	run *JobRun,
	in *imageEditInput,
	limit int64,
) (providers.ImageResult, error) {
	var res providers.ImageResult

	switch in.Operation {
	case provider.OperationRemoveBackground:
		ed, ok := s.providers.Editor(in.Provider)
		if !ok || in.Edit == nil || in.Source == nil {
			return res, providerGone(in.Provider)
		}

		src, rerr := s.readEditSource(in.Source, ed.EditLimits(in.Edit.Model).SourceLimit())
		if rerr != nil {
			return res, rerr
		}

		run.Report(ctx, 0.1, "sending the picture to the provider")

		err := s.conns.WithTimeout(ctx, in.ConnectionID, imageTimeout, func(c connections.Credential) error {
			var gerr error

			res, gerr = ed.RemoveBackground(ctx, c, *in.Edit, src, limit)
			res.Files = echoFree(c, res.Files)

			return gerr
		})
		if err != nil {
			return res, providerError(err)
		}
	case provider.OperationGenerate:
		gen, ok := s.providers.Image(in.Provider)
		if !ok || in.Image == nil {
			return res, providerGone(in.Provider)
		}

		run.Report(ctx, 0.1, "asking the provider")

		err := s.conns.WithTimeout(ctx, in.ConnectionID, imageTimeout, func(c connections.Credential) error {
			var gerr error

			res, gerr = gen.GenerateImages(ctx, c, *in.Image, limit)
			res.Files = echoFree(c, res.Files)

			return gerr
		})
		if err != nil {
			return res, providerError(err)
		}
	default:
		return res, newError(command.CodeInternal, "the operation of the job is not known.")
	}

	if len(res.Files) == 0 {
		return res, providerError(&providers.Error{Kind: providers.KindUnreadable})
	}

	// One image was asked for: the provider billed one, and no more is held.
	res.Files = res.Files[:1]

	return res, nil
}

// echoFree replaces the revised prompt of a file that repeats the key (T-55); the bytes are not text.
func echoFree(c connections.Credential, files []providers.GeneratedFile) []providers.GeneratedFile {
	for i := range files {
		if c.Echoes(files[i].RevisedPrompt) {
			files[i].RevisedPrompt = connections.EchoedAnswer
		}
	}

	return files
}

// applyImageEdit commits the batch that puts the file in the document, as the actor that asked and against the
// revision Prepare read. A batch the document refuses does not fail the job: the file is stored and paid for, so
// the job says what happened and leaves the asset in the library.
func (s *svc) applyImageEdit(
	ctx context.Context, run *JobRun, in *imageEditInput, item *GeneratedItem, out *GeneratedResult, transparent bool,
) {
	applied := &ImageApplied{ImageID: in.ImageID, LayerID: in.LayerID}
	out.Image = applied

	keep := func(why string) {
		out.Notes = append(out.Notes, why+"; the image is in the library as "+item.AssetID)
	}

	if item.State != string(IngestReady) {
		keep("the file did not become a ready image, so it was not put in the document")

		return
	}

	var ops []command.Operation

	switch in.Operation {
	case provider.OperationRemoveBackground:
		if !transparent {
			keep("the provider returned a picture with no transparent pixel, so the layer was not changed")

			return
		}

		ops = []command.Operation{
			operation("layer.replace_source", map[string]any{"layer_id": in.LayerID, "asset": item.Hash}),
		}
	default:
		id := s.newIDs(coreimage.PrefixLayer)
		applied.LayerID = id
		ops = []command.Operation{operation("layer.add_image", map[string]any{
			"asset": item.Hash, "id": id, "name": editedLayerName, "fit": "cover",
		})}

		if in.LayerID != "" {
			ops = append(ops, operation("layer.fit", map[string]any{"layer_id": id, "mode": "cover", "to": in.LayerID}))
		}
	}

	caller := Caller{Actor: run.Actor, Scopes: []string{ScopeEdit}, OwnerUserID: in.Owner}

	res, err := s.ImageEdit(WithCaller(ctx, caller), ImageEditRequest{
		ImageID: in.ImageID, BaseRevision: in.Revision, Operations: ops, IdempotencyKey: "imgedit-" + run.ID,
	})
	if err != nil {
		reason := "the document refused the change"

		if e, ok := errors.AsType[*Error](err); ok {
			reason += " (" + e.Code + ": " + e.Message + ")"
		}

		keep(reason)

		return
	}

	applied.Revision, applied.Applied = res.Revision, res.Applied
	if !res.Applied {
		keep("the change left the document as it was")
	}
}

func operation(name string, args map[string]any) command.Operation {
	raw, err := json.Marshal(args)
	if err != nil {
		panic(err) // a map of strings always encodes
	}

	return command.Operation{Op: name, Args: raw}
}
