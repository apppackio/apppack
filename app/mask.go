package app

import (
	"net/url"
	"strings"
)

// sensitiveNameSubstrings is a case-insensitive deny-list of substrings that,
// when found in a config variable name, mark its value as likely sensitive.
//
// This is a best-effort heuristic, not a security control. It will miss
// things (e.g. STRIPE_SK, FOO_PW) and over-mask others (e.g. AUTH_ENABLED).
var sensitiveNameSubstrings = []string{
	"SECRET",
	"PASSWORD",
	"PASSWD",
	"TOKEN",
	"API_KEY",
	"APIKEY",
	"PRIVATE_KEY",
	"CREDENTIAL",
	"AUTH",
	"SALT",
	"SIGNATURE",
	"ACCESS_KEY",
	"SESSION_KEY",
	"DSN",
}

const (
	// maskChar is used to obscure sensitive values.
	maskChar = "•"
	// maxMaskRunLength caps the number of mask characters printed so the
	// mask never reveals the exact length of the original value.
	maxMaskRunLength = 12
	// shortValueMaskLength is the fixed-width mask printed for values that
	// are too short to safely reveal partial characters.
	shortValueMaskLength = 8
	// minMaskableLength is the value length above which we show the first
	// and last 2 characters instead of a fully opaque mask.
	minMaskableLength = 8
	// partialRevealLength is the number of characters shown from each end
	// of a long value.
	partialRevealLength = 2
)

// IsSensitiveName reports whether name looks like it holds a secret value,
// based on a case-insensitive substring match against a deny-list.
func IsSensitiveName(name string) bool {
	upper := strings.ToUpper(name)
	for _, substr := range sensitiveNameSubstrings {
		if strings.Contains(upper, substr) {
			return true
		}
	}

	return false
}

// maskRun returns a run of mask characters, capped at maxMaskRunLength so
// the original value's length is never revealed.
func maskRun(length int) string {
	if length > maxMaskRunLength {
		length = maxMaskRunLength
	}

	return strings.Repeat(maskChar, length)
}

// MaskValue obscures value, retaining the first and last 2 characters for
// values longer than 8 characters. Values of 8 characters or fewer are
// replaced with a fixed-width mask so their length is never leaked.
func MaskValue(value string) string {
	if len(value) <= minMaskableLength {
		return maskRun(shortValueMaskLength)
	}

	prefix := value[:partialRevealLength]
	suffix := value[len(value)-partialRevealLength:]

	return prefix + maskRun(len(value)-2*partialRevealLength) + suffix
}

// MaskURLPassword parses value as a URL and, if it has a scheme and
// userinfo with a password set, returns a copy with only the password
// component masked. The second return value reports whether masking was
// applied.
//
// The replacement is done via string surgery on the original text (rather
// than reassembling via url.URL.String()) so the scheme, username, host,
// port and path are left byte-for-byte intact and the bullet mask
// characters aren't percent-encoded.
func MaskURLPassword(value string) (string, bool) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" || u.User == nil {
		return value, false
	}

	password, ok := u.User.Password()
	if !ok || password == "" {
		return value, false
	}

	schemeSepIdx := strings.Index(value, "://")
	if schemeSepIdx == -1 {
		// no authority component to do targeted surgery on; mask the
		// whole value as a fallback.
		return MaskValue(value), true
	}

	authorityStart := schemeSepIdx + len("://")

	// the authority component ends at the first '/', '?' or '#' after the
	// scheme separator, or at the end of the string.
	authorityEnd := len(value)

	for i := authorityStart; i < len(value); i++ {
		if c := value[i]; c == '/' || c == '?' || c == '#' {
			authorityEnd = i

			break
		}
	}

	relAt := strings.LastIndex(value[authorityStart:authorityEnd], "@")
	if relAt == -1 {
		return value, false
	}

	atIdx := authorityStart + relAt

	relColon := strings.LastIndex(value[authorityStart:atIdx], ":")
	if relColon == -1 {
		return value, false
	}

	colonIdx := authorityStart + relColon

	masked := value[:colonIdx+1] + maskRun(shortValueMaskLength) + value[atIdx:]

	return masked, true
}

// MaskConfigValue applies best-effort masking to value based on its config
// variable name. It first tries to mask a password embedded in a URL (e.g.
// DATABASE_URL, REDIS_URL), preserving the rest of the URL. Failing that,
// it falls back to masking the entire value if name looks sensitive. The
// second return value reports whether masking was applied.
func MaskConfigValue(name, value string) (string, bool) {
	if masked, ok := MaskURLPassword(value); ok {
		return masked, true
	}

	if IsSensitiveName(name) {
		return MaskValue(value), true
	}

	return value, false
}
