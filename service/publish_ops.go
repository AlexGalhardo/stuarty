package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/approval"
	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/untrusted"
)

// JobPublishUpload is the job kind of SDD-020 section 10: one post, one connection, after approval.
const JobPublishUpload = "publish.upload"

// PublishArgs is the post of a publication: what the person or the agent decides (SDD-020 section 10).
type PublishArgs struct {
	// ConnectionID is the connection of the network and the account the video goes to.
	ConnectionID string `json:"connection_id"`
	// Export is the id of a succeeded export job of the project: the video.
	Export string `json:"export"`
	// Thumbnail is the id of a succeeded export job of a still of the project; optional.
	Thumbnail   string   `json:"thumbnail,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	// Visibility is private (the default), unlisted or public.
	Visibility string `json:"visibility,omitempty"`
	// MadeForKids must be set: the person declares it.
	MadeForKids  *bool `json:"made_for_kids,omitempty"`
	AIDisclosure bool  `json:"ai_disclosure,omitempty"`
}

// PublishNowRequest starts the publication of one post.
type PublishNowRequest struct {
	ProjectID string `json:"project_id"`
	// TimelineID anchors the job on one timeline, as every job does; needed when the project has several.
	TimelineID string `json:"timeline_id,omitempty"`
	PublishArgs
	WaitMS int  `json:"wait_ms,omitempty"`
	DryRun bool `json:"dry_run,omitempty"`
}

// PublishValidateRequest checks a post against the limits of its network.
type PublishValidateRequest struct {
	ProjectID string `json:"project_id"`
	PublishArgs
}

// PublishValidateResult says whether a post may be submitted.
type PublishValidateResult struct {
	Network string `json:"network"`
	// State is prepared when no issue is left, and invalid otherwise.
	State  string          `json:"state"`
	Issues []publish.Issue `json:"issues"`
	// Summary is what a person would read before approving.
	Summary string `json:"summary,omitempty" untrusted:"true"`
}

// PublishStatusRequest names the job of a publication.
type PublishStatusRequest struct {
	JobID string `json:"job_id"`
}

// PublishResult is what a publication produced; it is the result of a succeeded job.
type PublishResult struct {
	Network      string `json:"network"`
	ConnectionID string `json:"connection_id"`
	RemoteID     string `json:"remote_id"     untrusted:"true"`
	URL          string `json:"url"           untrusted:"true"`
	// State is processing or published.
	State string `json:"state"`
	// ThumbnailError is the kind of the error of the thumbnail when the video went up and the thumbnail did
	// not (quota, auth...).
	ThumbnailError string `json:"thumbnail_error,omitempty"`
}

// PublishStatusResult is the state of a publication (SDD-020 section 4).
type PublishStatusResult struct {
	JobID string `json:"job_id"`
	// State is waiting_approval, uploading, processing, published or failed.
	State  string         `json:"state"`
	Result *PublishResult `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// publishFile is a file the post sends, as it was when the person was asked: the run checks it again.
type publishFile struct {
	Rel    string `json:"rel"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256,omitempty"`
	Format string `json:"format"`
}

// publishInput is the input column of a publish.upload job.
type publishInput struct {
	Network   string       `json:"network"`
	Args      PublishArgs  `json:"args"`
	Output    publishFile  `json:"output"`
	Thumbnail *publishFile `json:"thumbnail,omitempty"`
}

func (in *publishInput) post() publish.Post {
	p := publish.Post{
		Output: publish.File{Bytes: in.Output.Bytes, Format: in.Output.Format}, Title: in.Args.Title,
		Description: in.Args.Description, Tags: in.Args.Tags, Visibility: publish.Visibility(in.Args.Visibility),
		MadeForKids: in.Args.MadeForKids, AIDisclosure: in.Args.AIDisclosure,
	}

	if in.Thumbnail != nil {
		p.Thumbnail = &publish.File{Bytes: in.Thumbnail.Bytes, Format: in.Thumbnail.Format}
	}

	return p
}

// registerPublishKind registers publish.upload as a job kind that needs approval (SDD-015 section 5).
func (s *svc) registerPublishKind() {
	if s.social == nil {
		return
	}

	s.kinds[JobPublishUpload] = &ApprovableKind{
		Name: JobPublishUpload, Scope: ScopeRender, Prepare: s.preparePublish, Run: s.runPublish,
	}
}

func decodePublishArgs(raw json.RawMessage) (PublishArgs, error) {
	var a PublishArgs

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&a); err != nil {
		return a, validationAt("args", "the arguments of a publication are not valid: %s.", jsonErrorText(err))
	}

	return a, nil
}

