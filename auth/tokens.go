package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/apppackio/apppack/state"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/sirupsen/logrus"
)

type Tokens struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

func (t *Tokens) GetUserInfo() (*UserInfo, error) {
	logrus.WithFields(logrus.Fields{"url": userInfoURL}).Debug("fetching user info")

	req, err := http.NewRequest(http.MethodGet, userInfoURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+t.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to retrieve user info. Status code %d", resp.StatusCode)
	}

	var userInfo UserInfo

	if err = json.Unmarshal(contents, &userInfo); err != nil {
		return nil, err
	}

	return &userInfo, nil
}

func (t *Tokens) WriteToCache() error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}

	return state.WriteToCache("tokens", data)
}

func (t *Tokens) IsExpired() (*bool, error) {
	// Allow common JWT signature algorithms (RS256, HS256, ES256)
	// We're not verifying the signature (using UnsafeClaimsWithoutVerification),
	// just parsing to check expiration time
	allowedAlgorithms := []jose.SignatureAlgorithm{
		jose.RS256, // RSA with SHA-256 (most common for Auth0)
		jose.HS256, // HMAC with SHA-256
		jose.ES256, // ECDSA with P-256 and SHA-256
	}
	parsedToken, err := jwt.ParseSigned(t.AccessToken, allowedAlgorithms)
	if err == nil {
		out := jwt.Claims{}
		// AWS will verify the token
		// this just checks the expiration data to see if a refresh should happen first
		err = parsedToken.UnsafeClaimsWithoutVerification(&out)
		if err == nil && out.Expiry.Time().After(time.Now().Add(2*time.Second)) {
			logrus.WithFields(logrus.Fields{"expiration_date": out.Expiry.Time().Local().String()}).Debug("token has not expired")

			b := false

			return &b, nil
		}

		logrus.WithFields(logrus.Fields{"expiration_date": out.Expiry.Time().Local().String()}).Debug("token expired")

		b := true

		return &b, nil
	}

	logrus.WithFields(logrus.Fields{"error": err}).Debug("unable to parse token")

	return nil, errors.New("unable to parse token")
}

func (t *Tokens) GetAppList() ([]*AppRole, error) {
	logrus.WithFields(logrus.Fields{"url": appListURL}).Debug("fetching app list")

	req, err := http.NewRequest(http.MethodGet, appListURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+t.IDToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to retrieve app list. Status code %d", resp.StatusCode)
	}

	var appList []*AppRole

	if err = json.Unmarshal(contents, &appList); err != nil {
		return nil, err
	}

	return appList, nil
}

func (t *Tokens) GetAppRole(name string) (*AppRole, error) {
	appList, err := t.GetAppList()
	if err != nil {
		return nil, err
	}

	for _, appRole := range appList {
		if appRole.AppName == name {
			return appRole, nil
		}
	}

	return nil, errors.New("app not found in user info")
}

func (t *Tokens) GetAdminList() ([]*AdminRole, error) {
	logrus.WithFields(logrus.Fields{"url": adminListURL}).Debug("fetching admin list")

	req, err := http.NewRequest(http.MethodGet, adminListURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", "Bearer "+t.IDToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	contents, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to retrieve account list. Status code %d", resp.StatusCode)
	}

	var adminList []*AdminRole

	if err = json.Unmarshal(contents, &adminList); err != nil {
		return nil, err
	}

	return adminList, nil
}

func (t *Tokens) GetAdminRole(idOrAlias string) (*AdminRole, error) {
	adminRoles, err := t.GetAdminList()
	if err != nil {
		return nil, err
	}
	// allow users to skip specifying a role if there is only one
	if idOrAlias == "" {
		if len(adminRoles) == 1 {
			return adminRoles[0], nil
		}

		return nil, errors.New("no account ID or alias specified")
	}

	var found *AdminRole

	for _, a := range adminRoles {
		if a.AccountID == idOrAlias || a.AccountAlias == idOrAlias {
			if found != nil {
				return nil, fmt.Errorf("account alias %s is not unique", idOrAlias)
			}

			found = a
		}
	}

	if found == nil {
		// user does not have admin access to the account
		return nil, fmt.Errorf("administrator privileges required (account %s)", idOrAlias)
	}

	return found, nil
}

func (t *Tokens) GetCredentials(role Role, duration int) (*types.Credentials, error) {
	userInfo, err := UserInfoFromCache()
	if err != nil {
		return nil, err
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"), // STS is a global service, use us-east-1 as default
	)
	if err != nil {
		return nil, err
	}

	svc := sts.NewFromConfig(cfg)
	roleARN := role.GetRoleARN()
	logrus.WithFields(logrus.Fields{"role": roleARN}).Debug("assuming role")

	durationSeconds := int32(duration)
	resp, err := svc.AssumeRoleWithWebIdentity(context.Background(), &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          &roleARN,
		WebIdentityToken: &t.IDToken,
		RoleSessionName:  &userInfo.Email,
		DurationSeconds:  &durationSeconds,
	})
	if err != nil {
		return nil, err
	}

	return resp.Credentials, nil
}

type UserInfo struct {
	Email         string `json:"email"`
	Sub           string `json:"sub"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Nickname      string `json:"nickname"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
	UpdatedAt     string `json:"updated_at"`
	EmailVerified bool   `json:"email_verified"`
}

func (u *UserInfo) WriteToCache() error {
	data, err := json.Marshal(u)
	if err != nil {
		return err
	}

	return state.WriteToCache("user", data)
}
