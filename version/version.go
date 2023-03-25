package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
)

var BuildDate = time.Now().String()
var Version = "<version>"
var Commit = "<commit>"
var Environment = "development"

// This code is largely cherry-picked from https://github.com/cli/cli/blob/82927b0cc2a831adda22b0a7bf43938bd15e1126/internal/update/update.go
// It is licensed under the MIT license https://github.com/cli/cli/blob/82927b0cc2a831adda22b0a7bf43938bd15e1126/LICENSE

var gitDescribeSuffixRE = regexp.MustCompile(`\d+-\d+-g[a-f0-9]{8}$`)

// ReleaseInfo stores information about a release
type ReleaseInfo struct {
	Version     string    `json:"tag_name"`
	URL         string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
}

type StateEntry struct {
	CheckedForUpdateAt time.Time   `yaml:"checked_for_update_at"`
	LatestRelease      ReleaseInfo `yaml:"latest_release"`
}

// CheckForUpdate checks whether this software has had a newer release on GitHub
func CheckForUpdate(ctx context.Context, client *http.Client, stateFilePath, repo, currentVersion string) (*ReleaseInfo, error) {
	// stateEntry, _ := getStateEntry(stateFilePath)
	// if stateEntry != nil && time.Since(stateEntry.CheckedForUpdateAt).Hours() < 24 {
	// 	return nil, nil
	// }

	releaseInfo, err := getLatestReleaseInfo(ctx, client, repo)
	if err != nil {
		return nil, err
	}

	err = setStateEntry(stateFilePath, time.Now(), *releaseInfo)
	if err != nil {
		return nil, err
	}
	if versionGreaterThan(releaseInfo.Version, currentVersion) {
		return releaseInfo, nil
	}

	return nil, nil
}

func getLatestReleaseInfo(ctx context.Context, client *http.Client, repo string) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected HTTP %d", res.StatusCode)
	}
	dec := json.NewDecoder(res.Body)
	var latestRelease ReleaseInfo
	if err := dec.Decode(&latestRelease); err != nil {
		return nil, err
	}
	return &latestRelease, nil
}

func getStateEntry(stateFilePath string) (*StateEntry, error) {
	content, err := os.ReadFile(stateFilePath)
	if err != nil {
		return nil, err
	}

	var stateEntry StateEntry
	err = json.Unmarshal(content, &stateEntry)
	if err != nil {
		return nil, err
	}

	return &stateEntry, nil
}

func setStateEntry(stateFilePath string, t time.Time, r ReleaseInfo) error {
	data := StateEntry{CheckedForUpdateAt: t, LatestRelease: r}
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(stateFilePath), 0755)
	if err != nil {
		return err
	}

	err = os.WriteFile(stateFilePath, content, 0600)
	return err
}

func versionGreaterThan(v, w string) bool {
	w = gitDescribeSuffixRE.ReplaceAllStringFunc(w, func(m string) string {
		idx := strings.IndexRune(m, '-')
		n, _ := strconv.Atoi(m[0:idx])
		return fmt.Sprintf("%d-pre.0", n+1)
	})

	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}