func jsonErrorText(err error) string {
	if te, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return "the member " + te.Field + " has the wrong type"
	}

	if strings.HasPrefix(err.Error(), "json: unknown field") {
		return strings.TrimPrefix(err.Error(), "json: ")
	}

	return "they are not a JSON object"
}

// publishTarget is what Prepare and PublishValidate learn about a post before anything is sent.
type publishTarget struct {
	conn     social.Connector
	desc     social.Description
	connRow  connections.Connection
	account  social.Account
	signedIn bool
	input    publishInput
	issues   []publish.Issue
}

// resolvePublish reads the connection and the files of a post and validates it. It makes no request.
func (s *svc) resolvePublish(ctx context.Context, c Caller, projectID string, a PublishArgs) (*publishTarget, error) {
	reg, err := s.socialRegistry()
	if err != nil {
		return nil, err
	}

	if a.ConnectionID == "" {
		return nil, validationAt("connection_id", "name the connection the video goes to.").
			withHint("list them with social:account:list")
	}

	row, gerr := s.conns.Get(ctx, a.ConnectionID)
	if gerr != nil {
		return nil, connectionError(gerr)
	}

	conn, cerr := reg.Get(row.Kind)
	if cerr != nil {
		return nil, validationAt("connection_id", "the connection is not one of a social network.")
	}

	t := &publishTarget{conn: conn, desc: conn.Describe(), connRow: row}
	t.input = publishInput{Network: t.desc.Network, Args: a}

	if werr := s.conns.With(ctx, row.ID, func(cred connections.Credential) error {
		t.account, t.signedIn = conn.SignedIn(cred.Secret)

		return nil
	}); werr != nil {
		return nil, connectionError(werr)
	}

	if !t.signedIn {
		return nil, validationAt("connection_id", "the connection is not signed in.").
			withHint("sign in again from the Connections page, then publish")
	}

	out, ferr := s.publishFile(ctx, c, projectID, "export", a.Export)
	if ferr != nil {
		return nil, ferr
	}

	t.input.Output = *out

	if a.Thumbnail != "" {
		th, ferr := s.publishFile(ctx, c, projectID, "thumbnail", a.Thumbnail)
		if ferr != nil {
			return nil, ferr
		}

		t.input.Thumbnail = th
	}

	t.issues = conn.Validate(t.input.post())

	return t, nil
}

// inProject says whether the id a job carries is the project or one of its timelines: the export of a
// timeline is kept under the id of the timeline (SDD-005 section 5.12), and it belongs to its project.
func (s *svc) inProject(ctx context.Context, carried, projectID string) bool {
	return carried == projectID || s.projectOf(ctx, carried) == s.projectOf(ctx, projectID)
}

// projectOf is the project an id belongs to: the id of a timeline names its project, any other id is its own.
func (s *svc) projectOf(ctx context.Context, id string) string {
	if owner, err := s.store.Productions().ProjectOfTimeline(ctx, id); err == nil {
		return owner
	}

	return id
}

// publishFile resolves the file of an export job of the project, from the database and never from the
// request, and checks it is the file the job recorded.
func (s *svc) publishFile(ctx context.Context, c Caller, projectID, path, jobID string) (*publishFile, error) {
	if jobID == "" {
		return nil, validationAt(
			path,
			"name the export job of the file.",
		).withHint("export first, then publish its job id")
	}

	row, err := s.store.Jobs().Get(ctx, jobID)
	if err != nil || !s.canSee(ctx, c, &row) || row.ProjectID == nil || !s.inProject(ctx, *row.ProjectID, projectID) {
		return nil, validationAt(path, "that export is not a job of this project.")
	}

	if row.Kind != JobExport || row.State != JobSucceeded {
		return nil, validationAt(path, "that job is not a succeeded export.").withHint("wait for the export to succeed")
	}

	f, ferr := s.ExportFile(ctx, ExportFileRequest{JobID: jobID})
	if ferr != nil {
		return nil, ferr
	}

	full := filepath.Join(s.workspace, filepath.FromSlash(f.Path))
	root := filepath.Join(s.workspace, "exports")

	if rel, rerr := filepath.Rel(root, full); rerr != nil || strings.HasPrefix(rel, "..") {
		return nil, internalError()
	}

	st, serr := os.Stat(full)
	if serr != nil || !st.Mode().IsRegular() {
		return nil, validationAt(path, "the file of that export is gone.").withHint("export it again")
	}

	var res JobResult

	_ = json.Unmarshal(row.Result, &res)

	out := &publishFile{
		Rel:    f.Path,
		Bytes:  st.Size(),
		Format: strings.ToLower(strings.TrimPrefix(filepath.Ext(f.Path), ".")),
	}
	if res.Export != nil {
		out.SHA256 = res.Export.SHA256

		if res.Export.Bytes > 0 && res.Export.Bytes != st.Size() {
			return nil, validationAt(
				path,
				"the file of that export changed after it was made.",
			).withHint("export it again")
		}
	}

	return out, nil
}

