package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// urlCredentials is the data structure that is embedded into the URL in the Session query parameter.
type urlCredentials struct {
	SessionID    string `json:"sessionId"`
	SessionKey   string `json:"sessionKey"`
	SessionToken string `json:"sessionToken"`
}

// signInToken is the payload that's returned from the token request endpoint.
type signInToken struct {
	Token string `json:"SigninToken"`
}

// getSignInToken sends a request to retrieve the sign-in token from AWS federation endpoint.
func getSignInToken(ctx context.Context, creds aws.Credentials) (*signInToken, error) {
	urlCreds := urlCredentials{
		SessionID:    creds.AccessKeyID,
		SessionKey:   creds.SecretAccessKey,
		SessionToken: creds.SessionToken,
	}

	byteArr, err := json.Marshal(&urlCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal credentials: %w", err)
	}

	tokenRequestEndpoint := fmt.Sprintf(
		"https://signin.aws.amazon.com/federation?Action=getSigninToken&Session=%s",
		url.QueryEscape(string(byteArr)),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenRequestEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build token request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request signin token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var token signInToken
	if err := json.Unmarshal(body, &token); err != nil {
		return nil, fmt.Errorf("failed to unmarshal signin token: %w", err)
	}

	return &token, nil
}
