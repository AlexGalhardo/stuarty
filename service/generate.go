package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	core "github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/asset"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
)

// The job kinds of image and voice generation (SDD-016 section 6.1).
const (
	JobImageGenerate = "image.generate"
	JobVoiceGenerate = "voice.generate"
)

// Limits of a generation (SDD-016 section 6.1).
const (
	// imageTimeout and voiceTimeout are how long one request may take.
	imageTimeout = 180 * time.Second
	voiceTimeout = 120 * time.Second
	// maxGeneratedImage and maxGeneratedAudio bound one file a provider returns; the import limit of the
	// workspace bounds them too.
	maxGeneratedImage = 32 << 20
	maxGeneratedAudio = 64 << 20
)

// ImageArgs are the arguments of an image.generate job: the connection, and the request of SDD-016 section 5.1.
type ImageArgs struct {
	ConnectionID string `json:"connection_id"`
	provider.ImageRequest
}

// VoiceArgs are the arguments of a voice.generate job: the connection, and the request of SDD-016 section 5.2.
type VoiceArgs struct {
	ConnectionID string `json:"connection_id"`
	provider.VoiceRequest
}

// GeneratedItem is one file a generation stored as an asset.
type GeneratedItem struct {
	// AssetID is the asset of the project; Hash is the content hash the library names it by.
	AssetID string `json:"asset_id"`
	Hash    string `json:"hash"`
	Kind    string `json:"kind"`
	Bytes   int64  `json:"bytes"`
	// State is the ingest state when the job ended: the ingest runs after the job.
	State string `json:"state"`
	// RevisedPrompt is the prompt as the provider rewrote it, when it does: untrusted text.
	RevisedPrompt string `json:"revised_prompt,omitempty" untrusted:"true"`
}

// GeneratedResult is the result of a succeeded image.generate or voice.generate job (SDD-016 section 6.1).
type GeneratedResult struct {
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Items     []GeneratedItem `json:"items"`
	Requested int             `json:"requested"`
	Delivered int             `json:"delivered"`
	// Notes say, in words the product wrote, what did not arrive.
	Notes []string   `json:"notes,omitempty"`
	Cost  core.Money `json:"cost"`
	// Image is what an image.edit job did to the document (SDD-016 section 6.4); other kinds leave it out.
	Image *ImageApplied `json:"image,omitempty"`
}

// generateInput is the input column of an image.generate or voice.generate job: what was asked, and the price
// the person saw, so what is spent is judged with it (SDD-016 section 6.1).
type generateInput struct {
	ConnectionID string                 `json:"connection_id"`
	Provider     string                 `json:"provider"`
	Image        *provider.ImageRequest `json:"image,omitempty"`
	Voice        *provider.VoiceRequest `json:"voice,omitempty"`
	Price        provider.Price         `json:"price"`
	PriceSource  string                 `json:"price_source"`
	Estimate     core.Money             `json:"estimate"`
}

func (s *svc) imageGenerateKind() ApprovableKind {
	return ApprovableKind{Name: JobImageGenerate, Scope: ScopeEdit, Prepare: s.prepareImage, Run: s.runImage}
}

func (s *svc) voiceGenerateKind() ApprovableKind {
	return ApprovableKind{Name: JobVoiceGenerate, Scope: ScopeEdit, Prepare: s.prepareVoice, Run: s.runVoice}
}

func decodeGenerateArgs[T any](raw json.RawMessage, members string) (T, *Error) {
	var a T

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if len(bytes.TrimSpace(raw)) == 0 || dec.Decode(&a) != nil || dec.More() {
		return a, newError(command.CodeValidation, "args must be one JSON object with %s.", members).withPath("args")
	}

	return a, nil
}

// mediaConnection finds the connection and checks that its provider has the capability and that the connection
// accepts text, which is all a prompt is (SDD-014 section 2).
func (s *svc) mediaConnection(ctx context.Context, id, capability, members string) (connections.Connection, error) {
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
	if !ok || !slices.Contains(p.Descriptor().Capabilities, capability) ||
		!slices.Contains(conn.Accepts, connections.AcceptText) {
		return conn, newError(command.CodeValidation,
			"the connection is not a connection to a provider of %s that accepts text.", capability).
			withPath("args.connection_id").
			withHint("list the providers with provider:list and connect one with connection:add")
	}

	return conn, nil
}

