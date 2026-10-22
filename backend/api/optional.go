package api

import (
	"encoding/json"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// Optional is a request field that tells "left out" (Set false) from null (Set true, Value nil).
// Tag it `json:",omitempty"` so huma does not require it.
type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	return json.Unmarshal(b, &o.Value)
}

func (Optional[T]) Schema(r huma.Registry) *huma.Schema {
	t := reflect.TypeFor[T]()
	s := *r.Schema(t, true, t.Name())
	if s.Ref != "" {
		return &huma.Schema{OneOf: []*huma.Schema{&s, {Type: "null"}}}
	}
	s.Nullable = true
	return &s
}
