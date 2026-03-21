package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nemith/dothome"
	"github.com/spf13/cobra"
)

// userAgent must be set on all requests to api-prod.nvidia.com.
// Akamai's bot protection blocks Go's default "Go-http-client/1.1" UA,
// causing the connection to hang indefinitely.
const userAgent = "cumulus-schema/1.0"

var versionPattern = regexp.MustCompile(`^5\.(\d+)(?:\.(\d+))?$`)

type versionInfo struct {
	major int
	minor int
	slug  string
}

func parseVersion(version string) (versionInfo, error) {
	m := versionPattern.FindStringSubmatch(version)
	if m == nil {
		return versionInfo{}, fmt.Errorf("invalid version %q (expected 5.x or 5.x.y)", version)
	}
	minor, _ := strconv.Atoi(m[1])
	return versionInfo{
		major: 5,
		minor: minor,
		slug:  "5" + m[1],
	}, nil
}

// specURL returns the download URL for a given version.
// Versions 5.0-5.8 use the old docs.nvidia.com path.
// Versions 5.9+ use the new api-prod.nvidia.com endpoint.
func specURL(v versionInfo) string {
	if v.minor >= 9 {
		filename := fmt.Sprintf("openapi %d.%d.0.json", v.major, v.minor)
		return "https://api-prod.nvidia.com/openapi-browser/" + url.PathEscape(filename)
	}
	return "https://docs.nvidia.com/networking-ethernet-software/cumulus-linux-" +
		url.PathEscape(v.slug) + "/api/openapi.json"
}

func cachePaths(slug string) (jsonPath, tsPath string, err error) {
	layout, err := dothome.CLIAppLayout(dothome.AppConfig{Name: "cumulus-schema"})
	if err != nil {
		return "", "", fmt.Errorf("resolving cache dir: %w", err)
	}
	base := filepath.Join(layout.CacheDir, "openapi-"+slug)
	return base + ".json", base + ".lastmod", nil
}

// fetchSpec downloads a spec for the given parsed version, using the cache.
// Set noCache to skip the cache entirely.
func fetchSpec(v versionInfo, noCache bool) ([]byte, error) {
	url := specURL(v)

	if noCache {
		body, _, _, err := httpFetch(url, "")
		if err != nil {
			return nil, err
		}
		return body, nil
	}

	jsonPath, tsPath, err := cachePaths(v.slug)
	if err != nil {
		return nil, err
	}

	cachedData, _ := os.ReadFile(jsonPath)
	cachedLastMod, _ := os.ReadFile(tsPath)
	storedLastMod := strings.TrimSpace(string(cachedLastMod))

	// Have cache + Last-Modified: do conditional GET.
	if len(cachedData) > 0 && storedLastMod != "" {
		body, lastMod, notModified, err := httpFetch(url, storedLastMod)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: validation failed: %v; using cache\n", err)
			return cachedData, nil
		}
		if notModified {
			fmt.Fprintf(os.Stderr, "Cache is current %s\n", jsonPath)
			return cachedData, nil
		}
		writeCache(jsonPath, tsPath, body, lastMod)
		return body, nil
	}

	// Have cache but no Last-Modified: just use it.
	if len(cachedData) > 0 {
		fmt.Fprintf(os.Stderr, "Using cached %s\n", jsonPath)
		return cachedData, nil
	}

	// No cache: download fresh.
	body, lastMod, _, err := httpFetch(url, "")
	if err != nil {
		return nil, err
	}

	writeCache(jsonPath, tsPath, body, lastMod)
	return body, nil
}

// httpFetch does a GET with our custom User-Agent. If ifModSince is
// non-empty it's sent as If-Modified-Since for cache validation.
// Returns (nil, "", true, nil) on 304 Not Modified.
func httpFetch(url, ifModSince string) (body []byte, lastMod string, notModified bool, err error) {
	if ifModSince != "" {
		fmt.Fprintf(os.Stderr, "Checking %s\n", url)
	} else {
		fmt.Fprintf(os.Stderr, "Fetching %s\n", url)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("User-Agent", userAgent)
	if ifModSince != "" {
		req.Header.Set("If-Modified-Since", ifModSince)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", false, fmt.Errorf("downloading: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return nil, "", true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, fmt.Errorf("server returned %s", resp.Status)
	}

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", false, fmt.Errorf("reading response: %w", err)
	}

	return body, resp.Header.Get("Last-Modified"), false, nil
}

func writeCache(jsonPath, tsPath string, body []byte, lastMod string) {
	if err := os.MkdirAll(filepath.Dir(jsonPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create cache dir: %v\n", err)
		return
	}

	if err := os.WriteFile(jsonPath, body, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write cache: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "Cached %s (%d bytes)\n", jsonPath, len(body))

	if lastMod != "" {
		if err := os.WriteFile(tsPath, []byte(lastMod), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not write timestamp: %v\n", err)
		}
	}
}

func newFetchCmd() *cobra.Command {
	var (
		outputFile string
		noCache    bool
	)

	cmd := &cobra.Command{
		Use:   "fetch <version>",
		Short: "Download an NVUE OpenAPI spec from NVIDIA",
		Long: strings.TrimSpace(`
Download the NVUE OpenAPI spec for a given Cumulus Linux version.

Specs are cached locally and validated with If-Modified-Since.
Use --no-cache to skip the cache entirely.

Examples:
  cumulus-schema fetch 5.16
  cumulus-schema fetch 5.14 -o cumulus-514.json
  cumulus-schema fetch 5.5 --no-cache
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := parseVersion(args[0])
			if err != nil {
				return err
			}

			data, err := fetchSpec(v, noCache)
			if err != nil {
				return err
			}

			var w io.Writer = os.Stdout
			if outputFile != "" && outputFile != "-" {
				file, err := os.Create(outputFile)
				if err != nil {
					return err
				}
				defer file.Close()
				w = file
			}

			n, err := w.Write(data)
			if err != nil {
				return err
			}
			if outputFile != "" && outputFile != "-" {
				fmt.Fprintf(os.Stderr, "Wrote %s (%d bytes)\n", outputFile, n)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "Skip cache entirely (don't read or write)")

	return cmd
}
