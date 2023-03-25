package auth

import (
	"fmt"
	"net/url"

	"github.com/apppackio/apppack/state"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	awsconsoleurl "github.com/jkueh/go-aws-console-url"
	"github.com/sirupsen/logrus"
)

const (
	deviceCodeURL = "https://auth.apppack.io/oauth/device/code"
	oauthTokenURL = "https://auth.apppack.io/oauth/token"
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

func AppAWSSession(appName string, sessionDuration int) (*session.Session, *AppRole, error) {
	tokens, err := GetTokens()
	if err != nil {
		return nil, nil, err
	}
	appRole, err := tokens.GetAppRole(appName)
	if err != nil {
		return nil, nil, err
	}
	creds, err := tokens.GetCredentials(appRole, sessionDuration)
	if err != nil {
		return nil, nil, err
	}
	logrus.WithFields(logrus.Fields{"access key": *creds.AccessKeyId}).Debug("creating AWS session")

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

func AdminAWSSession(idOrAlias string, sessionDuration int, region string) (*session.Session, *AdminRole, error) {
	tokens, err := GetTokens()
	if err != nil {
		return nil, nil, err
	}
	adminRole, err := tokens.GetAdminRole(idOrAlias)
	if err != nil {
		return nil, nil, err
	}
	creds, err := tokens.GetCredentials(adminRole, sessionDuration)
	if err != nil {
		return nil, nil, err
	}
	logrus.WithFields(logrus.Fields{"access_key": *creds.AccessKeyId}).Debug("creating AWS session")
	if region == "" {
		region = adminRole.Region
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
				).WithRegion(region),
			},
		),
	), adminRole, nil
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