// generationPrice finds the price of a model in the unit of the capability; a price in another unit is no price.
func (s *svc) generationPrice(conn *connections.Connection, model, unit string) (provider.Price, string, error) {
	price, source, perr := s.providers.PriceOf(conn.Kind, conn.ID, model)
	if perr != nil {
		return price, source, providerError(perr)
	}

	if source == provider.SourceNone || price.Unit != unit {
		what := map[string]string{provider.UnitImage: "image", provider.UnitMChar: "million characters"}[unit]

		return price, source, newError(
			CodePriceUnknown,
			"no price per %s is known for the model %s of %s.",
			what,
			model,
			conn.Kind,
		).
			withDetail("provider", conn.Kind).
			withDetail("model", model).
			withDetail("unit", unit).
			withHint("ask a person to set the price with provider:price (unit %s) or on the Connections page; "+
				"a price of tokens does not apply", unit)
	}

	return price, source, nil
}

func (s *svc) prepareImage(ctx context.Context, req JobStartRequest) (Prepared, error) {
	const members = "model, prompt and optionally size and count"

	args, aerr := decodeGenerateArgs[ImageArgs](req.Args, "connection_id, "+members)
	if aerr != nil {
		return Prepared{}, aerr
	}

	conn, err := s.mediaConnection(ctx, args.ConnectionID, provider.CapabilityImage, members)
	if err != nil {
		return Prepared{}, err
	}

	gen, _ := s.providers.Image(conn.Kind)
	r := provider.NormalizeImage(args.ImageRequest)

	if ferr := provider.ValidateImage(r, gen.ImageLimits(r.Model)); ferr != nil {
		return Prepared{}, newError(command.CodeValidation, "%s", ferr.Message).withPath("args." + ferr.Path)
	}

	price, source, perr := s.generationPrice(&conn, r.Model, provider.UnitImage)
	if perr != nil {
		return Prepared{}, perr
	}

	cost := provider.EstimateImage(r, price)
	estimate := core.Money{Micros: cost.Micros, Currency: cost.Currency}

	input, merr := json.Marshal(generateInput{
		ConnectionID: conn.ID, Provider: conn.Kind, Image: &r, Price: price, PriceSource: source, Estimate: estimate,
	})
	if merr != nil {
		return Prepared{}, merr
	}

	head := fmt.Sprintf("Generate %d image(s) of %s with %s, model %s.",
		r.Count, r.Size, gen.Descriptor().Name, r.Model)

	return Prepared{
		Summary: summaryWithExcerpt(head, r.Prompt), ConnectionID: conn.ID, Estimate: estimate, Input: input,
	}, nil
}

func (s *svc) prepareVoice(ctx context.Context, req JobStartRequest) (Prepared, error) {
	const members = "model, text, voice and optionally language and format"

	args, aerr := decodeGenerateArgs[VoiceArgs](req.Args, "connection_id, "+members)
	if aerr != nil {
		return Prepared{}, aerr
	}

	conn, err := s.mediaConnection(ctx, args.ConnectionID, provider.CapabilityVoice, members)
	if err != nil {
		return Prepared{}, err
	}

	gen, _ := s.providers.Voice(conn.Kind)
	r := provider.NormalizeVoice(args.VoiceRequest)

	if ferr := provider.ValidateVoice(r, gen.VoiceLimits(r.Model)); ferr != nil {
		return Prepared{}, newError(command.CodeValidation, "%s", ferr.Message).withPath("args." + ferr.Path)
	}

	price, source, perr := s.generationPrice(&conn, r.Model, provider.UnitMChar)
	if perr != nil {
		return Prepared{}, perr
	}

	cost := provider.EstimateVoice(r, price)
	estimate := core.Money{Micros: cost.Micros, Currency: cost.Currency}

	input, merr := json.Marshal(generateInput{
		ConnectionID: conn.ID, Provider: conn.Kind, Voice: &r, Price: price, PriceSource: source, Estimate: estimate,
	})
	if merr != nil {
		return Prepared{}, merr
	}

	head := fmt.Sprintf("Generate speech with %s, model %s, voice %s, %s: a text of %d characters.",
		gen.Descriptor().Name, r.Model, r.Voice, r.Format, utf8.RuneCountInString(r.Text))

	return Prepared{
		Summary: summaryWithExcerpt(head, r.Text), ConnectionID: conn.ID, Estimate: estimate, Input: input,
	}, nil
}

