package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/nemith/dothome"
	"nemith.io/nvueschema"
)

var logger = log.NewWithOptions(os.Stderr, log.Options{
	Level: log.WarnLevel,
})

func setVerbose(v bool) {
	if v {
		logger.SetLevel(log.InfoLevel)
	} else {
		logger.SetLevel(log.WarnLevel)
	}
}

// cachedFetch downloads the spec for a version, using the local cache.
// It does conditional GETs with If-Modified-Since when possible.
func cachedFetch(v nvueschema.VersionInfo, noCache bool) ([]byte, error) {
	if noCache {
		return nvueschema.FetchSpec(v)
	}

	u := nvueschema.SpecURL(v)

	jsonPath, tsPath, err := cachePaths(v.Slug)
	if err != nil {
		return nil, err
	}

	cachedData, _ := os.ReadFile(jsonPath)
	cachedLastMod, _ := os.ReadFile(tsPath)
	storedLastMod := strings.TrimSpace(string(cachedLastMod))

	// Have cache + Last-Modified: do conditional GET.
	if len(cachedData) > 0 && storedLastMod != "" {
		body, lastMod, notModified, err := httpFetchConditional(u, storedLastMod)
		if err != nil {
			logger.Warn("validation failed, using cache", "err", err)
			return cachedData, nil
		}
		if notModified {
			logger.Info("cache is current", "path", jsonPath)
			return cachedData, nil
		}
		writeCache(jsonPath, tsPath, body, lastMod)
		return body, nil
	}

	// Have cache but no Last-Modified: just use it.
	if len(cachedData) > 0 {
		logger.Info("using cached", "path", jsonPath)
		return cachedData, nil
	}

	// No cache: download fresh.
	logger.Info("fetching", "url", u)
	body, err := nvueschema.FetchSpec(v)
	if err != nil {
		return nil, err
	}

	writeCache(jsonPath, tsPath, body, "")
	return body, nil
}

func cachePaths(slug string) (jsonPath, tsPath string, err error) {
	layout, err := dothome.CLIAppLayout(dothome.AppConfig{Name: "nvueschema"})
	if err != nil {
		return "", "", fmt.Errorf("resolving cache dir: %w", err)
	}
	base := filepath.Join(layout.CacheDir, "openapi-"+slug)
	return base + ".json", base + ".lastmod", nil
}

// httpFetchConditional does a conditional GET with If-Modified-Since.
// Returns the body and Last-Modified header, or notModified=true on 304.
func httpFetchConditional(url, ifModSince string) (body []byte, lastMod string, notModified bool, err error) {
	logger.Info("checking", "url", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", false, err
	}
	req.Header.Set("User-Agent", nvueschema.UserAgent)
	req.Header.Set("If-Modified-Since", ifModSince)

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
		logger.Warn("could not create cache dir", "err", err)
		return
	}

	if err := os.WriteFile(jsonPath, body, 0o644); err != nil {
		logger.Warn("could not write cache", "err", err)
		return
	}

	logger.Info("cached", "path", jsonPath, "bytes", len(body))

	if lastMod != "" {
		if err := os.WriteFile(tsPath, []byte(lastMod), 0o644); err != nil {
			logger.Warn("could not write timestamp", "err", err)
		}
	}
}
