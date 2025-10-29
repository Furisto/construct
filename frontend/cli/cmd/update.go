package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"

	"github.com/Masterminds/semver/v3"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/furisto/construct/shared/config"
	updater "github.com/inconshreveable/go-update"
	"github.com/spf13/cobra"
)

const (
	USReleaseUrl = "https://us-construct-releases.s3.us-east-1.amazonaws.com"
	EUReleaseUrl = "https://eu-construct-releases.s3.eu-central-1.amazonaws.com"
)

type Manifest struct {
	Channel   string     `json:"channel"`
	Version   string     `json:"version"`
	Artifacts []Artifact `json:"artifacts"`
}

type Artifact struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
}

func NewUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "update",
		Short:   "Update Construct to the latest version",
		GroupID: "system",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true

			httpClient := getHttpClient(cmd.Context())
			configStore := getConfigStore(cmd.Context())

			return fail.HandleError(cmd, update(httpClient, configStore, cmd.OutOrStdout()))
		},
	}
}

func update(httpClient *http.Client, configStore *config.Store, stdout io.Writer) error {
	releaseUrlVal, ok := configStore.Get("update.url")
	var releaseUrl string
	if !ok {
		releaseUrl = selectEndpoint(httpClient)
	} else {
		if value, ok := releaseUrlVal.String(); ok {
			releaseUrl = value
		} else {
			return fmt.Errorf("update.url is not a string")
		}
	}

	channelVal, ok := configStore.Get("update.channel")
	var channel string
	if !ok {
		channel = "latest"
	} else {
		if value, ok := channelVal.String(); ok {
			channel = value
		} else {
			return fmt.Errorf("update.channel is not a string")
		}
	}

	updater := NewUpdater(
		WithHttpClient(httpClient),
		WithChannel(channel),
		WithDownloadUrl(releaseUrl),
		WithStdout(stdout),
	)

	return updater.Run()
}

func (u *Updater) Run() error {
	manifest, err := u.downloadManifest()
	if err != nil {
		return err
	}

	shouldUpdate, err := u.checkForUpdate(manifest)
	if err != nil {
		return err
	}

	if !shouldUpdate {
		fmt.Fprintln(u.stdout, "Already up to date! Current version: ", Version)
		return nil
	}
	fmt.Fprintln(u.stdout, "Updating from ", Version, " to ", manifest.Version)

	platform, err := u.platformString()
	if err != nil {
		return err
	}

	checksum, err := u.downloadChecksum(manifest.Version, platform)
	if err != nil {
		return err
	}

	artifact, err := u.downloadArtifact(manifest, platform)
	if err != nil {
		return err
	}
	defer artifact.Close()

	return u.applyUpdate(artifact, checksum)
}

func (u *Updater) downloadManifest() (*Manifest, error) {
	parsedUrl, err := url.Parse(u.downloadUrl)
	if err != nil {
		return nil, err
	}
	manifestUrl := parsedUrl.JoinPath(fmt.Sprintf("%s.json", u.channel))

	response, err := u.httpClient.Get(manifestUrl.String())
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get manifest: %s", response.Status)
	}

	var manifest Manifest
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

func (u *Updater) checkForUpdate(manifest *Manifest) (bool, error) {
	latestVersion, err := semver.NewVersion(manifest.Version)
	if err != nil {
		return false, err
	}

	var current string
	if Version == "unknown" {
		current = manifest.Version
	} else {
		current = Version
	}

	currentVersion, err := semver.NewVersion(current)
	if err != nil {
		return false, err
	}

	if latestVersion.LessThanEqual(currentVersion) {
		return false, nil
	}

	return true, nil
}

func (u *Updater) downloadChecksum(version string, platform string) ([]byte, error) {
	parsedUrl, err := url.Parse(u.downloadUrl)
	if err != nil {
		return nil, err
	}
	checksumUrl := parsedUrl.JoinPath("versions", version, fmt.Sprintf("%s_checksum.txt", platform))

	response, err := u.httpClient.Get(checksumUrl.String())
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get checksum: %s", response.Status)
	}

	checksumHex, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	checksum, err := hex.DecodeString(string(bytes.TrimSpace(checksumHex)))
	if err != nil {
		return nil, fmt.Errorf("invalid checksum format: %w", err)
	}
	return checksum, nil
}

func (u *Updater) downloadArtifact(manifest *Manifest, platform string) (io.ReadCloser, error) {
	for _, artifact := range manifest.Artifacts {
		if artifact.Platform == platform {
			artifactResponse, err := u.httpClient.Get(artifact.URL)
			if err != nil {
				return nil, err
			}

			if artifactResponse.StatusCode != 200 {
				return nil, fmt.Errorf("failed to download artifact: %s", artifact.URL)
			}
			return artifactResponse.Body, nil
		}
	}

	return nil, fmt.Errorf("artifact not found for platform: %s", platform)
}

func (u *Updater) applyUpdate(artifact io.Reader, checksum []byte) error {
	gzipReader, err := gzip.NewReader(artifact)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Name == "construct" {
			tempFile, err := os.CreateTemp("", "construct-update-*")
			if err != nil {
				return err
			}
			defer os.Remove(tempFile.Name())
			defer tempFile.Close()

			_, err = io.Copy(tempFile, tarReader)
			if err != nil {
				return err
			}
			tempFile.Close()

			updateFile, err := os.Open(tempFile.Name())
			if err != nil {
				return err
			}
			defer updateFile.Close()

			return updater.Apply(updateFile, updater.Options{
				Checksum: checksum,
			})
		}
	}

	return fmt.Errorf("construct binary not found in archive")
}

type geoResponse struct {
	Continent string `json:"continent"`
}

// TODO: Use CDN
func selectEndpoint(httpClient *http.Client) string {
	geoRequest, err := http.NewRequest("GET", "http://ip-api.com/json/", nil)
	if err != nil {
		return USReleaseUrl
	}
	urlValues := geoRequest.URL.Query()
	urlValues.Add("fields", "continent")
	geoRequest.URL.RawQuery = urlValues.Encode()

	resp, err := httpClient.Do(geoRequest)
	if err != nil {
		return USReleaseUrl
	}
	defer resp.Body.Close()

	var geo geoResponse
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil {
		return USReleaseUrl
	}

	switch geo.Continent {
	case "Europe", "Africa":
		return EUReleaseUrl
	default:
		return USReleaseUrl
	}
}

func (u *Updater) platformString() (string, error) {
	os := runtime.GOOS
	arch := runtime.GOARCH

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	return fmt.Sprintf("%s_%s", os, arch), nil
}

type UpdateOption func(*Updater)

func WithHttpClient(httpClient *http.Client) UpdateOption {
	return func(o *Updater) {
		o.httpClient = httpClient
	}
}

func WithChannel(channel string) UpdateOption {
	return func(o *Updater) {
		o.channel = channel
	}
}

func WithDownloadUrl(downloadUrl string) UpdateOption {
	return func(o *Updater) {
		o.downloadUrl = downloadUrl
	}
}

func WithStdout(stdout io.Writer) UpdateOption {
	return func(o *Updater) {
		o.stdout = stdout
	}
}

type Updater struct {
	httpClient  *http.Client
	channel     string
	downloadUrl string
	stdout      io.Writer
}

func NewUpdater(options ...UpdateOption) *Updater {
	updater := &Updater{
		httpClient:  http.DefaultClient,
		channel:     "stable",
		downloadUrl: USReleaseUrl,
		stdout:      os.Stdout,
	}

	for _, option := range options {
		option(updater)
	}

	return updater
}
