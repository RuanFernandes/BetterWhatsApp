package updater

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultRepository             = "RuanFernandes/BetterWhatsApp"
	defaultReleasesURL            = "https://api.github.com/repos/" + DefaultRepository + "/releases?per_page=30"
	defaultUserAgent              = "BetterWhatsApp/0.1.0"
	maxReleaseResponseBytes       = 4 << 20
	maxInstallerSize        int64 = 512 << 20
)

type Release struct {
	Version     string
	Name        string
	URL         string
	AssetName   string
	DownloadURL string
	AssetSize   int64
	Digest      string
	PublishedAt time.Time
}

type Downloaded struct {
	Release Release
	Path    string
	Size    int64
}

type Client struct {
	httpClient  *http.Client
	releasesURL string
	downloadDir string
	userAgent   string
}

type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	HTMLURL     string        `json:"html_url"`
	Draft       bool          `json:"draft"`
	PublishedAt string        `json:"published_at"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
}

func New(downloadDir string) *Client {
	return newClient(
		&http.Client{Timeout: 20 * time.Second},
		defaultReleasesURL,
		downloadDir,
		defaultUserAgent,
	)
}

func newClient(httpClient *http.Client, releasesURL, downloadDir, userAgent string) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{
		httpClient:  httpClient,
		releasesURL: strings.TrimSpace(releasesURL),
		downloadDir: filepath.Clean(downloadDir),
		userAgent:   strings.TrimSpace(userAgent),
	}
}

func (c *Client) Check(ctx context.Context, currentVersion string) (Release, bool, error) {
	release, err := c.latestRelease(ctx)
	if err != nil {
		return Release{}, false, err
	}
	return release, IsNewer(currentVersion, release.Version), nil
}

func (c *Client) Download(ctx context.Context, release Release) (Downloaded, error) {
	if release.Version == "" {
		return Downloaded{}, errors.New("update version is empty")
	}
	if release.AssetName == "" || release.DownloadURL == "" {
		return Downloaded{}, errors.New("update installer asset is incomplete")
	}

	downloadURL, err := url.Parse(release.DownloadURL)
	if err != nil ||
		downloadURL.Scheme != "https" ||
		!strings.EqualFold(downloadURL.Hostname(), "github.com") {
		return Downloaded{}, errors.New("update installer URL is not a trusted GitHub URL")
	}

	assetName := filepath.Base(filepath.Clean(release.AssetName))
	if assetName == "." ||
		assetName == string(filepath.Separator) ||
		assetName != release.AssetName ||
		!strings.HasSuffix(strings.ToLower(assetName), "-installer.exe") {
		return Downloaded{}, errors.New("update installer asset name is invalid")
	}

	versionDirectory := filepath.Join(c.downloadDir, safePathSegment(release.Version))
	if err := os.MkdirAll(versionDirectory, 0o700); err != nil {
		return Downloaded{}, fmt.Errorf("create update directory: %w", err)
	}
	targetPath := filepath.Join(versionDirectory, assetName)

	if fileInfo, statErr := os.Stat(targetPath); statErr == nil && fileInfo.Mode().IsRegular() {
		if size, verifyErr := verifyFile(targetPath, release.AssetSize, release.Digest); verifyErr == nil {
			return Downloaded{Release: release, Path: targetPath, Size: size}, nil
		}
		_ = os.Remove(targetPath)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return Downloaded{}, fmt.Errorf("inspect downloaded installer: %w", statErr)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return Downloaded{}, fmt.Errorf("create installer download request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", c.userAgent)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Downloaded{}, fmt.Errorf("download installer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Downloaded{}, fmt.Errorf("download installer returned HTTP %s", response.Status)
	}
	if response.ContentLength > maxInstallerSize {
		return Downloaded{}, errors.New("update installer exceeds the allowed size")
	}

	temporary, err := os.CreateTemp(versionDirectory, ".installer-*.part")
	if err != nil {
		return Downloaded{}, fmt.Errorf("create temporary installer: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	defer cleanup()

	hasher := sha256.New()
	size, err := io.Copy(
		io.MultiWriter(temporary, hasher),
		io.LimitReader(response.Body, maxInstallerSize+1),
	)
	if err != nil {
		return Downloaded{}, fmt.Errorf("write downloaded installer: %w", err)
	}
	if size > maxInstallerSize {
		return Downloaded{}, errors.New("update installer exceeds the allowed size")
	}
	if release.AssetSize > 0 && size != release.AssetSize {
		return Downloaded{}, fmt.Errorf("installer size mismatch: got %d bytes, want %d", size, release.AssetSize)
	}
	if err := verifyDigest(hasher.Sum(nil), release.Digest); err != nil {
		return Downloaded{}, err
	}
	if err := temporary.Close(); err != nil {
		return Downloaded{}, fmt.Errorf("close temporary installer: %w", err)
	}

	_ = os.Remove(targetPath)
	if err := os.Rename(temporaryPath, targetPath); err != nil {
		return Downloaded{}, fmt.Errorf("finalize downloaded installer: %w", err)
	}
	return Downloaded{Release: release, Path: targetPath, Size: size}, nil
}

func (c *Client) latestRelease(ctx context.Context) (Release, error) {
	if c.releasesURL == "" {
		return Release{}, errors.New("GitHub releases URL is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.releasesURL, nil)
	if err != nil {
		return Release{}, fmt.Errorf("create GitHub releases request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", c.userAgent)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return Release{}, fmt.Errorf("check GitHub releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub releases returned HTTP %s", response.Status)
	}

	payload, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseResponseBytes+1))
	if err != nil {
		return Release{}, fmt.Errorf("read GitHub releases: %w", err)
	}
	if int64(len(payload)) > maxReleaseResponseBytes {
		return Release{}, errors.New("GitHub releases response is too large")
	}

	var releases []githubRelease
	if err := json.Unmarshal(payload, &releases); err != nil {
		return Release{}, fmt.Errorf("decode GitHub releases: %w", err)
	}

	var latest Release
	found := false
	for _, candidate := range releases {
		if candidate.Draft {
			continue
		}
		asset, ok := selectInstallerAsset(candidate.Assets)
		if !ok || strings.TrimSpace(candidate.TagName) == "" {
			continue
		}
		publishedAt, _ := time.Parse(time.RFC3339, candidate.PublishedAt)
		release := Release{
			Version:     strings.TrimSpace(candidate.TagName),
			Name:        strings.TrimSpace(candidate.Name),
			URL:         strings.TrimSpace(candidate.HTMLURL),
			AssetName:   asset.Name,
			DownloadURL: asset.BrowserDownloadURL,
			AssetSize:   asset.Size,
			Digest:      asset.Digest,
			PublishedAt: publishedAt,
		}
		if release.Name == "" {
			release.Name = release.Version
		}
		if !found || releaseIsLater(release, latest) {
			latest = release
			found = true
		}
	}
	if !found {
		return Release{}, errors.New("no published Windows installer release was found")
	}
	return latest, nil
}

func selectInstallerAsset(assets []githubAsset) (githubAsset, bool) {
	var selected githubAsset
	bestRank := 100
	for _, asset := range assets {
		name := strings.ToLower(strings.TrimSpace(asset.Name))
		if !strings.Contains(name, "betterwhatsapp") ||
			!strings.HasSuffix(name, "-installer.exe") ||
			strings.TrimSpace(asset.BrowserDownloadURL) == "" {
			continue
		}

		rank := 3
		switch {
		case name == "betterwhatsapp-amd64-installer.exe":
			rank = 0
		case strings.HasSuffix(name, "-amd64-installer.exe"):
			rank = 1
		}
		if rank < bestRank {
			selected = asset
			bestRank = rank
		}
	}
	return selected, bestRank < 100
}

func releaseIsLater(candidate, current Release) bool {
	if candidate.PublishedAt.After(current.PublishedAt) {
		return true
	}
	if candidate.PublishedAt.Equal(current.PublishedAt) {
		candidateBuild, candidateOK := buildNumber(candidate.Version)
		currentBuild, currentOK := buildNumber(current.Version)
		return candidateOK && currentOK && candidateBuild > currentBuild
	}
	return false
}

func IsNewer(currentVersion, latestVersion string) bool {
	currentVersion = normalizeVersion(currentVersion)
	latestVersion = normalizeVersion(latestVersion)
	if latestVersion == "" || currentVersion == latestVersion {
		return false
	}

	currentBuild, currentBuildOK := buildNumber(currentVersion)
	latestBuild, latestBuildOK := buildNumber(latestVersion)
	if currentBuildOK && latestBuildOK {
		return latestBuild > currentBuild
	}

	currentSemver, currentSemverOK := semanticVersion(currentVersion)
	latestSemver, latestSemverOK := semanticVersion(latestVersion)
	if currentSemverOK && latestSemverOK {
		for index := range currentSemver {
			if latestSemver[index] != currentSemver[index] {
				return latestSemver[index] > currentSemver[index]
			}
		}
		return false
	}
	return true
}

func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

func buildNumber(version string) (int, bool) {
	if !strings.HasPrefix(version, "build-") {
		return 0, false
	}
	value, err := strconv.Atoi(strings.TrimPrefix(version, "build-"))
	return value, err == nil && value >= 0
}

func semanticVersion(version string) ([3]int, bool) {
	var result [3]int
	version = strings.SplitN(version, "-", 2)[0]
	parts := strings.Split(version, ".")
	if len(parts) != len(result) {
		return result, false
	}
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return result, false
		}
		result[index] = value
	}
	return result, true
}

func safePathSegment(value string) string {
	var builder strings.Builder
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z',
			char >= 'A' && char <= 'Z',
			char >= '0' && char <= '9',
			char == '.', char == '-', char == '_':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func verifyFile(path string, expectedSize int64, expectedDigest string) (int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, io.LimitReader(file, maxInstallerSize+1))
	if err != nil {
		return 0, err
	}
	if size > maxInstallerSize {
		return 0, errors.New("downloaded installer exceeds the allowed size")
	}
	if expectedSize > 0 && size != expectedSize {
		return 0, fmt.Errorf("installer size mismatch: got %d bytes, want %d", size, expectedSize)
	}
	if err := verifyDigest(hasher.Sum(nil), expectedDigest); err != nil {
		return 0, err
	}
	return size, nil
}

func verifyDigest(actual []byte, expected string) error {
	expected = strings.TrimSpace(strings.ToLower(expected))
	if expected == "" {
		return nil
	}
	expected = strings.TrimPrefix(expected, "sha256:")
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha256.Size {
		return errors.New("GitHub release contains an invalid SHA-256 digest")
	}
	if subtle.ConstantTimeCompare(actual, decoded) != 1 {
		return errors.New("downloaded installer failed SHA-256 verification")
	}
	return nil
}
