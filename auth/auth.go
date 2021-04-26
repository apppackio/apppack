package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
	awsconsoleurl "github.com/jkueh/go-aws-console-url"
)

const (
	auth0AppURL   = "https://auth.apppack.io"
	deviceCodeURL = "https://auth.apppack.io/oauth/device/code"
	oauthTokenURL = "https://auth.apppack.io/oauth/token"
	userInfoURL   = "https://auth.apppack.io/userinfo"
	appListURL    = "https://api.apppack.io/apps"
	clientID      = "x15zAd2hgdbugNWSZz2mP2k5jcZfNFk3"
	scope         = "openid profile email offline_access"
	audience      = "https://paaws.lloop.us"
	grantType     = "urn:ietf:params:oauth:grant-type:device_code"
	cachePrefix   = "io.apppack"
)

type Tokens struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type DeviceCodeResp struct {
	DeviceCode              string `json:"device_code"`
	ExpiresIn               int    `json:"expires_in"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	Interval                int    `json:"interval"`
	VerificationURIComplete string `json:"verification_uri_complete"`
}

type UserInfo struct {
	Email string    `json:"email"`
	Apps  []AppRole `json:"https://apppack.io/apps"`
}

type AppRole struct {
	RoleARN   string `json:"role_arn"`
	AccountID string `json:"account_id"`
	AppName   string `json:"name"`
	Region    string `json:"region"`
	Pipeline  bool   `json:"pipeline"`
}

func getAppRole(IDToken string, name string) (*AppRole, error) {
	appList, err := getAppListWithIDToken(IDToken)
	if err != nil {
		tokens, err := refreshTokens()
		if err != nil {
			return nil, err
		}
		appList, err = getAppListWithIDToken(tokens.IDToken)
		if err != nil {
			return nil, err
		}
	}
	for _, appRole := range appList {
		if appRole.AppName == name {
			return appRole, nil
		}
	}
	return nil, fmt.Errorf("app not found in user info")
}

func getCredentials(appName string) (*sts.Credentials, *AppRole, error) {
	tokens, userInfo, err := verifyAuth()
	if err != nil {
		return nil, nil, err
	}
	appRole, err := getAppRole(tokens.IDToken, appName)
	if err != nil {
		return nil, nil, err
	}
	sess := session.Must(session.NewSession())
	svc := sts.New(sess)
	duration := int64(900)
	resp, err := svc.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          &appRole.RoleARN,
		WebIdentityToken: &tokens.IDToken,
		RoleSessionName:  &userInfo.Email,
		DurationSeconds:  &duration,
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == sts.ErrCodeExpiredTokenException {
				tokens, err = refreshTokens()
				if err != nil {
					return nil, nil, err
				}
				resp, err = svc.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
					RoleArn:          &appRole.RoleARN,
					WebIdentityToken: &tokens.IDToken,
					RoleSessionName:  &userInfo.Email,
					DurationSeconds:  &duration,
				})
				if err != nil {
					return nil, nil, err
				}
				return resp.Credentials, appRole, nil
			}
			return nil, nil, err
		}
		return nil, nil, err
	}
	return resp.Credentials, appRole, nil
}

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
	file, err := os.Create(filepath.Join(path, name))
	if err != nil {
		return err
	}
	err = file.Chmod(os.FileMode(0600))
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(data)
	if err != nil {
		return err
	}
	return nil
}

func cacheFile(name string) (*os.File, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(dir, cachePrefix, name))
	if err != nil {
		return nil, err
	}
	return file, nil
}

func readTokensFromUserCache() (*Tokens, error) {
	file, err := cacheFile("tokens")
	if err != nil {
		return nil, err
	}
	var obj Tokens
	err = json.NewDecoder(file).Decode(&obj)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return &obj, nil
}

func readUserInfoFromUserCache() (*UserInfo, error) {
	file, err := cacheFile("user")
	if err != nil {
		return nil, err
	}
	var obj UserInfo
	err = json.NewDecoder(file).Decode(&obj)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return &obj, nil
}

func getUserInfoWithAccessToken(accessToken string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		if err != nil {
			return nil, fmt.Errorf("unable to retrieve user info. Status code %d", resp.StatusCode)
		}
	}
	err = writeToUserCache("user", contents)
	if err != nil {
		return nil, err
	}
	var userInfo UserInfo

	if err = json.Unmarshal(contents, &userInfo); err != nil {
		return nil, err
	}
	return &userInfo, nil
}

func getAppListWithIDToken(IDToken string) ([]*AppRole, error) {
	req, err := http.NewRequest("GET", appListURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", IDToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unable to retrieve app list. Status code %d", resp.StatusCode)
	}
	//err = writeToUserCache("apps", contents)
	//if err != nil {
	//	return err
	//}
	var appList []*AppRole

	if err = json.Unmarshal(contents, &appList); err != nil {
		return nil, err
	}
	return appList, nil
}

func verifyAuth() (*Tokens, *UserInfo, error) {
	tokens, err := readTokensFromUserCache()
	if err != nil {
		return nil, nil, err
	}
	userInfo, err := readUserInfoFromUserCache()
	if err != nil {
		return nil, nil, err
	}
	return tokens, userInfo, err
}

func refreshTokens() (*Tokens, error) {
	tokens, err := readTokensFromUserCache()
	if err != nil {
		return nil, err
	}
	reqBody, err := json.Marshal(map[string]string{
		"grant_type": "refresh_token", "refresh_token": (*tokens).RefreshToken, "client_id": clientID,
	})
	if err != nil {
		return nil, err
	}
	return tokenRequest(oauthTokenURL, reqBody)
}

// LoginInit start login process with Auth0
func LoginInit() (*DeviceCodeResp, error) {
	reqBody, err := json.Marshal(map[string]string{
		"client_id": clientID, "scope": scope, "audience": audience,
	})
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(deviceCodeURL, "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		text, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s", text)
	}
	var data DeviceCodeResp
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func tokenRequest(url string, jsonData []byte) (*Tokens, error) {
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	contents, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New(string(contents))
	}
	writeToUserCache("tokens", contents)
	var tokens Tokens
	if err = json.Unmarshal(contents, &tokens); err != nil {
		log.Fatalln(err)
	}

	return &tokens, nil
}

func LoginComplete(deviceCode string) (*UserInfo, error) {
	reqBody, err := json.Marshal(map[string]string{
		"grant_type": grantType, "device_code": deviceCode, "client_id": clientID,
	})
	if err != nil {
		return nil, err
	}
	tokens, err := tokenRequest(oauthTokenURL, reqBody)
	if err != nil {
		return nil, err
	}

	userInfo, err := getUserInfoWithAccessToken((*tokens).AccessToken)
	if err != nil {
		return nil, err
	}
	return userInfo, nil
}

func Logout() error {
	dir, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, cachePrefix)
	err = os.RemoveAll(path)
	if err != nil {
		return err
	}
	return nil
}

func AwsSession(appName string) (*session.Session, *AppRole, error) {
	creds, appRole, err := getCredentials(appName)
	if err != nil {
		return nil, nil, err
	}
	return session.Must(
		session.NewSessionWithOptions(
			session.Options{
				Config: *aws.NewConfig().WithCredentials(
					credentials.NewStaticCredentials(
						*creds.AccessKeyId,
						*creds.SecretAccessKey,
						*creds.SessionToken,
					),
				).WithRegion(appRole.Region),
			},
		),
	), appRole, nil
}

func AppList() ([]*AppRole, error) {
	tokens, _, err := verifyAuth()
	if err != nil {
		return nil, err
	}
	appList, err := getAppListWithIDToken(tokens.IDToken)
	if err != nil {
		tokens, err := refreshTokens()
		if err != nil {
			return nil, err
		}
		appList, err = getAppListWithIDToken(tokens.IDToken)
		if err != nil {
			return nil, err
		}
	}
	return appList, err
}

func WhoAmI() (*string, error) {
	userInfo, err := readUserInfoFromUserCache()
	if err != nil {
		return nil, err
	}
	return &userInfo.Email, nil
}

// GetConsoleURL - Returns the sign-in URL
func GetConsoleURL(sess *session.Session, destinationURL string) (*string, error) {
	creds, err := sess.Config.Credentials.Get()
	if err != nil {
		return nil, err
	}
	if creds.SessionToken == "" {
		return nil, fmt.Errorf("can't generate a signin token without a session token")
	}
	token, err := awsconsoleurl.GetSignInToken(&creds)
	return aws.String(fmt.Sprintf(
		"https://signin.aws.amazon.com/federation?Action=login&Destination=%s&SigninToken=%s",
		url.QueryEscape(destinationURL),
		token.Token,
	)), err
}
