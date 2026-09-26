package auth

import (
	"strings"
	"unicode"

	"github.com/apppackio/apppack/version"
	"github.com/aws/aws-sdk-go-v2/config"
)

// appName is the application identifier AWS records for every request the CLI
// makes against the customer's account.
const appName = "apppack-cli"

// userAgentAppID is `apppack-cli#<version>`, or a bare `apppack-cli` for builds
// where the version was not stamped in at link time (the default is the literal
// placeholder `<version>`). The `#` separator matches how the SDK reports its
// own versioned components, e.g. `api/ecs#1.67.2`.
func userAgentAppID() string {
	v := strings.TrimPrefix(version.Version, "v")
	if v == "" || !unicode.IsDigit(rune(v[0])) {
		return appName
	}

	return appName + "#" + v
}

// WithUserAgentAppID sets the SDK's application identifier so AppPack requests
// are attributable in the customer's CloudTrail logs, where they otherwise look
// like any other Go SDK call. It appends `app/apppack-cli#<version>` to the
// User-Agent, alongside the OS and architecture the SDK already reports.
//
// Every config the CLI builds should use this option.
func WithUserAgentAppID() config.LoadOptionsFunc {
	return config.WithAppID(userAgentAppID())
}
