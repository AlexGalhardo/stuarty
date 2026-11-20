package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
)

// Scope is the permission a caller needs to issue a command (ADR-0019).
type Scope string

// The scopes of a command.
const (
	ScopeEdit  Scope = "edit"
	ScopeMedia Scope = "media"
)

// Spec describes a command. Everything a surface shows about a command comes from
// here and from the arguments type (SDD-002 §A.2).
type Spec struct {
	Name        string   // namespace.action, lower case, stable
	Summary     string   // one sentence, tool and help text
	Description string   // a short paragraph for command_help
	Example     string   // a valid args value, as JSON
	Errors      []string // the error codes the command can return
	Scope       Scope
	Internal    bool // only the service may issue it (SDD-002 §A.9)
	// Domain is the domain of the command (SDD-000 section 1.1). It is the domain of the registry that holds it:
	// a command may leave it empty and inherit, or name it and be refused by a registry of another domain.
	Domain string
}

// Command is a registered kind of edit: its Spec, the Go struct that is the schema
// of its arguments, and the function that applies it.
type Command struct {
	Spec
	argsType reflect.Type
	apply    func(*Context, any) error
}

// Define builds a Command whose arguments are the struct A. The tags of A are
// documented in args.go. apply receives a pointer to the decoded arguments and
// edits c.Doc, which is a private copy; it returns an *Error for a failed
// precondition.
func Define[A any](s Spec, apply func(c *Context, a *A) error) Command {
	return Command{
		Spec:     s,
		argsType: reflect.TypeFor[A](),
		apply: func(c *Context, a any) error {
			typed, ok := a.(*A)
			if !ok {
				return Errorf(CodeInternal, "the arguments have an unexpected type.")
			}
			return apply(c, typed)
		},
	}
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9]*\.[a-z][a-z0-9_]*$`)

// Check returns every way in which the command is incomplete: no schema, example,
// error list, description or scope, an example that its own schema rejects,
// or an unknown error code.
func (c Command) Check() error {
	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }
	if !namePattern.MatchString(c.Name) {
		add("name %q is not namespace.action in lower case", c.Name)
	}
	if strings.TrimSpace(c.Summary) == "" {
		add("no summary")
	}
	if strings.TrimSpace(c.Description) == "" {
		add("no description")
	}
	if c.apply == nil {
		add("no apply function")
	}
	switch {
	case c.argsType == nil || c.argsType.Kind() != reflect.Struct:
		add("no arguments schema (the arguments must be a struct)")
	case strings.TrimSpace(c.Example) == "":
		add("no example")
	default:
		if _, e := decodeArgs(json.RawMessage(c.Example), c.argsType); e != nil {
			add("the example does not satisfy the schema: %s", e.Message)
		}
	}
	if len(c.Errors) == 0 {
		add("no error list")
	}
	seen := map[string]bool{}
	for _, code := range c.Errors {
		if !KnownCode(code) {
			add("unknown error code %q", code)
		}
		if seen[code] {
			add("error code %s listed twice", code)
		}
		seen[code] = true
	}
	if c.Scope != ScopeEdit && c.Scope != ScopeMedia {
		add("scope must be edit or media")
	}

	if !KnownDomain(c.Domain) {
		add("no domain (%q is not one of %s)", c.Domain, strings.Join(DomainNames(), ", "))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("command %q: %s", c.Name, strings.Join(problems, "; "))
}

// Registry holds the commands of one build.
type Registry struct {
	// Debug makes Apply run timeline.Validate on every result and report a
	// violation as E_INTERNAL (SDD-002 §A.3 rule 3). Tests turn it on.
	Debug bool

	domain string
	cmds   map[string]*Command
}

// NewRegistry returns an empty registry of one domain, the kind of document its commands edit (the timeline
// registry is DomainVideo, the image registry DomainImage). A domain that is not one of SDD-000 section 1.1
// makes every Register fail, so a later epic cannot register operations that no domain owns.
func NewRegistry(domain string) *Registry {
	return &Registry{domain: domain, cmds: map[string]*Command{}}
}

// Domain is the domain of the registry.
func (r *Registry) Domain() string { return r.domain }

// Register adds a command. An incomplete command, one whose name is taken and one with no domain, or with a
// domain other than its registry's, is an error. The command takes the domain of the registry.
func (r *Registry) Register(c Command) error {
	if !KnownDomain(r.domain) {
		return fmt.Errorf("command %q: the registry has no domain (%q is not one of %s)",
			c.Name, r.domain, strings.Join(DomainNames(), ", "))
	}

	if c.Domain != "" && c.Domain != r.domain {
		return fmt.Errorf(
			"command %q: domain %q differs from the domain %q of its registry",
			c.Name, c.Domain, r.domain,
		)
	}

	c.Domain = r.domain

	if err := c.Check(); err != nil {
		return err
	}
	if _, dup := r.cmds[c.Name]; dup {
		return fmt.Errorf("command %q is registered twice", c.Name)
	}
	r.cmds[c.Name] = &c
	return nil
}

// MustRegister registers commands and panics on an error: for init functions.
func (r *Registry) MustRegister(cs ...Command) {
	for i := range cs {
		if err := r.Register(cs[i]); err != nil {
			panic(err)
		}
	}
}

// Get returns the command with that name.
func (r *Registry) Get(name string) (*Command, bool) {
	c, ok := r.cmds[name]
	return c, ok
}

// Lookup is Get.
func (r *Registry) Lookup(name string) (*Command, bool) { return r.Get(name) }

// List returns every registered command sorted by name.
func (r *Registry) List() []*Command {
	out := make([]*Command, 0, len(r.cmds))
	for _, c := range r.cmds {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b *Command) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Names returns the sorted names of the registered commands.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.cmds))
	for n := range r.cmds {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Schema returns the JSON Schema of the arguments of a command, with
// additionalProperties false at every object level.
func (r *Registry) Schema(name string) (json.RawMessage, error) {
	c, ok := r.cmds[name]
	if !ok {
		return nil, r.unknown(name)
	}
	s := schemaOf(c.argsType)
	s["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	b, err := json.Marshal(s)
	if err != nil {
		return nil, AsError(err)
	}
	return b, nil
}

// Help returns the help text of a command: what it does, its arguments, an
// example and its errors.
func (r *Registry) Help(name string) (string, error) {
	c, ok := r.cmds[name]
	if !ok {
		return "", r.unknown(name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n\n%s\n\nArguments:\n", c.Name, c.Summary, c.Description)
	fields := argFields(c.argsType)
	if len(fields) == 0 {
		b.WriteString("  none\n")
	}
	for _, f := range fields {
		need := "optional"
		if f.required {
			need = "required"
		}
		fmt.Fprintf(&b, "  %s (%s, %s)", f.name, typeWord(f.fieldType), need)
		if f.enum != nil {
			fmt.Fprintf(&b, " one of %s", strings.Join(f.enum, ", "))
		}
		if f.min != nil || f.max != nil {
			fmt.Fprintf(&b, " %s", rangeWord(f))
		}
		if f.desc != "" {
			fmt.Fprintf(&b, ": %s", f.desc)
		}
		b.WriteByte('\n')
		if shape := itemShape(f.fieldType); shape != "" {
			fmt.Fprintf(&b, "    each item of %s: %s\n", f.name, shape)
		}
	}
	fmt.Fprintf(&b, "\nExample: {\"op\": %q, \"args\": %s}\n", c.Name, c.Example)
	b.WriteString(
		"The ids in the example are placeholders; replace them with ids from the project before editing, or the command fails with E_NOT_FOUND.\n",
	)
	fmt.Fprintf(&b, "Errors: %s\nScope: %s\n", strings.Join(c.Errors, ", "), c.Scope)
	if schema, err := r.Schema(name); err == nil {
		fmt.Fprintf(&b, "JSON Schema: %s\n", schema)
	}
	return b.String(), nil
}

// itemShape describes the object an array argument holds, as "{name (type,
// required), ...}", so a caller sees the shape of words[] or files[] before it
// sends one. It is empty for any other type.
func itemShape(t reflect.Type) string {
	if t.Kind() != reflect.Slice {
		return ""
	}
	el := t.Elem()
	if el.Kind() != reflect.Struct {
		return ""
	}
	parts := []string{}
	for _, f := range argFields(el) {
		need := "optional"
		if f.required {
			need = "required"
		}
		parts = append(parts, fmt.Sprintf("%s (%s, %s)", f.name, typeWord(f.fieldType), need))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func (r *Registry) unknown(name string) *Error {
	known := r.Names()
	return Errorf(CodeUnknownCommand, "There is no command named %q.", name).
		WithHint("Use one of the known commands: %s.", strings.Join(known, ", ")).
		WithDetail("known", known)
}

var (
	defaultOnce sync.Once
	defaultReg  *Registry
	registered  []Command
)

// Default is the registry of this binary, filled by the init functions of the
// command files: every registered command is exposed.
func Default() *Registry {
	defaultOnce.Do(func() { defaultReg = NewRegistry(DomainVideo) })
	return defaultReg
}

// FullRegistry returns a new registry with every registered command. Tests and
// the generators use it when they need a registry of their own.
func FullRegistry() *Registry {
	r := NewRegistry(DomainVideo)
	for i := range registered {
		if err := r.Register(registered[i]); err != nil {
			panic(err)
		}
	}

	return r
}

// Register adds a command to the Default registry and panics when it is incomplete;
// command files call it from init.
func Register(c Command) {
	if err := Default().Register(c); err != nil {
		panic(errors.Join(errors.New("command registry"), err))
	}

	registered = append(registered, c)
}
