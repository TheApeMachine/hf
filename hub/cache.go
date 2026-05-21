package hub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type cachePaths struct {
	root      string
	repo      string
	refs      string
	blobs     string
	snapshots string
	info      string
	metadata  string
	tmp       string
}

func newCachePaths(cacheDir string, repoType RepoType, repoID string) cachePaths {
	repoName := string(repoType) + "s--" + strings.ReplaceAll(repoID, "/", "--")
	repoDir := filepath.Join(cacheDir, repoName)

	return cachePaths{
		root:      cacheDir,
		repo:      repoDir,
		refs:      filepath.Join(repoDir, "refs"),
		blobs:     filepath.Join(repoDir, "blobs"),
		snapshots: filepath.Join(repoDir, "snapshots"),
		info:      filepath.Join(repoDir, "info"),
		metadata:  filepath.Join(repoDir, "metadata"),
		tmp:       filepath.Join(repoDir, "tmp"),
	}
}

func (paths cachePaths) ensure() error {
	for _, path := range []string{
		paths.refs,
		paths.blobs,
		paths.snapshots,
		paths.info,
		paths.metadata,
		paths.tmp,
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("hub: mkdir %s: %w", path, err)
		}
	}

	return nil
}

func (paths cachePaths) snapshotDir(commit string) string {
	return filepath.Join(paths.snapshots, commit)
}

func (paths cachePaths) snapshotFile(commit, filename string) string {
	return filepath.Join(paths.snapshotDir(commit), filepath.FromSlash(filename))
}

func (paths cachePaths) metadataFile(commit, filename string) string {
	return filepath.Join(paths.metadata, commit, filepath.FromSlash(filename)+".json")
}

func (paths cachePaths) refFile(revision string) string {
	return filepath.Join(paths.refs, filepath.FromSlash(revision))
}

func (paths cachePaths) infoFile(revision string) string {
	return filepath.Join(paths.info, filepath.FromSlash(revision))
}

func (paths cachePaths) blobFile(identity string) string {
	return filepath.Join(paths.blobs, sanitizeIdentity(identity))
}

func (paths cachePaths) writeRef(revision, commit string) error {
	path := paths.refFile(revision)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("hub: mkdir ref: %w", err)
	}

	return os.WriteFile(path, []byte(commit+"\n"), 0o644)
}

func (paths cachePaths) writeMetadata(file File) error {
	path := paths.metadataFile(file.Commit, file.Filename)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("hub: mkdir metadata: %w", err)
	}

	data, err := json.MarshalIndent(file, "", "  ")

	if err != nil {
		return fmt.Errorf("hub: marshal metadata: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

func (paths cachePaths) writeInfo(revision string, repository *Repository) error {
	path := paths.infoFile(revision)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("hub: mkdir info: %w", err)
	}

	siblings := make([]siblingPayload, 0, len(repository.Siblings))

	for _, sibling := range repository.Siblings {
		payload := siblingPayload{
			RFilename: sibling.Filename,
			Size:      sibling.Size,
		}

		if sibling.LFS != nil {
			payload.LFS = &lfsPayload{
				SHA256:      sibling.LFS.SHA256,
				Size:        sibling.LFS.Size,
				PointerSize: sibling.LFS.PointerSize,
			}
		}

		siblings = append(siblings, payload)
	}

	payload := repositoryPayload{
		ID:       repository.ID,
		SHA:      repository.Commit,
		Siblings: siblings,
	}

	data, err := json.Marshal(payload)

	if err != nil {
		return fmt.Errorf("hub: marshal info: %w", err)
	}

	return os.WriteFile(path, data, 0o644)
}

func installBlob(paths cachePaths, tmpPath, identity string) (string, string, error) {
	sha, err := sha256File(tmpPath)

	if err != nil {
		return "", "", err
	}

	if strings.TrimSpace(identity) == "" {
		identity = sha
	}

	blobPath := paths.blobFile(identity)

	if _, err := os.Stat(blobPath); err == nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil {
			return "", "", fmt.Errorf("hub: remove temp file: %w", removeErr)
		}

		return blobPath, sha, nil
	}

	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		return "", "", fmt.Errorf("hub: mkdir blob: %w", err)
	}

	if err := os.Rename(tmpPath, blobPath); err != nil {
		return "", "", fmt.Errorf("hub: install blob: %w", err)
	}

	return blobPath, sha, nil
}

func linkSnapshot(blobPath, snapshotPath string) error {
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		return fmt.Errorf("hub: mkdir snapshot: %w", err)
	}

	if _, err := os.Stat(snapshotPath); err == nil {
		return nil
	}

	if err := os.Link(blobPath, snapshotPath); err == nil {
		return nil
	}

	return copyFile(blobPath, snapshotPath)
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)

	if err != nil {
		return fmt.Errorf("hub: open blob: %w", err)
	}

	defer source.Close()

	target, err := os.Create(dst)

	if err != nil {
		return fmt.Errorf("hub: create snapshot: %w", err)
	}

	defer target.Close()

	if _, err := io.Copy(target, source); err != nil {
		return fmt.Errorf("hub: copy snapshot: %w", err)
	}

	return nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)

	if err != nil {
		return "", fmt.Errorf("hub: open %s: %w", path, err)
	}

	defer file.Close()

	hash := sha256.New()

	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hub: sha256 %s: %w", path, err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sanitizeIdentity(identity string) string {
	identity = strings.Trim(identity, "\" ")
	identity = strings.ReplaceAll(identity, "/", "_")
	identity = strings.ReplaceAll(identity, "\\", "_")
	identity = strings.ReplaceAll(identity, ":", "_")

	if identity == "" {
		return "empty"
	}

	return identity
}
