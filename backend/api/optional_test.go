package api_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"meerkat/api"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionalTellsALeftOutFieldFromNull(t *testing.T) {
	type body struct {
		At api.Optional[time.Time] `json:"at,omitempty"`
	}
	decode := func(s string) api.Optional[time.Time] {
		var b body
		require.NoError(t, json.Unmarshal([]byte(s), &b))
		return b.At
	}

	assert.False(t, decode(`{}`).Set)
	assert.True(t, decode(`{"at":null}`).Set)
	assert.Nil(t, decode(`{"at":null}`).Value)
	v := decode(`{"at":"2026-03-04T00:00:00Z"}`)
	assert.True(t, v.Set)
	require.NotNil(t, v.Value)
	assert.True(t, time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC).Equal(*v.Value))
}

func TestOptionalSchemaIsTheValueOrNull(t *testing.T) {
	r := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	s := r.Schema(reflect.TypeFor[api.Optional[time.Time]](), true, "")

	assert.Equal(t, "string", s.Type)
	assert.Equal(t, "date-time", s.Format)
	assert.True(t, s.Nullable)
}