func issuesError(issues []publish.Issue) *Error {
	first := issues[0]
	e := newError(command.CodeValidation, "%s", first.Message).withPath(first.Path).withDetail("issues", issues)

	return e.withHint("fix the fields named in details.issues and publish again")
}

// publishSummary is the line a person reads before approving: where, as whom, what, who may watch, and
// which file. Text that came from outside is cleaned first, since the summary must be showable
// (SDD-015 section 3).
func publishSummary(t *publishTarget) string {
	clean := func(s string, n int) string {
		kept, _ := untrusted.Cut(untrusted.Clean(strings.ReplaceAll(s, "\n", " ")), n)

		return kept
	}

	a := t.input.Args
	vis := publish.Visibility(a.Visibility)

	if vis == "" {
		vis = publish.Private
	}

	return fmt.Sprintf("Publish %q to %s, channel %q (%s), visible to: %s; file %s, %d bytes",
		clean(a.Title, 100), t.desc.Name, clean(t.account.Title, 80), t.connRow.ID, vis,
		filepath.Base(t.input.Output.Rel), t.input.Output.Bytes)
}

// preparePublish is the Prepare of publish.upload: it checks and describes, and sends nothing.
func (s *svc) preparePublish(ctx context.Context, req JobStartRequest) (Prepared, error) {
	c, ok := CallerFrom(ctx)
	if !ok {
		return Prepared{}, newError(CodeUnauthorized, "no caller.")
	}

	a, aerr := decodePublishArgs(req.Args)
	if aerr != nil {
		return Prepared{}, aerr
	}

	t, err := s.resolvePublish(ctx, c, req.ProjectID, a)
	if err != nil {
		return Prepared{}, err
	}

	if len(t.issues) > 0 {
		return Prepared{}, issuesError(t.issues)
	}

	raw, merr := json.Marshal(t.input)
	if merr != nil {
		return Prepared{}, merr
	}

	return Prepared{
		Summary: publishSummary(t), ConnectionID: t.connRow.ID, Estimate: approval.Money{}, Input: raw,
	}, nil
}

// runPublish is the Run of publish.upload, after a person (or a rule the person wrote) approved. It
// checks the files are the ones that were shown, and reaches the network only through the connection.
func (s *svc) runPublish(ctx context.Context, run JobRun) (JobOutcome, error) {
	var in publishInput
	if err := json.Unmarshal(run.Input, &in); err != nil {
		return JobOutcome{}, internalError()
	}

	reg, err := s.socialRegistry()
	if err != nil {
		return JobOutcome{}, err
	}

	conn, cerr := reg.Get(in.Network)
	if cerr != nil {
		return JobOutcome{}, newError(command.CodeNotFound, "the network is not built into this server.")
	}

	full, ferr := s.approvedFile(in.Output)
	if ferr != nil {
		return JobOutcome{}, ferr
	}

	up := social.Upload{
		Post:     in.post(),
		Path:     full,
		Key:      uploadKey(&in),
		Sessions: social.FileSessions{Dir: filepath.Join(s.workspace, "publish", "uploads")},
		Report:   func(p float64, m string) { run.Report(ctx, p, m) },
	}

	if in.Thumbnail != nil {
		th, terr := s.approvedFile(*in.Thumbnail)
		if terr != nil {
			return JobOutcome{}, terr
		}

		up.ThumbnailPath = th
	}

	var pub social.Published

	werr := s.conns.With(ctx, in.Args.ConnectionID, func(cred connections.Credential) error {
		var perr error

		pub, perr = conn.Publish(ctx, cred, up)

		return perr
	})
	if werr != nil {
		return JobOutcome{}, s.socialError(werr)
	}

	raw, merr := json.Marshal(JobResult{Publish: &PublishResult{
		Network: in.Network, ConnectionID: in.Args.ConnectionID, RemoteID: pub.RemoteID, URL: pub.URL,
		State: string(pub.State), ThumbnailError: pub.ThumbnailError,
	}})
	if merr != nil {
		return JobOutcome{}, merr
	}

	return JobOutcome{Result: raw}, nil
}

// approvedFile is the path of a file the person was shown, checked once more: a file that changed since
// the approval is not sent.
func (s *svc) approvedFile(f publishFile) (string, error) {
	full := filepath.Join(s.workspace, filepath.FromSlash(f.Rel))
	root := filepath.Join(s.workspace, "exports")

	if rel, err := filepath.Rel(root, full); err != nil || strings.HasPrefix(rel, "..") {
		return "", internalError()
	}

	changed := newError(command.CodeInvalidState, "the file changed or is gone since the publication was approved.").
		withHint("export it again and publish again")

	st, err := os.Stat(full)
	if err != nil || !st.Mode().IsRegular() || st.Size() != f.Bytes {
		return "", changed
	}

	// The size alone misses a file rewritten with as many bytes; the export recorded its digest.
	if f.SHA256 != "" {
		if sum, herr := fileSHA256(full); herr != nil || sum != f.SHA256 {
			return "", changed
		}
	}

	return full, nil
}

