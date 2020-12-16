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
	Email    string            `json:"email"`
	AwsRoles map[string]string `json:"https://paaws.lloop.us/aws_roles"`
}

func getCredentials(appName string) (*sts.Credentials, error) {
	tokens, userInfo, err := verifyAuth()
	if err != nil {
		return nil, err
	}
	roleArn, ok := (*userInfo).AwsRoles[appName]
	if !ok {
		tokens, err = refreshTokens()
		if err != nil {
			return nil, err
		}
		userInfo, err := getUserInfoWithAccessToken(tokens.AccessToken)
		if err != nil {
			return nil, err
		}
		roleArn, ok = (*userInfo).AwsRoles[appName]
		if !ok {
			return nil, fmt.Errorf("You don't have access to the app %s", appName)
		}
	}
	sess := session.Must(session.NewSession())
	svc := sts.New(sess)
	duration := int64(900)
	resp, err := svc.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          &roleArn,
		WebIdentityToken: &tokens.IDToken,
		RoleSessionName:  &userInfo.Email,
		DurationSeconds:  &duration,
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == sts.ErrCodeExpiredTokenException {
				tokens, err = refreshTokens()
				if err != nil {
					return nil, err
				}
				resp, err = svc.AssumeRoleWithWebIdentity(&sts.AssumeRoleWithWebIdentityInput{
					RoleArn:          &roleArn,
					WebIdentityToken: &tokens.IDToken,
					RoleSessionName:  &userInfo.Email,
					DurationSeconds:  &duration,
				})
				if err != nil {
					return nil, err
				}
			}
		} else {
			if err != nil {
				return nil, err
			}
		}
	}
	return resp.Credentials, nil
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

func getUserInfo() (*UserInfo, error) {
	token, err := readTokensFromUserCache()
	if err != nil {
		return nil, err
	}
	userInfo, err := getUserInfoWithAccessToken((*token).AccessToken)
	if err != nil {
		return nil, err
	}
	return userInfo, nil
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
			return nil, fmt.Errorf("Unable to retrieve user info. Status code %d", resp.StatusCode)
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
	tokens, err = tokenRequest(oauthTokenURL, reqBody)
	if err != nil {
		return nil, err
	}
	return tokens, nil
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

func AwsSession(appName string) (*session.Session, error) {
	creds, err := getCredentials(appName)
	if err != nil {
		return nil, err
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
				).WithRegion("us-east-1"), // TODO: configure region
			},
		),
	), nil
}

func WhoAmI() (*string, error) {
	userInfo, err := readUserInfoFromUserCache()
	if err != nil {
		return nil, err
	}
	return &userInfo.Email, nil
}

// GetConsoleURL - Returns the sign-in URL
func GetConsoleURL(creds *credentials.Value, destinationURL string) (*string, error) {
	token, err := awsconsoleurl.GetSignInToken(creds)
	return aws.String(fmt.Sprintf(
		"https://signin.aws.amazon.com/federation?Action=login&Destination=%s&SigninToken=%s",
		url.QueryEscape(destinationURL),
		token.Token,
	)), err
}
