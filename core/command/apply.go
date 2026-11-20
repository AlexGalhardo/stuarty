package command

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/core/timeval"
)

// Operation is one use of a command: {"op": "<command>", "args": {...}}.
type Operation struct {
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

// Env is what the caller supplies to Apply. Apply itself is pure: no clock, no
// randomness, no I/O.
type Env struct {
	// NewIDs returns a fresh identifier for a prefix; deterministic in tests,
	// random in production. A command uses it only when the caller supplied no id.
	NewIDs func(prefix string) string

	// Extra is what the commands of another document type need besides identifiers: the lookup of a file and
	// the text measurer of an image (SDD-021 section 4). The timeline commands ignore it.
	Extra any

	// Timelines is how the composition commands read the other timelines of the production (SDD-002 A.3,
	// SDD-001 section 3.5.1): the caller answers from its own store, so Apply stays pure. Nil when the caller
	// has no production.
	Timelines timeline.Graph
}

// Result is the outcome of one operation.
type Result struct {
	Doc       *timeline.Document // the new document; the input is not modified
	Inverse   []Primitive        // undo (SDD-002 §A.6), in the order they were computed
	Diff      []Change           // SDD-002 §A.5
	Effective map[string]any     // the values actually applied (SDD-002 §A.4)
	// Spawned are the timelines the operation made besides Doc (composition.create_from_selection); the
	// caller stores them in the same transaction as Doc.
	Spawned []*timeline.Document
}

// BatchResult is the outcome of a batch: the net difference of all its operations.
type BatchResult struct {
	Doc       *timeline.Document
	Inverse   []Primitive
	Diff      []Change
	Effective []map[string]any // one entry per operation, in order
	// Spawned are the timelines the operations made, in order (see Result.Spawned).
	Spawned []*timeline.Document
}

// NoOp reports whether the batch left the document as it was: nothing to commit, so no
// revision and no undo step (VV-050). The effective values still describe each operation.
func (b BatchResult) NoOp() bool { return len(b.Diff) == 0 && len(b.Inverse) == 0 }

// Context is what a command sees: the working document, which it may edit freely
// because Apply gives each operation a private copy, and the environment.
type Context struct {
	Doc *timeline.Document
	Env Env

	// Custom is the document of a command defined with DefineIn, a type other than the timeline.
	Custom any

	effective map[string]any
	spawned   *[]*timeline.Document
}

// Spawn records a new timeline that the command made (SDD-002 A.3): the caller stores it with the commit.
func (c *Context) Spawn(d *timeline.Document) {
	if c.spawned != nil {
		*c.spawned = append(*c.spawned, d)
	}
}

// NewID returns an identifier for a new object: supplied when the caller gave one
// in its arguments, otherwise from Env.NewIDs (SDD-002 §A.3 rule 4). A caller-given
// id that is already used is the command's E_ID_EXISTS to report.
func (c *Context) NewID(prefix, supplied string) (string, *Error) {
	if supplied != "" {
		return supplied, nil
	}
	if c.Env.NewIDs == nil {
		return "", Errorf(CodeInternal, "an identifier was needed but the caller supplied no generator.")
	}
	return c.Env.NewIDs(prefix), nil
}

// FPS returns the frame rate of the working document.
func (c *Context) FPS() timeval.FPS {
	f := c.Doc.Settings.FPS
	return timeval.FPS{Num: int64(f.Num), Den: int64(f.Den)}
}

// SetEffective records a value actually applied, under key.
func (c *Context) SetEffective(key string, value any) {
	c.effective[key] = value
}

// SetEffectiveTime records a time argument as {"frame", "seconds", "requested"}.
func (c *Context) SetEffectiveTime(key string, frame int, requested string) {
	c.effective[key] = TimeEffective(c.FPS(), frame, requested)
}

// ParseTime parses a time argument (SDD-002 §A.4) at the project frame rate. A bad
// string is E_INVALID_TIME with the path of the argument.
func (c *Context) ParseTime(path, s string) (int, *Error) {
	frame, err := timeval.Parse(s, c.FPS())
	if err != nil {
		if te, ok := errors.AsType[*timeval.Error](err); ok {
			return 0, Errorf(
				CodeInvalidTime,
				"%s.",
				sentence(te.Message),
			).WithHint("%s.", sentence(te.Hint)).
				WithPath(path)
		}
		return 0, AsError(err)
	}
	return int(frame), nil
}

// TimeEffective is the effective entry of a time argument: the frame applied, its
// seconds to the millisecond, and the string that was requested (null when the
// value is a default).
func TimeEffective(fps timeval.FPS, frame int, requested string) map[string]any {
	var req any
	if requested != "" {
		req = requested
	}
	return map[string]any{"frame": frame, "seconds": fps.Seconds(int64(frame)), "requested": req}
}

// Apply applies one operation to doc and returns the new document, the inverse
// primitives, the diff and the effective values. It validates the arguments, runs
// the command on a private copy and leaves doc untouched, also on error.
func (r *Registry) Apply(doc *timeline.Document, op Operation, env Env) (Result, error) {
	work := timeline.Clone(doc)
	var spawned []*timeline.Document
	effective, err := r.run(work, op, env, &spawned)
	if err != nil {
		return Result{}, err
	}
	before, after := rawPair(doc, work)
	return Result{
		Doc:       work,
		Inverse:   inverseOf(before, after),
		Diff:      diffOf(fullPair(doc, work)),
		Effective: effective,
		Spawned:   spawned,
	}, nil
}

// run applies op to work in place. work is the caller's private copy; on error it
// may be half edited and must be dropped. The timelines the command spawns are appended to spawned.
func (r *Registry) run(
	work *timeline.Document, op Operation, env Env, spawned *[]*timeline.Document,
) (effective map[string]any, err error) {
	cmd, ok := r.cmds[op.Op]
	if !ok {
		return nil, r.unknown(op.Op)
	}
	args, e := decodeArgs(op.Args, cmd.argsType)
	if e != nil {
		return nil, e
	}
	ctx := &Context{Doc: work, Env: env, effective: map[string]any{}, spawned: spawned}
	locks := lockedItems(work)
	defer func() {
		if recover() != nil {
			effective, err = nil, AsError(nil)
		}
	}()
	if cerr := cmd.apply(ctx, args.Addr().Interface()); cerr != nil {
		return nil, AsError(cerr)
	}
	packMagnetTracks(work)
	if lerr := checkLocks(locks, work); lerr != nil {
		return nil, lerr
	}
	if r.Debug {
		if vs := timeline.Validate(work); len(vs) > 0 {
			return nil, validationBug(op.Op, vs)
		}
	}
	return ctx.effective, nil
}

func validationBug(name string, vs []timeline.Violation) *Error {
	codes := make([]string, 0, len(vs))
	for _, v := range vs {
		codes = append(codes, v.Code+" "+v.Path)
	}
	return Errorf(CodeInternal, "the command %s produced an invalid document; the batch was not applied.", name).
		WithHint("Report this as a bug with the operation that was sent.").
		WithDetail("violations", codes)
}

// ApplyBatch applies the operations in order, each to the document the previous one
// produced. If one fails the whole batch fails with that error and op_index set; doc
// is untouched. The diff and the inverse are those of the net change.
func (r *Registry) ApplyBatch(doc *timeline.Document, ops []Operation, env Env) (BatchResult, error) {
	if len(ops) == 0 {
		return BatchResult{}, Errorf(CodeValidation, "The batch has no operations.").
			WithPath("ops").WithHint("Send at least one operation.")
	}
	work := timeline.Clone(doc) // one private copy for the whole batch; dropped if an operation fails
	effective := make([]map[string]any, 0, len(ops))
	var spawned []*timeline.Document
	for i, op := range ops {
		eff, err := r.run(work, op, env, &spawned)
		if err != nil {
			e := *AsError(err)
			idx := i
			e.OpIndex = &idx
			return BatchResult{}, &e
		}
		effective = append(effective, eff)
	}
	before, after := rawPair(doc, work)
	return BatchResult{
		Doc:       work,
		Inverse:   inverseOf(before, after),
		Diff:      diffOf(fullPair(doc, work)),
		Effective: effective,
		Spawned:   spawned,
	}, nil
}

// sentence capitalizes the first letter of s.
func sentence(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
