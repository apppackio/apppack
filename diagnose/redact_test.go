package diagnose_test

import (
	"testing"

	"github.com/apppackio/apppack/diagnose"
	"github.com/stretchr/testify/assert"
)

func TestRedact(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		in   string
		want string
	}{
		"env assignment": {
			`SECRET_KEY=hunter2trustno1`,
			`SECRET_KEY=[REDACTED]`,
		},
		"colon separated": {
			`DJANGO_SECRET_KEY: abc123xyz`,
			`DJANGO_SECRET_KEY: [REDACTED]`,
		},
		"lowercase key": {
			`api_key=deadbeef`,
			`api_key=[REDACTED]`,
		},
		"connection string": {
			`postgres://appuser:s3cr3tpw@db.internal:5432/mydb`,
			`postgres://appuser:[REDACTED]@db.internal:5432/mydb`,
		},
		"aws access key": {
			`using AKIAIOSFODNN7EXAMPLE for auth`,
			`using [REDACTED] for auth`,
		},
		"jwt": {
			`Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk`,
			`Bearer [REDACTED]`,
		},
		"json key-value": {
			`"SECRET_KEY": "abc123xyz"`,
			`"SECRET_KEY": [REDACTED]`,
		},
		"quoted value with spaces": {
			`SECRET_KEY = "hello world secret"`,
			`SECRET_KEY = [REDACTED]`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, diagnose.Redact(tc.in))
		})
	}
}

// Near-misses that must NOT be mangled. Over-redaction destroys the diagnosis,
// which is the whole product.
func TestRedactLeavesInnocentTextAlone(t *testing.T) {
	t.Parallel()

	for name, in := range map[string]string{
		"key name with no value":           `SECRET_KEY is not set`,
		"missing env var error":            `KeyError: 'DATABASE_PASSWORD'`,
		"prose":                            `the token could not be validated`,
		"url without credentials":          `postgres://db.internal:5432/mydb`,
		"ordinary assignment":              `PORT=8080`,
		"module path":                      `django.core.exceptions.ImproperlyConfigured`,
		"error with InvalidTokenError":     `InvalidTokenError: token has expired`,
		"error with TokenError":            `TokenError: invalid or expired signature`,
		"error with AuthTokenMissingError": `AuthTokenMissingError: no token provided in request headers`,
		"tokenized identifier":             `tokenized_value=metadata_only`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, in, diagnose.Redact(in))
		})
	}
}