// summaryWithExcerpt is what a person reads before approving (SDD-016 section 6.1): a sentence the product wrote
// and an excerpt of the prompt or the text cleaned as textSummary cleans it, within the 500 characters of
// SDD-015 section 3.
func summaryWithExcerpt(head, text string) string {
	room := min(maxExcerpt, core.MaxSummary-utf8.RuneCountInString(head)-len(" Starts with: “”…"))
	if room < 10 {
		return head
	}

	return head + " Starts with: “" + excerpt(text, room) + "”…"
}

// generationLimit is the largest file of a generation: the bound of the kind and the import limit of the workspace.
func (s *svc) generationLimit(kindMax int64) int64 {
	if s.limits.MaxImportBytes > 0 {
		return min(kindMax, s.limits.MaxImportBytes)
	}

	return kindMax
}

func (s *svc) readGenerateInput(run *JobRun) (generateInput, error) {
	var in generateInput
	if err := json.Unmarshal(run.Input, &in); err != nil {
		return in, newError(command.CodeInternal, "the input of the job is not readable.")
	}

	return in, nil
}

// runImage is the job once approved: one request through the connection, the files checked and stored as assets
// with the origin generated, and the real cost, the price the person saw times the images that arrived.
func (s *svc) runImage(ctx context.Context, run JobRun) (JobOutcome, error) {
	in, err := s.readGenerateInput(&run)
	if err != nil {
		return JobOutcome{}, err
	}

	gen, ok := s.providers.Image(in.Provider)
	if !ok || in.Image == nil {
		return JobOutcome{}, providerGone(in.Provider)
	}

	req, limit := *in.Image, s.generationLimit(maxGeneratedImage)

	if lerr := s.checkFreeSpace(limit * int64(req.Count)); lerr != nil {
		return JobOutcome{}, lerr
	}

	run.Report(ctx, 0.1, "asking the provider")

	var res providers.ImageResult

	err = s.conns.WithTimeout(ctx, in.ConnectionID, imageTimeout, func(c connections.Credential) error {
		var gerr error

		res, gerr = gen.GenerateImages(ctx, c, req, limit)
		for i := range res.Files {
			// A server that speaks the protocol may repeat the key it was sent (T-55).
			if c.Echoes(res.Files[i].RevisedPrompt) {
				res.Files[i].RevisedPrompt = connections.EchoedAnswer
			}
		}

		return gerr
	})
	if err != nil {
		return JobOutcome{}, providerError(err)
	}

	// The provider billed what it delivered, whatever the product makes of the files.
	spent := moneyOf(provider.ImageCost(len(res.Files), in.Price))
	out := &GeneratedResult{
		Provider: in.Provider, Model: req.Model, Requested: req.Count, Delivered: len(res.Files), Cost: spent,
		Items: []GeneratedItem{},
	}

	if res.Failed > 0 {
		out.Notes = append(
			out.Notes,
			fmt.Sprintf("%d of %d images did not arrive: %s", res.Failed, req.Count, failureNote(res.Err)),
		)
	}

	each := provider.Cost{Micros: in.Price.UnitMicros, Currency: in.Price.Currency}

	for i := range res.Files {
		f := &res.Files[i]

		format, valid := provider.CheckImage(f.Data)
		if !valid {
			out.Notes = append(out.Notes, fmt.Sprintf("image %d is not a PNG, a JPEG or a WebP and was not kept", i+1))

			continue
		}

		name := fmt.Sprintf("image-%s-%d.%s", shortJob(run.ID), i+1, provider.ImageExt(format))

		item, serr := s.storeGenerated(ctx, &run, "image", name, f.Data, &asset.Origin{
			Kind: asset.Generated, Provider: cut(in.Provider, asset.MaxProvider), Model: cut(req.Model, asset.MaxModel),
			Prompt: cut(req.Prompt, asset.MaxPrompt), CostMicros: originCost(each), Source: "job " + run.ID,
		})
		if serr != nil {
			out.Notes = append(out.Notes, fmt.Sprintf("image %d was not stored: %s", i+1, serr.Message))

			continue
		}

		item.RevisedPrompt = f.RevisedPrompt
		out.Items = append(out.Items, item)
	}

	return s.finishGenerated(ctx, run.ProjectID, out, &spent)
}

