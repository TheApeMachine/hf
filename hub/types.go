package hub

import (
	"fmt"
	"strings"
	"time"

	fiberclient "github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/caramba/pkg/config"
)

type RepoType string

const (
	ModelRepo   RepoType = "model"
	DatasetRepo RepoType = "dataset"
	SpaceRepo   RepoType = "space"
)

const (
	defaultRevision = "main"
)

/*
Client downloads and caches Hugging Face Hub assets with revision-aware
provenance metadata.
*/
type Client struct {
	config     *config.HubConfig
	httpClient *fiberclient.Client
}

/*
NewClient constructs a Hub client from config.yml-backed settings.
*/
func NewClient(hubConfig *config.HubConfig) *Client {
	if hubConfig == nil {
		hubConfig = config.NewHubConfig()
	}

	return NewClientWithHTTP(hubConfig, nil)
}

/*
NewClientWithHTTP constructs a Hub client with an injected HTTP transport.
It is primarily used by tests.
*/
func NewClientWithHTTP(
	hubConfig *config.HubConfig, httpClient *fiberclient.Client,
) *Client {
	if hubConfig == nil {
		hubConfig = config.NewHubConfig()
	}

	if httpClient == nil {
		httpClient = fiberclient.New()
		httpClient.SetStreamResponseBody(true)
	}

	return &Client{
		config:     hubConfig,
		httpClient: httpClient,
	}
}

type DownloadRequest struct {
	RepoID   string
	RepoType RepoType
	Revision string
	Filename string
	CacheDir string
	Token    string
	Force    bool
	DryRun   bool
}

type SnapshotRequest struct {
	RepoID     string
	RepoType   RepoType
	Revision   string
	CacheDir   string
	Token      string
	Include    []string
	Exclude    []string
	Force      bool
	DryRun     bool
	MaxWorkers int
}

type File struct {
	RepoID        string    `json:"repo_id"`
	RepoType      RepoType  `json:"repo_type"`
	Revision      string    `json:"revision"`
	Commit        string    `json:"commit"`
	Filename      string    `json:"filename"`
	Path          string    `json:"path"`
	BlobPath      string    `json:"blob_path"`
	ETag          string    `json:"etag,omitempty"`
	XetHash       string    `json:"xet_hash,omitempty"`
	SHA256        string    `json:"sha256,omitempty"`
	Size          int64     `json:"size"`
	Cached        bool      `json:"cached"`
	WouldDownload bool      `json:"would_download"`
	DownloadedAt  time.Time `json:"downloaded_at,omitempty"`
}

type Snapshot struct {
	RepoID   string   `json:"repo_id"`
	RepoType RepoType `json:"repo_type"`
	Revision string   `json:"revision"`
	Commit   string   `json:"commit"`
	Path     string   `json:"path"`
	Files    []File   `json:"files"`
}

type Repository struct {
	ID       string
	RepoType RepoType
	Revision string
	Commit   string
	Siblings []Sibling
}

type Sibling struct {
	Filename string
	Size     int64
	LFS      *LFSMetadata
}

type LFSMetadata struct {
	SHA256      string
	Size        int64
	PointerSize int64
}

type RepoLocation struct {
	RepoID   string
	RepoType RepoType
	Revision string
}

func parseRepoType(value string) (RepoType, error) {
	switch RepoType(strings.TrimSpace(value)) {
	case "", ModelRepo:
		return ModelRepo, nil
	case DatasetRepo:
		return DatasetRepo, nil
	case SpaceRepo:
		return SpaceRepo, nil
	default:
		return "", fmt.Errorf("hub: unsupported repo type %q", value)
	}
}

func normalizeRevision(revision string) string {
	if strings.TrimSpace(revision) == "" {
		return defaultRevision
	}

	return revision
}

func (repoType RepoType) apiPlural() (string, error) {
	switch repoType {
	case "", ModelRepo:
		return "models", nil
	case DatasetRepo:
		return "datasets", nil
	case SpaceRepo:
		return "spaces", nil
	default:
		return "", fmt.Errorf("hub: unsupported repo type %q", repoType)
	}
}

func (repoType RepoType) resolvePrefix() (string, error) {
	switch repoType {
	case "", ModelRepo:
		return "", nil
	case DatasetRepo:
		return "datasets", nil
	case SpaceRepo:
		return "spaces", nil
	default:
		return "", fmt.Errorf("hub: unsupported repo type %q", repoType)
	}
}
