package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/apppackio/apppack/state"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/sirupsen/logrus"
)

const (
	deviceCodeURL = "https://auth.apppack.io/oauth/device/code"
	oauthTokenURL = "https://auth.apppack.io/oauth/token" // #nosec G101 -- URL path, not a credential
	userInfoURL   = "https://auth.apppack.io/userinfo"
	appListURL    = "https://api.apppack.io/apps"
	adminListURL  = "https://api.apppack.io/accounts"
	clientID      = "x15zAd2hgdbugNWSZz2mP2k5jcZfNFk3"
	audience      = "https://paaws.lloop.us"
	grantType     = "urn:ietf:params:oauth:grant-type:device_code"
)

func Logout() error {
	return state.ClearCache()
}

func AppAWSSession(appName string, sessionDuration int) (aws.Config, *AppRole, error) {
	tokens, err := GetTokens()
	if err != nil {
		return aws.Config{}, nil, err
	}

	appRole, err := tokens.GetAppRole(appName)
	if err != nil {
		return aws.Config{}, nil, err
	}

	creds, err := tokens.GetCredentials(appRole, sessionDuration)
	if err != nil {
		return aws.Config{}, nil, err
	}

	logrus.WithFields(logrus.Fields{"access key": *creds.AccessKeyId}).Debug("creating AWS config")

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			*creds.AccessKeyId,
			*creds.SecretAccessKey,
			*creds.SessionToken,
		)),
		config.WithRegion(appRole.Region),
	)
	if err != nil {
		return aws.Config{}, nil, err
	}

	return cfg, appRole, nil
}

func AdminAWSSession(idOrAlias string, sessionDuration int, region string) (aws.Config, *AdminRole, error) {
	tokens, err := GetTokens()
	if err != nil {
		return aws.Config{}, nil, err
	}

	adminRole, err := tokens.GetAdminRole(idOrAlias)
	if err != nil {
		return aws.Config{}, nil, err
	}

	creds, err := tokens.GetCredentials(adminRole, sessionDuration)
	if err != nil {
		return aws.Config{}, nil, err
	}

	logrus.WithFields(logrus.Fields{"access_key": *creds.AccessKeyId}).Debug("creating AWS config")

	if region == "" {
		region = adminRole.Region
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			*creds.AccessKeyId,
			*creds.SecretAccessKey,
			*creds.SessionToken,
		)),
		config.WithRegion(region),
	)
	if err != nil {
		return aws.Config{}, nil, err
	}

	return cfg, adminRole, nil
}

func AppList() ([]*AppRole, error) {
	tokens, err := GetTokens()
	if err != nil {
		return nil, err
	}

	return tokens.GetAppList()
}

func AdminList() ([]*AdminRole, error) {
	tokens, err := GetTokens()
	if err != nil {
		return nil, err
	}

	return tokens.GetAdminList()
}

func WhoAmI() (*string, error) {
	userInfo, err := UserInfoFromCache()
	if err != nil {
		return nil, err
	}

	return &userInfo.Email, nil
}

// GetConsoleURL - Returns the sign-in URL
func GetConsoleURL(cfg aws.Config, destinationURL string) (*string, error) {
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, err
	}

	if creds.SessionToken == "" {
		return nil, errors.New("can't generate a signin token without a session token")
	}

	token, err := getSignInToken(context.Background(), creds)
	if err != nil {
		return nil, err
	}

	consoleURL := fmt.Sprintf(
		"https://signin.aws.amazon.com/federation?Action=login&Destination=%s&SigninToken=%s",
		url.QueryEscape(destinationURL),
		token.Token,
	)

	return &consoleURL, nil
}