// runVoice is runImage for one speech.
func (s *svc) runVoice(ctx context.Context, run JobRun) (JobOutcome, error) {
	in, err := s.readGenerateInput(&run)
	if err != nil {
		return JobOutcome{}, err
	}

	gen, ok := s.providers.Voice(in.Provider)
	if !ok || in.Voice == nil {
		return JobOutcome{}, providerGone(in.Provider)
	}

	req, limit := *in.Voice, s.generationLimit(maxGeneratedAudio)

	if lerr := s.checkFreeSpace(limit); lerr != nil {
		return JobOutcome{}, lerr
	}

	run.Report(ctx, 0.1, "asking the provider")

	var res providers.VoiceResult

	err = s.conns.WithTimeout(ctx, in.ConnectionID, voiceTimeout, func(c connections.Credential) error {
		var gerr error

		res, gerr = gen.GenerateVoice(ctx, c, req, limit)

		return gerr
	})
	if err != nil {
		return JobOutcome{}, providerError(err)
	}

	spent := moneyOf(provider.EstimateVoice(req, in.Price))
	out := &GeneratedResult{
		Provider: in.Provider, Model: req.Model, Requested: 1, Delivered: 1, Cost: spent, Items: []GeneratedItem{},
	}

	if !provider.CheckAudio(res.File.Data, req.Format) {
		out.Notes = append(out.Notes, fmt.Sprintf("the audio is not a %s file and was not kept", req.Format))

		return s.finishGenerated(ctx, run.ProjectID, out, &spent)
	}

	name := fmt.Sprintf("voice-%s-1.%s", shortJob(run.ID), provider.AudioExt(req.Format))

	item, serr := s.storeGenerated(ctx, &run, "audio", name, res.File.Data, &asset.Origin{
		Kind: asset.Generated, Provider: cut(in.Provider, asset.MaxProvider), Model: cut(req.Model, asset.MaxModel),
		Prompt: cut(req.Text, asset.MaxPrompt), CostMicros: originCost(spentCost(&spent)), Source: "job " + run.ID,
	})
	if serr != nil {
		out.Notes = append(out.Notes, "the audio was not stored: "+serr.Message)

		return s.finishGenerated(ctx, run.ProjectID, out, &spent)
	}

	out.Items = append(out.Items, item)

	return s.finishGenerated(ctx, run.ProjectID, out, &spent)
}

// finishGenerated makes the outcome of a generation. A batch that stored nothing fails, with the cost of what the
// provider delivered still recorded. Otherwise it waits, up to ingestWait, for the ingest of the files, so that the
// job ends with assets a project can use (the ingest runs on its own queue and does not need this worker).
func (s *svc) finishGenerated(
	ctx context.Context, projectID string, out *GeneratedResult, spent *core.Money,
) (JobOutcome, error) {
	if len(out.Items) == 0 {
		return JobOutcome{Cost: spent}, newError(CodeProvider, "the provider answered, but no file could be kept.").
			withDetail("kind", providers.KindUnreadable).
			withDetail("notes", out.Notes).
			withHint("%s", providerHint(&providers.Error{Kind: providers.KindUnreadable}))
	}

	s.awaitIngest(ctx, projectID, out)

	raw, merr := json.Marshal(JobResult{Generated: out})
	if merr != nil {
		return JobOutcome{Cost: spent}, merr
	}

	return JobOutcome{Result: raw, Cost: spent}, nil
}

// storeGenerated puts one checked file in the media store and links it to the project as an asset whose origin is
// the generation (SDD-016 section 6.1). The file is content-addressed, so the same bytes are one file.
func (s *svc) storeGenerated(
	ctx context.Context, run *JobRun, kind, name string, data []byte, origin *asset.Origin,
) (GeneratedItem, *Error) {
	if s.media.Content == nil {
		return GeneratedItem{}, internalError()
	}

	row, err := s.store.Projects().Get(ctx, run.ProjectID)
	if err != nil {
		return GeneratedItem{}, asErr(projectNotFound(run.ProjectID, err))
	}

	if row.State == ProjectTrashed {
		return GeneratedItem{}, newError(command.CodeInvalidState, "the project %s is in the trash.", row.ID).
			withHint("%s", hintRestoreProject)
	}

	o := origin.Normalize()
	if bad := o.Validate(); bad != nil {
		return GeneratedItem{}, asErr(ruleError(bad))
	}

	// The file belongs to the library of the owner of the project, as an import by that owner would.
	owner := ""
	if row.OwnerUserID != nil {
		owner = *row.OwnerUserID
	}

	got := s.putAndAttach(ctx, run.Actor, owner, &row, bytes.NewReader(data), "",
		ImportedAsset{Name: displayName(name), State: string(IngestPending)}, &libraryRecord{origin: o})
	if got.Error != nil {
		return GeneratedItem{}, got.Error
	}

	// The ingest may have run already (the queue runs it after the commit), or the bytes may be a file the
	// workspace had: the state and the kind are read back, and the kind we checked stands until the probe says.
	item := GeneratedItem{AssetID: got.AssetID, Hash: got.Hash, Kind: kind, Bytes: int64(len(data)), State: got.State}
	if ar, gerr := s.store.Assets().Get(ctx, got.Hash); gerr == nil {
		item.State = string(ar.IngestState)

		if ar.Kind != "" {
			item.Kind = ar.Kind
		}
	}

	return item, nil
}

