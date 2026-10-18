package domain_test

import (
	"testing"

	"meerkat/admin/internal/domain"

	"github.com/stretchr/testify/assert"
)

func TestAuditFilterNormalised(t *testing.T) {
	f := domain.AuditFilter{Page: -3, Limit: 0, Username: "  AliCe "}.Normalised()
	assert.Equal(t, domain.AuditFilter{Page: 1, Limit: domain.DefaultLimit, Username: "alice"}, f)
	assert.Equal(t, 50, domain.AuditFilter{Page: 3, Limit: 25}.Normalised().Offset())
}
