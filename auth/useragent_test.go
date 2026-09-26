package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/apppackio/apppack/version"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func TestUserAgentAppID(t *testing.T) {
	for _, tc := range []struct {
		name    string
		version string
		want    string
	}{
		{"released version", "4.8.3", "apppack-cli#4.8.3"},
		{"v-prefixed version", "v4.8.3", "apppack-cli#4.8.3"},
		{"prerelease version", "4.8.3-next", "apppack-cli#4.8.3-next"},
		{"unstamped build", "<version>", "apppack-cli"},
		{"empty version", "", "apppack-cli"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := version.Version
			version.Version = tc.version
			defer func() { version.Version = original }()

			if got := userAgentAppID(); got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// captureHTTPClient records the request it is handed and fails it, so a client
// call never leaves the test.
type captureHTTPClient struct {
	request *http.Request
}

func (c *captureHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.request = req

	return nil, errors.New("request not sent")
}

func TestWithUserAgentAppIDSetsHeader(t *testing.T) {
	original := version.Version
	version.Version = "4.8.3"
	defer func() { version.Version = original }()

	httpClient := &captureHTTPClient{}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		WithUserAgentAppID(),
		ignoreSharedConfigFiles(),
		config.WithRegion("us-east-1"),
		config.WithRetryMaxAttempts(1),
		config.WithHTTPClient(httpClient),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("id", "secret", "token")),
	)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	// the stub HTTP client always fails the call; we only want the request it was given
	_, _ = sts.NewFromConfig(cfg).GetCallerIdentity(context.Background(), &sts.GetCallerIdentityInput{})

	if httpClient.request == nil {
		t.Fatal("no request was made")
	}

	userAgent := httpClient.request.Header.Get("User-Agent")
	if !strings.Contains(userAgent, "app/apppack-cli#4.8.3") {
		t.Errorf("expected app/apppack-cli#4.8.3 in User-Agent, got: %s", userAgent)
	}
}
