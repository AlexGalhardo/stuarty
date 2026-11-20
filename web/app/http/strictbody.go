package httpapi

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

// strictBody refuses a JSON body that names a member the input type of the route does not have: E_VALIDATION
// with the path of the member (SDD-003 section 1 item 7), as the MCP server does. vukem binds the body leniently,
// so a typo such as dryrun would apply an edit for real (VV-302). Query parameters stay free; members whose type
// is a map or any (the arguments of an operation) are the Service's to validate. A route that declares no input
// (the upload of a file, whose body is the file) reads no member, so it has none to misspell.
func strictBody[In any](skip func() bool) func(http.Handler) http.Handler {
	t := reflect.TypeFor[In]()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if t.Kind() != reflect.Struct || t.NumField() == 0 || skip() || r.Body == nil || r.Body == http.NoBody ||
				!isJSONBody(r) {
				next.ServeHTTP(w, r)
				return
			}

			head, err := io.ReadAll(io.LimitReader(r.Body, web.DefaultMaxBody+1))
			if err != nil || len(head) > web.DefaultMaxBody {
				// Too large to inspect here, or unreadable: the binder reports it as it always did.
				r.Body = joinedBody{Reader: io.MultiReader(bytes.NewReader(head), r.Body), Closer: r.Body}
				next.ServeHTTP(w, r)

				return
			}

			r.Body = io.NopCloser(bytes.NewReader(head))

			if path, bad := unknownMember(t, head, ""); bad {
				web.Fail(w, r, &service.Error{
					Code:    command.CodeValidation,
					Path:    path,
					Message: "the member " + path + " is not part of this request.",
					Hint:    "check the spelling in the OpenAPI document; a misspelled option is refused instead of ignored",
				})

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

type joinedBody struct {
	io.Reader
	io.Closer
}

func isJSONBody(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))

	return err == nil && (mt == "application/json" || strings.HasSuffix(mt, "+json"))
}

var (
	jsonUnmarshaler = reflect.TypeFor[json.Unmarshaler]()
	textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()
	timeType        = reflect.TypeFor[time.Time]()
)

// unknownMember returns the path of the first member of raw that the struct type t has no field for.
func unknownMember(t reflect.Type, raw []byte, prefix string) (string, bool) {
	t = indirect(t)

	switch t.Kind() {
	case reflect.Struct:
		if t == timeType || decodesItself(t) {
			return "", false
		}

		var members map[string]json.RawMessage
		if json.Unmarshal(raw, &members) != nil {
			return "", false
		}

		fields := memberTypes(t)

		for _, name := range slices.Sorted(maps.Keys(members)) {
			path := name
			if prefix != "" {
				path = prefix + "." + name
			}

			ft, ok := fields[name]
			if !ok {
				return path, true
			}

			if path, bad := unknownMember(ft, members[name], path); bad {
				return path, true
			}
		}
	case reflect.Slice, reflect.Array:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return "", false
		}

		for i, item := range items {
			if path, bad := unknownMember(t.Elem(), item, fmt.Sprintf("%s[%d]", prefix, i)); bad {
				return path, true
			}
		}
	default:
	}

	return "", false
}

func indirect(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

func decodesItself(t reflect.Type) bool {
	p := reflect.PointerTo(t)

	return p.Implements(jsonUnmarshaler) || p.Implements(textUnmarshaler)
}

// memberTypes maps the JSON member names of a struct to their types, flattening embedded structs.
func memberTypes(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}

	for f := range t.Fields() {
		tag, hasTag := f.Tag.Lookup("json")
		name, _, _ := strings.Cut(tag, ",")

		if name == "-" && !strings.Contains(tag, ",") {
			continue
		}

		if f.Anonymous && name == "" {
			if et := indirect(f.Type); et.Kind() == reflect.Struct {
				maps.Copy(out, memberTypes(et))

				continue
			}
		}

		if !f.IsExported() {
			continue
		}

		if !hasTag || name == "" {
			name = f.Name
		}

		out[name] = f.Type
	}

	return out
}

// networkOnly reports the routes that exist in network mode only: in local mode they answer E_NOT_FOUND whatever
// the body says, so the body is not read for a misspelled member first.
func networkOnly(id string) bool {
	return strings.HasPrefix(id, "auth.") || strings.HasPrefix(id, "google.") || strings.HasPrefix(id, "users.")
}
