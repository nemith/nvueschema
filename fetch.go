package nvueschema

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

// userAgent must be set on all requests to api-prod.nvidia.com.
// Akamai's bot protection blocks Go's default "Go-http-client/1.1" UA,
// causing the connection to hang indefinitely.
const UserAgent = "cumulus-schema/1.0"

var versionPattern = regexp.MustCompile(`^5\.(\d+)(?:\.(\d+))?$`)

// VersionInfo holds parsed version information.
type VersionInfo struct {
	Major int
	Minor int
	Slug  string
}

// ParseVersion parses a Cumulus Linux version string (e.g. "5.14").
func ParseVersion(version string) (VersionInfo, error) {
	m := versionPattern.FindStringSubmatch(version)
	if m == nil {
		return VersionInfo{}, fmt.Errorf("invalid version %q (expected 5.x or 5.x.y)", version)
	}
	minor, _ := strconv.Atoi(m[1])
	return VersionInfo{
		Major: 5,
		Minor: minor,
		Slug:  "5" + m[1],
	}, nil
}

// SpecURL returns the download URL for a given version.
// Versions 5.0-5.8 use the old docs.nvidia.com path.
// Versions 5.9+ use the new api-prod.nvidia.com endpoint.
func SpecURL(v VersionInfo) string {
	if v.Minor >= 9 {
		filename := fmt.Sprintf("openapi %d.%d.0.json", v.Major, v.Minor)
		return "https://api-prod.nvidia.com/openapi-browser/" + url.PathEscape(filename)
	}
	return "https://docs.nvidia.com/networking-ethernet-software/cumulus-linux-" +
		url.PathEscape(v.Slug) + "/api/openapi.json"
}

// FetchSpec downloads the spec for the given version from Nvidia.
func FetchSpec(v VersionInfo) ([]byte, error) {
	u := SpecURL(v)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return body, nil
}
