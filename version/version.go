package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
		fmt.Println(fmt.Sprintf("{:?}", err))
		return release, err
	}
	return release, nil
}

func IsUpToDate(latest *Release) bool {
	if latest == nil {
		latestObj, err := GetLatestRelease()
		if err != nil {
			return false
		}
		latest = &latestObj
	}
	if latest.Name < Version {
		return true
	}
	return false
}
