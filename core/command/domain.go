package command

import (
	"reflect"

	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Commands of a document type other than the timeline (SDD-021 section 1) live in a registry of their own,
// so that a batch of one kind of document can never reach the commands of another. They share what this
// package already has: the decoder and the schema generator of the arguments, the Spec and its checks,
// Context.NewID and Context.SetEffective, the errors and the style helpers below.

// DefineIn builds a Command whose target is a document of type D that is not a timeline. apply receives the
// document, which is a private copy it may edit freely, and the decoded arguments. Context.Doc is nil for
// such a command; the document is the d argument.
func DefineIn[D, A any](s Spec, apply func(c *Context, d *D, a *A) error) Command {
	return Command{
		Spec:     s,
		argsType: reflect.TypeFor[A](),
		apply: func(c *Context, a any) error {
			d, okDoc := c.Custom.(*D)
			typed, okArgs := a.(*A)
			if !okDoc || !okArgs {
				return Errorf(
					CodeInternal,
					"the command %s was given a document or arguments of the wrong type.",
					s.Name,
				)
			}
			return apply(c, d, typed)
		},
	}
}

// ApplyIn runs one operation of a registry of a non-timeline document on doc, in place: the caller owns the
// private copy and drops it when an error comes back. It returns the effective values of the operation.
func (r *Registry) ApplyIn(doc any, op Operation, env Env) (effective map[string]any, err error) {
	cmd, ok := r.cmds[op.Op]
	if !ok {
		return nil, r.unknown(op.Op)
	}

	args, e := decodeArgs(op.Args, cmd.argsType)
	if e != nil {
		return nil, e
	}

	ctx := &Context{Custom: doc, Env: env, effective: map[string]any{}}

	defer func() {
		if rec := recover(); rec != nil {
			effective, err = nil, Errorf(CodeInternal, "the command %s failed: %v.", op.Op, rec).
				WithHint("Report this as a bug with the operation that was sent.")
		}
	}()

	if cerr := cmd.apply(ctx, args.Addr().Interface()); cerr != nil {
		return nil, AsError(cerr)
	}

	return ctx.effective, nil
}

// ValidateStyle checks a style argument (a partial text style whose null values remove a key) for a document
// that has no template colours: a "$name" reference is refused.
func ValidateStyle(style map[string]any) *Error {
	c := &Context{Doc: &timeline.Document{}}

	return c.validateStyle(style, false, true)
}

// MergeStyle merges a partial style into the stored one (a null value removes a key). An empty result is nil.
func MergeStyle(current *timeline.CaptionStyle, given map[string]any) (*timeline.CaptionStyle, *Error) {
	return mergeStyle(current, given, false)
}

// CheckStyleFont returns E_FONT_UNKNOWN when the family or the weight of a style is not in the bundled set.
func CheckStyleFont(s *timeline.CaptionStyle) *Error {
	c := &Context{Doc: &timeline.Document{}}

	return c.checkFont(s)
}

// Unknown is the E_UNKNOWN_COMMAND of a name this registry does not have, listing the names it has.
func (r *Registry) Unknown(name string) *Error { return r.unknown(name) }
