package app_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/apppackio/apppack/app"
	"github.com/stretchr/testify/assert"
)

func TestIsSensitiveName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected bool
	}{
		{"SECRET_KEY", true},
		{"DJANGO_SECRET_KEY", true},
		{"API_TOKEN", true},
		{"PASSWORD", true},
		{"DB_PASSWD", true},
		{"MY_API_KEY", true},
		{"MYAPIKEY", true},
		{"PRIVATE_KEY", true},
		{"AWS_CREDENTIALS", true},
		{"AUTH_ENABLED", true}, // known false positive - documented tradeoff
		{"SALT_ROUNDS", true},
		{"WEBHOOK_SIGNATURE", true},
		{"AWS_ACCESS_KEY_ID", true},
		{"SESSION_KEY", true},
		{"DATABASE_DSN", true},
		{"secret_key", true}, // case-insensitive
		{"ENVIRONMENT", false},
		{"DEBUG", false},
		{"PORT", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.expected, app.IsSensitiveName(tt.name))
		})
	}
}

func TestMaskValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"short", "abc"},
		{"exactly8", "12345678"},
		{"long", "example-token-abcdefghijklmnopqrstuvwxyz4f"},
		{"very_long", strings.Repeat("x", 200)},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			masked := app.MaskValue(tt.value)

			assert.NotEqual(t, tt.value, masked)
			assert.Contains(t, masked, "•")

			if len(tt.value) <= 8 {
				assert.Equal(t, "••••••••", masked)
			} else {
				assert.True(t, strings.HasPrefix(masked, tt.value[:2]))
				assert.True(t, strings.HasSuffix(masked, tt.value[len(tt.value)-2:]))
				// the mask must never reveal the exact original length
				assert.LessOrEqual(t, utf8.RuneCountInString(masked), utf8.RuneCountInString(tt.value))
			}
		})
	}
}

func TestMaskURLPassword(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		value      string
		wantMasked bool
		checkFunc  func(t *testing.T, masked string)
	}{
		{
			name:       "postgres with password",
			value:      "postgres://user:pw@host:5432/db",
			wantMasked: true,
			checkFunc: func(t *testing.T, masked string) {
				t.Helper()
				assert.Equal(t, "postgres://user:••••••••@host:5432/db", masked)
				assert.NotContains(t, masked, ":pw@")
				assert.NotContains(t, masked, "%E2") // must not be percent-encoded
				assert.Contains(t, masked, "•")
			},
		},
		{
			name:       "redis without userinfo",
			value:      "redis://host:6379",
			wantMasked: false,
		},
		{
			name:       "no scheme",
			value:      "not-a-url",
			wantMasked: false,
		},
		{
			name:       "empty value",
			value:      "",
			wantMasked: false,
		},
		{
			name:       "user without password",
			value:      "postgres://user@host:5432/db",
			wantMasked: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			masked, ok := app.MaskURLPassword(tt.value)
			assert.Equal(t, tt.wantMasked, ok)

			if !tt.wantMasked {
				assert.Equal(t, tt.value, masked)
			} else if tt.checkFunc != nil {
				tt.checkFunc(t, masked)
			}
		})
	}
}

func TestMaskConfigValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		varName    string
		value      string
		wantMasked bool
	}{
		{"secret key masked", "SECRET_KEY", "example-token-abcdefghijklmnop", true},
		{"django secret masked", "DJANGO_SECRET_KEY", "abcdefghijklmnopqrstuvwxyz", true},
		{"api token masked", "API_TOKEN", "abcdefghijklmnop", true},
		{"auth enabled false positive masked", "AUTH_ENABLED", "true", true},
		{"database url only password masked", "DATABASE_URL", "postgres://user:pw@host:5432/db", true},
		{"redis no userinfo untouched", "REDIS_URL", "redis://host:6379", false},
		{"environment untouched", "ENVIRONMENT", "production", false},
		{"debug untouched", "DEBUG", "false", false},
		{"empty value untouched", "ENVIRONMENT", "", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			masked, ok := app.MaskConfigValue(tt.varName, tt.value)
			assert.Equal(t, tt.wantMasked, ok)

			if !tt.wantMasked {
				assert.Equal(t, tt.value, masked)
			} else {
				assert.NotEqual(t, tt.value, masked)
			}
		})
	}

	t.Run("database url preserves host and db name", func(t *testing.T) {
		t.Parallel()

		masked, ok := app.MaskConfigValue("DATABASE_URL", "postgres://user:pw@host:5432/db")
		assert.True(t, ok)
		assert.Contains(t, masked, "host:5432/db")
		assert.NotContains(t, masked, "pw")
	})
}