// uploadKey names the upload session of a post (SDD-020 section 7.4): the connection, the file and every
// member of the post the network receives when the session starts. A post that differs in any of them,
// made_for_kids or ai_disclosure included, starts a new session instead of resuming one made with the
// former metadata. The thumbnail is sent after the upload, so it is not part of the key.
func uploadKey(in *publishInput) string {
	post := in.Args
	post.Thumbnail = ""

	raw, _ := json.Marshal(post)

	return social.UploadKey(
		in.Args.ConnectionID, in.Output.Rel, strconv.FormatInt(in.Output.Bytes, 10), in.Output.SHA256, string(raw),
	)
}

// PublishValidate checks a post and says what a person would be asked to approve. It sends nothing.
func (s *svc) PublishValidate(ctx context.Context, req PublishValidateRequest) (*PublishValidateResult, error) {
	c, err := authorize(ctx, "PublishValidate", req.ProjectID)
	if err != nil {
		return nil, err
	}

	if _, lerr := s.loadProject(ctx, c, req.ProjectID); lerr != nil {
		return nil, lerr
	}

	t, err := s.resolvePublish(ctx, c, req.ProjectID, req.PublishArgs)
	if err != nil {
		return nil, err
	}

	out := &PublishValidateResult{Network: t.desc.Network, State: string(publish.StatePrepared), Issues: t.issues}

	if out.Issues == nil {
		out.Issues = []publish.Issue{}
	}

	if len(t.issues) > 0 {
		out.State = "invalid"

		return out, nil
	}

	out.Summary = publishSummary(t)

	return out, nil
}

// PublishNow starts the job publish.upload: it waits for a person to approve (SDD-015), then sends.
func (s *svc) PublishNow(ctx context.Context, req PublishNowRequest) (*Job, error) {
	raw, err := json.Marshal(req.PublishArgs)
	if err != nil {
		return nil, internalError()
	}

	return s.JobStart(ctx, JobStartRequest{
		ProjectID: req.ProjectID, TimelineID: req.TimelineID, Kind: JobPublishUpload, Args: raw, WaitMS: req.WaitMS,
		DryRun: req.DryRun,
	})
}

// PublishStatus reads the publication of a job. For a video that YouTube was still processing when the
// job ended, it asks the network once.
func (s *svc) PublishStatus(ctx context.Context, req PublishStatusRequest) (*PublishStatusResult, error) {
	job, err := s.JobGet(ctx, JobGetRequest{JobID: req.JobID})
	if err != nil {
		return nil, err
	}

	if job.Kind != JobPublishUpload {
		return nil, newError(command.CodeInvalidState, "the job is not a publication.")
	}

	out := &PublishStatusResult{JobID: job.JobID, Error: job.Error}

	switch job.State {
	case JobWaitingApproval:
		out.State = string(publish.StateWaitingApproval)
	case JobQueued, JobRunning:
		out.State = string(publish.StateUploading)
	case JobSucceeded:
		out.State = string(publish.StateProcessing)

		if job.Result != nil && job.Result.Publish != nil {
			out.Result = job.Result.Publish
			out.State = job.Result.Publish.State
		}
	default:
		out.State = string(publish.StateFailed)
	}

	if out.State == string(publish.StateProcessing) && out.Result != nil {
		s.refreshProcessing(ctx, out)
	}

	return out, nil
}

// refreshProcessing asks the network once whether a video that was still processing is done. A failure
// to ask leaves the state as the job recorded it.
func (s *svc) refreshProcessing(ctx context.Context, out *PublishStatusResult) {
	reg, err := s.socialRegistry()
	if err != nil {
		return
	}

	conn, cerr := reg.Get(out.Result.Network)
	if cerr != nil {
		return
	}

	var remote social.Remote

	werr := s.conns.With(ctx, out.Result.ConnectionID, func(cred connections.Credential) error {
		var serr error

		remote, serr = conn.Status(ctx, cred, out.Result.RemoteID)

		return serr
	})
	if werr != nil {
		return
	}

	res := *out.Result
	res.State = string(remote.State)
	out.Result, out.State = &res, string(remote.State)

	if remote.State == publish.StateFailed {
		out.Error = publishError(social.NewError(social.KindRejected, res.Network, remote.Reason, 0, nil))
	}
}
