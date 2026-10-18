package api_test

import (
	"encoding/json"
	"testing"

	"meerkat/api"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type thing struct {
	Name string `json:"name"`
}

func TestNullableMarshalsNullOrTheValue(t *testing.T) {
	out, err := json.Marshal(struct {
		A api.Nullable[thing] `json:"a"`
		B api.Nullable[thing] `json:"b"`
	}{B: api.Nullable[thing]{Value: &thing{Name: "x"}}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":null,"b":{"name":"x"}}`, string(out))
}
