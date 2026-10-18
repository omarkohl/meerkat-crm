package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"meerkat/api"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// body carries one field per project validator (the six registered in
// middleware/validation.go), as the DTOs in models do.
type body struct {
	Phone    string   `json:"phone,omitempty" validate:"phone"`
	Birthday string   `json:"birthday,omitempty" validate:"birthday"`
	Password string   `json:"password,omitempty" validate:"omitempty,strong_password"`
	Circles  []string `json:"circles,omitempty" validate:"unique_circles"`
	Username string   `json:"username,omitempty" validate:"no_at_sign"`
	URL      string   `json:"url,omitempty" validate:"safeurl"`
}

type validatedIn struct {
	Body body
}

func (i *validatedIn) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

func validatedAPI() http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	a := api.New(r.Group("/api/v1"), r)
	api.Register(a, huma.Operation{OperationID: "validated", Method: http.MethodPost, Path: "/validated"},
		func(context.Context, *validatedIn) (*struct{}, error) { return nil, nil })
	return r
}

func TestTheSixProjectValidatorsRunOnHumaBodies(t *testing.T) {
	h := validatedAPI()
	valid := `{"phone":"+1 555-0101","birthday":"--03-15","circles":["a","b"],"username":"alice","url":"https://example.com"}`
	w0 := do(h, http.MethodPost, "/api/v1/validated", valid)
	require.Equal(t, http.StatusNoContent, w0.Code, w0.Body.String())

	cases := map[string]string{
		"phone":    `{"phone":"abc"}`,
		"birthday": `{"birthday":"15.03.1990"}`,
		"password": `{"password":"password"}`,
		"circles":  `{"circles":["Friends","friends"]}`,
		"username": `{"username":"a@b"}`,
		"url":      `{"url":"javascript:alert(1)"}`,
	}
	for field, payload := range cases {
		t.Run(field, func(t *testing.T) {
			w := do(h, http.MethodPost, "/api/v1/validated", payload)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			var b errBody
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &b))
			assert.Equal(t, "VALIDATION_ERROR", b.Error.Code)
			assert.Contains(t, b.Error.Details, field, "details are keyed by the JSON name")
		})
	}
}

func TestCirclesThatDifferOnlyInWhitespaceAreDuplicates(t *testing.T) {
	h := validatedAPI()
	assert.Equal(t, http.StatusBadRequest, do(h, http.MethodPost, "/api/v1/validated", `{"circles":["family"," family ","work"]}`).Code)
	assert.Equal(t, http.StatusNoContent, do(h, http.MethodPost, "/api/v1/validated", `{"circles":[" family ","work","friends "]}`).Code)
}
