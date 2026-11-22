package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

type flagKind int

const (
	kindString flagKind = iota
	kindBool
	kindInt
	kindDuration
)

// flagDef declares one flag of a command.
type flagDef struct {
	Name    string
	Kind    flagKind
	Default string
	Usage   string
	// Repeat lets the flag appear several times; the values keep their order.
	Repeat bool
}

// parsed holds the flags and the positional arguments of one invocation.
type parsed struct {
	defs       map[string]flagDef
	values     map[string][]string
	Positional []string
	Help       bool
}

// validation builds the E_VALIDATION error the CLI raises for a bad invocation.
// jsonProblem says in a few words what is wrong with a JSON argument, without
// the Go type names of the decoder's own message.
func jsonProblem(err error) string {
	if ute, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		if ute.Field != "" {
			return fmt.Sprintf("%s must be %s, not %s", command.SafePath(ute.Field), jsonKind(ute.Type), ute.Value)
		}

		return fmt.Sprintf("a value must be %s, not %s", jsonKind(ute.Type), ute.Value)
	}

	if se, ok := errors.AsType[*json.SyntaxError](err); ok {
		return fmt.Sprintf("it is not valid JSON (at byte %d)", se.Offset)
	}

	if name, found := command.UnknownMember(err); found {
		return "the member " + name + " is not known"
	}

	return "it is not valid"
}

func jsonKind(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return "an object"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	default:
		return "a number"
	}
}

func validation(path, format string, args ...any) *service.Error {
	return &service.Error{Code: command.CodeValidation, Message: fmt.Sprintf(format, args...), Path: path}
}

// parseArgs reads flags and positional arguments in any order (SDD-003 section
// 5.1): a flag may follow a positional argument. Everything after "--" is
// positional. A single-dash argument is positional, except -h.
func parseArgs(defs []flagDef, args []string) (*parsed, error) {
	p := &parsed{defs: map[string]flagDef{}, values: map[string][]string{}}
	for _, d := range defs {
		p.defs[d.Name] = d
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		switch {
		case arg == "--":
			p.Positional = append(p.Positional, args[i+1:]...)
			return p, nil
		case arg == "-h" || arg == "--help":
			p.Help = true
		case strings.HasPrefix(arg, "--") && len(arg) > 2:
			consumed, err := p.readFlag(arg[2:], args[i+1:])
			if err != nil {
				return nil, err
			}

			i += consumed
		default:
			p.Positional = append(p.Positional, arg)
		}
	}

	return p, nil
}

func (p *parsed) isBool(name string) bool {
	d, ok := p.defs[name]

	return ok && d.Kind == kindBool
}

// readFlag reads one "--name[=value]" flag and reports how many following
// arguments it used.
func (p *parsed) readFlag(body string, rest []string) (int, error) {
	name, value, hasValue := strings.Cut(body, "=")

	def, ok := p.defs[name]
	if !ok {
		if base, found := strings.CutPrefix(name, "no-"); found && !hasValue && p.isBool(base) {
			p.values[base] = []string{"false"}
			return 0, nil
		}

		return 0, validation("", "unknown flag --%s", name)
	}

	consumed := 0

	if def.Kind == kindBool {
		if !hasValue {
			value = "true"
		}
	} else if !hasValue {
		if len(rest) == 0 {
			return 0, validation(name, "flag --%s needs a value", name)
		}

		value, consumed = rest[0], 1
	}

	if err := checkValue(def, value); err != nil {
		return 0, err
	}

	if def.Repeat {
		p.values[name] = append(p.values[name], value)
	} else {
		p.values[name] = []string{value}
	}

	return consumed, nil
}

func checkValue(def flagDef, value string) error {
	var err error

	switch def.Kind {
	case kindBool:
		_, err = strconv.ParseBool(value)
	case kindInt:
		var n int

		if n, err = strconv.Atoi(value); err == nil && def.Name == "limit" && n < 1 {
			return validation("limit", "flag --limit: %d is below 1; omit it for the default page size", n)
		}
	case kindDuration:
		_, err = time.ParseDuration(value)
	case kindString:
	}

	if err != nil {
		// The value is not repeated: it can be huge, and a validation message names the constraint only.
		return &service.Error{
			Code:    command.CodeValidation,
			Path:    def.Name,
			Message: fmt.Sprintf("flag --%s: the value is not a valid %s", def.Name, kindName(def.Kind)),
			Hint:    flagValueHint(def.Kind),
		}
	}

	return nil
}

func flagValueHint(k flagKind) string {
	switch k {
	case kindBool:
		return "use true or false"
	case kindInt:
		return "use a whole number"
	case kindDuration:
		return "use a duration such as 30s or 10m"
	case kindString:
	}

	return ""
}

func kindName(k flagKind) string {
	switch k {
	case kindBool:
		return "boolean"
	case kindInt:
		return "integer"
	case kindDuration:
		return "duration"
	case kindString:
	}

	return "string"
}

// has reports whether the flag was given on the command line.
func (p *parsed) has(name string) bool { return len(p.values[name]) > 0 }

// str returns the last value of a flag, or its default.
func (p *parsed) str(name string) string {
	if v := p.values[name]; len(v) > 0 {
		return v[len(v)-1]
	}

	return p.defs[name].Default
}

// all returns every value of a repeatable flag.
func (p *parsed) all(name string) []string { return p.values[name] }

func (p *parsed) boolean(name string) bool {
	v, _ := strconv.ParseBool(p.str(name))
	return v
}

func (p *parsed) integer(name string) int {
	v, _ := strconv.Atoi(p.str(name))
	return v
}

// timeout is the --timeout flag.
func (p *parsed) timeout() time.Duration {
	v, _ := time.ParseDuration(p.str("timeout"))
	return v
}
