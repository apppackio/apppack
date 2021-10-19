package auth

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/sirupsen/logrus"
	"gopkg.in/square/go-jose.v2/jwt"
)

func writeToUserCache(name string, data []byte) error {
	dir, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, cachePrefix)
	err = os.Mkdir(path, os.FileMode(0700))
	if err != nil {
		if !os.IsExist(err) {
			return err
		}
	}
	filename := filepath.Join(path, name)
	logrus.WithFields(logrus.Fields{"filename": filename}).Debug("writing to user cache")
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	err = file.Chmod(os.FileMode(0600))
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	return err
}

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
	req, err := http.NewRequest("GET", userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t.AccessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		if err != nil {
			return nil, fmt.Errorf("unable to retrieve user info. Status code %d", resp.StatusCode)
		}
	}
	if err != nil {
		return nil, err
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
	return writeToUserCache("tokens", data)
}

func (t *Tokens) IsExpired() (*bool, error) {
	parsedToken, err := jwt.ParseSigned(t.AccessToken)
	if err == nil {
		out := jwt.Claims{}
		// AWS will verify the token
		// this just checks the expiration data to see if a refresh should happen first
		err = parsedToken.UnsafeClaimsWithoutVerification(&out)
		if err == nil && out.Expiry.Time().After(time.Now().Add(2*time.Second)) {
			logrus.WithFields(logrus.Fields{"expiration_date": out.Expiry.Time().Local().String()}).Debug("token has not expired")
			b := false
			return &b, nil
		} else {
			logrus.WithFields(logrus.Fields{"expiration_date": out.Expiry.Time().Local().String()}).Debug("token expired")
			b := true
			return &b, nil
		}
	} else {
		logrus.WithFields(logrus.Fields{"error": err}).Debug("unable to parse token")
		return nil, fmt.Errorf("unable to parse token")
	}
}

func (t *Tokens) GetAppList() ([]*AppRole, error) {
	logrus.WithFields(logrus.Fields{"url": appListURL}).Debug("fetching app list")
	req, err := http.NewRequest("GET", appListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t.IDToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
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
	return nil, fmt.Errorf("app not found in user info")
}

func (t *Tokens) GetAdminList() ([]*AdminRole, error) {
	logrus.WithFields(logrus.Fields{"url": adminListURL}).Debug("fetching admin list")
	req, err := http.NewRequest("GET", adminListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", t.IDToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
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
		return nil, fmt.Errorf("no account ID or alias specified")
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
		return nil, fmt.Errorf("account %s not found", idOrAlias)
	}
	return found, nil
}

func (t *Tokens) GetCredentials(role Role) (*sts.Credentials, error) {
	userInfo, err := UserInfoFromCache()
	if err != nil {
		return nil, err
	}
	sess := session.Must(session.NewSession())
	svc := sts.New(sess)
	duration := int64(900)
	roleARN := role.GetRoleARN()
	logrus.WithFields(logrus.Fields{"role": roleARN}).Debug("assuming role")
	resp, err := svc.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          &roleARN,
		WebIdentityToken: &t.IDToken,
		RoleSessionName:  &userInfo.Email,
		DurationSeconds:  &duration,
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
	return writeToUserCache("user", data)
}
