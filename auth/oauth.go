package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const TokenRefreshErr = "unable to refresh auth token"

type DeviceCodeResp struct {
	DeviceCode              string `json:"device_code"`
	ExpiresIn               int    `json:"expires_in"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	Interval                int    `json:"interval"`
	VerificationURIComplete string `json:"verification_uri_complete"`
}

// OauthError handles errors from the Auth0 token endpoint
type OauthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type OauthConfig struct {
	ClientID      string
	Scope         []string
	GrantType     string
	Audience      string
	DeviceCodeURL string
	TokenURL      string
}

func (o *OauthConfig) GetDeviceCode() (*DeviceCodeResp, error) {
	reqBody, err := json.Marshal(map[string]string{
		"client_id": o.ClientID, "scope": strings.Join(o.Scope, " "), "audience": o.Audience,
	})
	if err != nil {
		return nil, err
	}
	logrus.WithFields(logrus.Fields{"url": deviceCodeURL}).Debug("fetching device code")
	resp, err := http.Post(deviceCodeURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		text, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s", text)
	}
	var data DeviceCodeResp
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (o *OauthConfig) GetTokenWithDeviceCode(deviceCode string) (*Tokens, error) {
	reqBody, err := json.Marshal(map[string]string{
		"grant_type": o.GrantType, "device_code": deviceCode, "client_id": o.ClientID,
	})
	if err != nil {
		return nil, err
	}
	return o.TokenRequest(reqBody)
}

func (o *OauthConfig) RefreshTokens(tokens *Tokens) (*Tokens, error) {
	reqBody, err := json.Marshal(map[string]string{
		"grant_type": "refresh_token", "refresh_token": tokens.RefreshToken, "client_id": o.ClientID,
	})
	if err != nil {
		return nil, err
	}
	return o.TokenRequest(reqBody)
}

func (o *OauthConfig) TokenRequest(jsonData []byte) (*Tokens, error) {
	logrus.WithFields(logrus.Fields{"url": o.TokenURL}).Debug("fetching token")
	resp, err := http.Post(o.TokenURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(string(contents))
	}
	var tokens Tokens
	if err = json.Unmarshal(contents, &tokens); err != nil {
		return nil, err
	}
	return &tokens, nil
}

func (o *OauthConfig) PollForToken(code *DeviceCodeResp) (*Tokens, error) {
	checkInterval := time.Duration(code.Interval) * time.Second
	expiresAt := time.Now().Add(time.Duration(code.ExpiresIn) * time.Second)

	for {
		time.Sleep(checkInterval)

		token, err := o.GetTokenWithDeviceCode(code.DeviceCode)
		if err == nil {
			return token, nil
		}
		// "authorization_pending" is the only error that we accept
		var authError OauthError
		if json.Unmarshal([]byte(err.Error()), &authError) != nil {
			return nil, err
		}
		if authError.Error != "authorization_pending" {
			return nil, err
		}

		if time.Now().After(expiresAt) {
			return nil, fmt.Errorf("device code expired -- try logging in again")
		}
	}
}

func readCacheFile(name string) ([]byte, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	filename := filepath.Join(dir, cachePrefix, name)
	logrus.WithFields(logrus.Fields{"filename": filename}).Debug("reading from user cache")
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func TokensFromCache() (*Tokens, error) {
	contents, err := readCacheFile("tokens")
	if err != nil {
		return nil, err
	}
	var t Tokens
	if err = json.Unmarshal(contents, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func UserInfoFromCache() (*UserInfo, error) {
	contents, err := readCacheFile("user")
	if err != nil {
		return nil, err
	}
	var u UserInfo
	if err = json.Unmarshal(contents, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

var Oauth = OauthConfig{
	ClientID:      clientID,
	Scope:         []string{"openid", "profile", "email", "offline_access"},
	GrantType:     grantType,
	Audience:      audience,
	DeviceCodeURL: deviceCodeURL,
	TokenURL:      oauthTokenURL,
}

// GetTokens gets the cached tokens from the filesystem and refreshes them if necessary
func GetTokens() (*Tokens, error) {
	tokens, err := TokensFromCache()
	if err != nil {
		return nil, err
	}
	expired, err := tokens.IsExpired()
	if err != nil {
		return nil, err
	}
	if !*expired {
		return tokens, nil
	}
	tokens, err = Oauth.RefreshTokens(tokens)
	if err != nil {
		return nil, fmt.Errorf("%s: %s", TokenRefreshErr, err)
	}
	if err = tokens.WriteToCache(); err != nil {
		return nil, err
	}
	return tokens, nil
}
