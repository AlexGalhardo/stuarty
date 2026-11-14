package timeline

import "reflect"

// Clone returns a deep copy of doc: no map, slice or pointer is shared with it.
func Clone(doc *Document) *Document {
	if doc == nil {
		return nil
	}
	out, _ := reflect.TypeAssert[*Document](deepCopy(reflect.ValueOf(doc)))
	return out
}

// CloneItem returns a deep copy of an item.
func CloneItem(it Item) Item {
	out, _ := reflect.TypeAssert[Item](deepCopy(reflect.ValueOf(it)))
	return out
}

// Raw returns doc as a RawDoc in canonical form: a field equal to its default is
// absent. It is the form in which a diff and the primitives of SDD-002 §A.5 and
// §A.6 see a document.
func Raw(doc *Document) RawDoc {
	raw, err := DecodeRaw(Canonical(doc))
	if err != nil {
		panic("timeline: canonical encoding is not valid JSON: " + err.Error())
	}
	return raw
}

func deepCopy(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type().Elem())
		out.Elem().Set(deepCopy(v.Elem()))
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		for i := range v.NumField() {
			out.Field(i).Set(deepCopy(v.Field(i)))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		for _, k := range v.MapKeys() {
			out.SetMapIndex(k, deepCopy(v.MapIndex(k)))
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := range v.Len() {
			out.Index(i).Set(deepCopy(v.Index(i)))
		}
		return out
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(deepCopy(v.Elem()))
		return out
	default:
		return v
	}
}
