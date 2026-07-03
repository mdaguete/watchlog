package cache

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ImageCache manages a local disk cache of TMDB images.
type ImageCache struct {
	dir        string
	httpClient *http.Client
}

// NewImageCache creates a cache in the given directory, creating it if needed.
func NewImageCache(dir string) (*ImageCache, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	return &ImageCache{
		dir:        dir,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// LocalPath returns the local cache path for a TMDB image URL.
// Input:  https://image.tmdb.org/t/p/w342/abc123.jpg
// Output: /data/cache/images/w342/abc123.jpg
func (c *ImageCache) LocalPath(tmdbURL string) string {
	// Extract size and filename from URL
	// Format: https://image.tmdb.org/t/p/{size}/{path}
	parts := strings.Split(tmdbURL, "/t/p/")
	if len(parts) != 2 {
		return ""
	}
	return filepath.Join(c.dir, parts[1])
}

// Exists checks if an image is already cached locally.
func (c *ImageCache) Exists(tmdbURL string) bool {
	local := c.LocalPath(tmdbURL)
	if local == "" {
		return false
	}
	_, err := os.Stat(local)
	return err == nil
}

// Ensure downloads an image if not already cached. Returns the local serve path.
func (c *ImageCache) Ensure(tmdbURL string) (string, error) {
	if tmdbURL == "" {
		return "", nil
	}

	localPath := c.LocalPath(tmdbURL)
	if localPath == "" {
		return tmdbURL, nil // can't parse, return original
	}

	// Already cached?
	if _, err := os.Stat(localPath); err == nil {
		return c.servePath(localPath), nil
	}

	// Download
	if err := c.download(tmdbURL, localPath); err != nil {
		return tmdbURL, err // fallback to remote URL on error
	}

	return c.servePath(localPath), nil
}

// ServePath converts a local filesystem path to the URL path served by the HTTP handler.
func (c *ImageCache) servePath(localPath string) string {
	rel, _ := filepath.Rel(c.dir, localPath)
	return "/static/cache/images/" + filepath.ToSlash(rel)
}

// Download fetches an image from TMDB and saves it locally.
func (c *ImageCache) download(url, destPath string) error {
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, io.LimitReader(resp.Body, 10<<20)) // 10MB max per image
	return err
}

// CacheAll downloads all uncached images from a list of URLs.
// Used during TMDB refresh to pre-cache all posters.
func (c *ImageCache) CacheAll(urls []string) int {
	cached := 0
	for _, url := range urls {
		if url == "" || c.Exists(url) {
			continue
		}
		if _, err := c.Ensure(url); err != nil {
			log.Printf("Cache: failed to download %s: %v", url, err)
			continue
		}
		cached++
	}
	return cached
}
