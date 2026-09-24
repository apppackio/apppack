package diagnose

import "regexp"

// redaction is a pattern and the replacement applied to text leaving this
// package.
type redaction struct {
	re          *regexp.Regexp
	replacement string
}

// redactions scrub credential-shaped strings from the model's response.
//
// This is deliberately applied to OUTPUT, not input. Logs are sent to Bedrock
// unmodified (see the spec): Bedrock does not retain them, and they already
// live in CloudWatch in the same account. The realistic leak is a diagnosis
// quoting a secret back and the user pasting it into a ticket.
//
// This is best-effort and the documentation must say so. It cannot catch a
// secret that does not look like one.
var redactions = []redaction{
	// KEY=value or KEY: value where the key name suggests a credential.
	// Requires an actual value, so "SECRET_KEY is not set" is left alone.
	// Keywords must be whole identifier segments, enforced by:
	// - Either preceded by underscore (e.g., api_KEY) or standalone (e.g., SECRET_KEY, TOKEN)
	// - Optionally followed by underscore and suffix (e.g., db_PASSWORD_SALT)
	// This prevents matching keywords within camelCase (e.g., InvalidTokenError).
	// Keys may be quoted (e.g., "SECRET_KEY" in JSON), and values may be quoted with spaces.
	{
		regexp.MustCompile(`(["'])?([A-Za-z0-9_]*_(?i:SECRET|PASSWORD|PASSWD|CREDENTIAL|API_?KEY|ACCESS_?KEY|TOKEN)(?:_[A-Za-z0-9_]*)?|(?i:SECRET|PASSWORD|PASSWD|CREDENTIAL|API_?KEY|ACCESS_?KEY|TOKEN)(?:_[A-Za-z0-9_]*)?)(["'])?(\s*[=:]\s*)(?:"[^"]*"|'[^']*'|[^\s"']+)`),
		`${1}${2}${3}${4}[REDACTED]`,
	},
	// Connection-string userinfo: proto://user:password@host
	{
		regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://[^:@/\s]+):([^@/\s]+)@`),
		`${1}:[REDACTED]@`,
	},
	// AWS access key IDs.
	{
		regexp.MustCompile(`\b(?:AKIA|ASIA|AROA|AIDA)[A-Z0-9]{16}\b`),
		`[REDACTED]`,
	},
	// JWTs.
	{
		regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{5,}\.[A-Za-z0-9_\-]{5,}\.[A-Za-z0-9_\-]{5,}\b`),
		`[REDACTED]`,
	},
}

// Redact removes credential-shaped strings from text before it is printed.
func Redact(s string) string {
	for _, r := range redactions {
		s = r.re.ReplaceAllString(s, r.replacement)
	}

	return s
}