// viewGeneration puts on the job the estimate of a generation (it is there from the moment the job exists) and,
// once it ended, its result: the text of text.generate and text.draft (SDD-019 section 6) or the files of
// image.generate and voice.generate.
func viewGeneration(j *Job, r *JobRow) {
	switch r.Kind {
	case JobTextGenerate, JobTextDraft, JobImageGenerate, JobVoiceGenerate, JobImageEdit:
	default:
		return
	}

	// The input of the four kinds names its estimate the same way.
	var in struct {
		Estimate core.Money `json:"estimate"`
	}

	if json.Unmarshal(r.Input, &in) == nil {
		estimate := in.Estimate
		j.Estimate = &estimate
	}

	var res JobResult
	if len(r.Result) > 0 && json.Unmarshal(r.Result, &res) == nil && (res.Text != nil || res.Generated != nil) {
		j.Result = &res
	}
}

// ingestWait is how long a generation job waits for the ingest of its files: an image or a narration probes in a
// moment, and a file that takes longer is reported as it is, still ingesting.
const ingestWait = 60 * time.Second

// awaitIngest waits for the ingest of the files of a generation and writes the state the files reached into the
// items; a file the ingest refused is said in a note, since it is stored and paid for.
func (s *svc) awaitIngest(ctx context.Context, projectID string, out *GeneratedResult) {
	row, err := s.store.Projects().Get(ctx, projectID)
	if err != nil {
		return
	}

	files := make([]ImportedAsset, len(out.Items))
	for i := range out.Items {
		files[i] = ImportedAsset{AssetID: out.Items[i].AssetID}
	}

	s.waitAssets(ctx, &row, files, ingestWait)

	for i := range out.Items {
		it := &out.Items[i]

		ar, gerr := s.store.Assets().Get(ctx, it.Hash)
		if gerr != nil {
			continue
		}

		it.State = string(ar.IngestState)

		if ar.Kind != "" {
			it.Kind = ar.Kind
		}

		if ar.IngestState == IngestFailed {
			out.Notes = append(
				out.Notes,
				it.AssetID+" was stored and its ingest failed: it cannot be used in a project",
			)
		}
	}
}

func providerGone(id string) error {
	return newError(command.CodeValidation, "the provider %s is no longer available.", id).
		withHint("connect a provider with connection:add")
}

func moneyOf(c provider.Cost) core.Money { return core.Money{Micros: c.Micros, Currency: c.Currency} }

func spentCost(m *core.Money) provider.Cost {
	return provider.Cost{Micros: m.Micros, Currency: m.Currency}
}

// originCost is the cost the origin of an asset records: the library keeps millionths of a US dollar, so a cost
// in another currency stays in the ledger and is left out of the origin.
func originCost(c provider.Cost) *int64 {
	if c.Currency != "USD" {
		return nil
	}

	m := c.Micros

	return &m
}

// shortJob is the last six characters of a job id, enough to tell the files of two jobs apart in a grid.
func shortJob(id string) string {
	if len(id) > 6 {
		return id[len(id)-6:]
	}

	return id
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}

	return string([]rune(s)[:n])
}

// failureNote is a sentence the product wrote about why a file did not arrive; the text of an error of a provider
// is never in it.
func failureNote(err error) string {
	if pe, ok := providers.AsError(err); ok {
		return strings.TrimSuffix(providerSentence(pe), ".")
	}

	if errors.Is(err, outbound.ErrDestination) || errors.Is(err, outbound.ErrNotPublic) {
		return "a file was offered at a host the connection does not list"
	}

	return "a file could not be fetched"
}
