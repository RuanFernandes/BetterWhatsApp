package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

type githubAssetTransport struct {
	serverURL string
	next      http.RoundTripper
}

func (transport githubAssetTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	target, err := url.Parse(transport.serverURL)
	if err != nil {
		return nil, err
	}
	cloned := request.Clone(request.Context())
	if request.URL.Hostname() == "github.com" {
		cloned.URL.Path = "/installer.exe"
		cloned.URL.RawPath = ""
	}
	cloned.URL.Scheme = target.Scheme
	cloned.URL.Host = target.Host
	return transport.next.RoundTrip(cloned)
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "same build", current: "build-4", latest: "build-4", want: false},
		{name: "newer build", current: "build-4", latest: "build-5", want: true},
		{name: "older build", current: "build-5", latest: "build-4", want: false},
		{name: "new semantic version", current: "0.1.0", latest: "0.2.0", want: true},
		{name: "development version", current: "0.1.0-dev", latest: "build-4", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsNewer(test.current, test.latest); got != test.want {
				t.Fatalf("IsNewer(%q, %q) = %v, want %v", test.current, test.latest, got, test.want)
			}
		})
	}
}

func TestClientChecksAndDownloadsVerifiedInstaller(t *testing.T) {
	installer := []byte("installer-binary")
	digest := sha256.Sum256(installer)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/releases":
			_ = json.NewEncoder(response).Encode([]map[string]any{
				{
					"tag_name":     "build-4",
					"name":         "Older",
					"html_url":     "https://github.com/RuanFernandes/BetterWhatsApp/releases/tag/build-4",
					"published_at": "2026-09-09T17:45:54Z",
					"assets": []map[string]any{
						{"name": "BetterWhatsApp-amd64-installer.exe", "browser_download_url": "https://github.com/RuanFernandes/BetterWhatsApp/releases/download/build-4/BetterWhatsApp-amd64-installer.exe", "size": 1},
					},
				},
				{
					"tag_name":     "build-5",
					"name":         "Newest",
					"html_url":     "https://github.com/RuanFernandes/BetterWhatsApp/releases/tag/build-5",
					"published_at": "2026-09-09T18:45:54Z",
					"assets": []map[string]any{
						{
							"name":                 "BetterWhatsApp-amd64-installer.exe",
							"browser_download_url": "https://github.com/RuanFernandes/BetterWhatsApp/releases/download/build-5/BetterWhatsApp-amd64-installer.exe",
							"size":                 len(installer),
							"digest":               "sha256:" + hex.EncodeToString(digest[:]),
						},
					},
				},
			})
		case "/installer.exe":
			_, _ = response.Write(installer)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	client := newClient(
		&http.Client{
			Transport: githubAssetTransport{
				serverURL: server.URL,
				next:      http.DefaultTransport,
			},
		},
		server.URL+"/releases",
		filepath.Join(t.TempDir(), "updates"),
		"test",
	)
	release, available, err := client.Check(context.Background(), "build-4")
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !available || release.Version != "build-5" {
		t.Fatalf("Check() = %#v, available=%v; want build-5 and available", release, available)
	}

	downloaded, err := client.Download(context.Background(), release)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if downloaded.Size != int64(len(installer)) {
		t.Fatalf("Download() size = %d, want %d", downloaded.Size, len(installer))
	}
	if _, err := client.Download(context.Background(), release); err != nil {
		t.Fatalf("Download() cached file error = %v", err)
	}
}
