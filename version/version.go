package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	vers "github.com/hashicorp/go-version"
	"github.com/logrusorgru/aurora"
)

var BuildDate = time.Now().String()
var Version = "<version>"
var Commit = "<commit>"
var Environment = "development"

type Release struct {
	Name      string
	CreatedAt string `json:"created_at"`
}

func GetLatestRelease() (Release, error) {
	resp, err := http.Get("https://api.github.com/repos/apppackio/apppack/releases/latest")
	if err != nil {
		return Release{}, err
	}
	var release Release
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		fmt.Println(fmt.Sprintf("%v", err))
		return release, err
	}
	return release, nil
}

func IsUpToDate(latest *Release) bool {
	if Environment != "production" {
		return true
	}

	if latest == nil {
		latestObj, err := GetLatestRelease()
		if err != nil {
			return false
		}
		latest = &latestObj
	}

	newVersion, err_1 := vers.NewVersion(latest.Name)
	if err_1 != nil {
		fmt.Println(aurora.Red(fmt.Sprintf("✖ Latest version name '%s' is invalid. Please report this issue to https://github.com/apppackio/apppack/issues/", latest.Name)))
		return false
	}
	current, err_2 := vers.NewVersion(Version)
	if err_2 != nil {
		fmt.Println(aurora.Red(fmt.Sprintf("✖ Current version name '%s' is invalid. Please report this issue to https://github.com/apppackio/apppack/issues/", Version)))
		return false
	}

	if current.LessThan(newVersion) {
		return false
	}
	return true
}
