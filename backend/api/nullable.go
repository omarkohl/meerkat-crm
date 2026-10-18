package api

import (
	"encoding/json"
	"reflect"

	"github.com/danielgtaylor/huma/v2"
)

// Nullable is a value that may be null in a response. huma cannot mark a referenced struct
// schema nullable, so the schema is the struct or null.
type Nullable[T any] struct{ Value *T }

func (Nullable[T]) Schema(r huma.Registry) *huma.Schema {
	t := reflect.TypeFor[T]()
	return &huma.Schema{OneOf: []*huma.Schema{r.Schema(t, true, t.Name()), {Type: "null"}}}
}

func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}
