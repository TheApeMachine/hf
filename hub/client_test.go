package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/caramba/pkg/config"
	"github.com/theapemachine/qpool"
)

func TestClient_Download(test *testing.T) {
	Convey("Given a Hub server with one model file", test, func() {
		defer silenceHubProgress()()

		var downloads atomic.Int64
		var repositoryHits atomic.Int64
		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests.Add(1)

			switch request.URL.Path {
			case "/api/models/org/repo/revision/main":
				repositoryHits.Add(1)
				writeJSON(writer, map[string]any{
					"id":  "org/repo",
					"sha": "commit123",
					"siblings": []map[string]any{
						{"rfilename": "config.json", "size": 13},
					},
				})
			case "/org/repo/resolve/main/config.json":
				if request.Method == http.MethodGet {
					downloads.Add(1)
				}

				writer.Header().Set("ETag", `"etag-config"`)
				_, _ = writer.Write([]byte(`{"ok":true}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		cacheDir := test.TempDir()
		client := NewClient(&config.HubConfig{
			Endpoint:   server.URL,
			CacheDir:   cacheDir,
			MaxWorkers: 1,
			Xet:        config.HubXetConfig{Active: true},
		})

		Convey("It should download into a commit-pinned snapshot", func() {
			file, err := client.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Filename: "config.json",
			})

			So(err, ShouldBeNil)
			So(file.Commit, ShouldEqual, "commit123")
			So(file.Cached, ShouldBeFalse)
			So(file.Path, ShouldEqual, filepath.Join(
				cacheDir,
				"models--org--repo",
				"snapshots",
				"commit123",
				"config.json",
			))

			data, err := os.ReadFile(file.Path)

			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `{"ok":true}`)

			infoPath := filepath.Join(
				cacheDir,
				"models--org--repo",
				"info",
				"main",
			)
			infoData, err := os.ReadFile(infoPath)

			So(err, ShouldBeNil)
			So(string(infoData), ShouldContainSubstring, "commit123")
		})

		Convey("It should reuse the snapshot on the second call", func() {
			_, err := client.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Filename: "config.json",
			})
			So(err, ShouldBeNil)

			file, err := client.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Filename: "config.json",
			})

			So(err, ShouldBeNil)
			So(file.Cached, ShouldBeTrue)
			So(downloads.Load(), ShouldEqual, 1)
			So(repositoryHits.Load(), ShouldEqual, 1)
		})

		Convey("It should resolve cached files while offline", func() {
			_, err := client.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Filename: "config.json",
			})
			So(err, ShouldBeNil)

			offlineClient := NewClient(&config.HubConfig{
				Endpoint:   server.URL,
				CacheDir:   cacheDir,
				Offline:    true,
				MaxWorkers: 1,
				Xet:        config.HubXetConfig{Active: true},
			})

			file, err := offlineClient.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Filename: "config.json",
			})

			So(err, ShouldBeNil)
			So(file.Cached, ShouldBeTrue)
			So(file.Commit, ShouldEqual, "commit123")
		})

		Convey("It should reuse a standard Hugging Face info snapshot before network", func() {
			cacheDir := test.TempDir()
			seedHuggingFaceSnapshot(
				test,
				cacheDir,
				ModelRepo,
				"org/repo",
				"main",
				"cachedcommit",
				map[string][]byte{
					"config.json": []byte(`{"cached":true}`),
				},
				true,
			)
			cachedClient := NewClient(&config.HubConfig{
				Endpoint:   server.URL,
				CacheDir:   cacheDir,
				MaxWorkers: 1,
				Xet:        config.HubXetConfig{Active: true},
			})

			file, err := cachedClient.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Filename: "config.json",
			})

			So(err, ShouldBeNil)
			So(file.Cached, ShouldBeTrue)
			So(file.Commit, ShouldEqual, "cachedcommit")
			So(requests.Load(), ShouldEqual, 0)

			data, err := os.ReadFile(file.Path)

			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `{"cached":true}`)
		})

		Convey("It should reuse a commit-pinned snapshot without a ref", func() {
			cacheDir := test.TempDir()
			commit := strings.Repeat("a", 40)
			seedHuggingFaceSnapshot(
				test,
				cacheDir,
				ModelRepo,
				"org/repo",
				commit,
				commit,
				map[string][]byte{
					"config.json": []byte(`{"commit":true}`),
				},
				false,
			)
			cachedClient := NewClient(&config.HubConfig{
				Endpoint:   server.URL,
				CacheDir:   cacheDir,
				MaxWorkers: 1,
				Xet:        config.HubXetConfig{Active: true},
			})

			file, err := cachedClient.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Revision: commit,
				Filename: "config.json",
			})

			So(err, ShouldBeNil)
			So(file.Cached, ShouldBeTrue)
			So(file.Commit, ShouldEqual, commit)
			So(requests.Load(), ShouldEqual, 0)
		})
	})
}

func TestClient_Snapshot(test *testing.T) {
	Convey("Given a Hub repository with several files", test, func() {
		defer silenceHubProgress()()

		var requests atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			requests.Add(1)

			switch {
			case request.URL.Path == "/api/datasets/org/data/revision/main":
				writeJSON(writer, map[string]any{
					"id":  "org/data",
					"sha": "datasetsha",
					"siblings": []map[string]any{
						{"rfilename": "README.md", "size": 5},
						{"rfilename": "data/train.parquet", "size": 7},
						{"rfilename": "data/test.parquet", "size": 6},
					},
				})
			case strings.HasPrefix(request.URL.Path, "/datasets/org/data/resolve/main/"):
				_, _ = writer.Write([]byte(filepath.Base(request.URL.Path)))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		client := NewClient(&config.HubConfig{
			Endpoint:   server.URL,
			CacheDir:   test.TempDir(),
			MaxWorkers: 2,
			Xet:        config.HubXetConfig{Active: true},
		})

		Convey("It should apply include and exclude filters", func() {
			snapshot, err := client.Snapshot(context.Background(), SnapshotRequest{
				RepoID:   "org/data",
				RepoType: DatasetRepo,
				Include:  []string{"**/*.parquet"},
				Exclude:  []string{"*test*"},
			})

			So(err, ShouldBeNil)
			So(snapshot.Commit, ShouldEqual, "datasetsha")
			So(snapshot.Files, ShouldHaveLength, 1)
			So(snapshot.Files[0].Filename, ShouldEqual, "data/train.parquet")
		})

		Convey("It should reuse a complete standard Hugging Face info snapshot before network", func() {
			cacheDir := test.TempDir()
			seedHuggingFaceSnapshot(
				test,
				cacheDir,
				DatasetRepo,
				"org/data",
				"main",
				"cachedatasetsha",
				map[string][]byte{
					"README.md":           []byte("readme"),
					"data/train.parquet":  []byte("train"),
					"data/test.parquet":   []byte("test"),
					"data/unused.parquet": []byte("unused"),
				},
				true,
			)
			cachedClient := NewClient(&config.HubConfig{
				Endpoint:   server.URL,
				CacheDir:   cacheDir,
				MaxWorkers: 2,
				Xet:        config.HubXetConfig{Active: true},
			})

			snapshot, err := cachedClient.Snapshot(context.Background(), SnapshotRequest{
				RepoID:   "org/data",
				RepoType: DatasetRepo,
				Include:  []string{"**/*.parquet"},
				Exclude:  []string{"*test*", "*unused*"},
			})

			So(err, ShouldBeNil)
			So(snapshot.Commit, ShouldEqual, "cachedatasetsha")
			So(snapshot.Files, ShouldHaveLength, 1)
			So(snapshot.Files[0].Filename, ShouldEqual, "data/train.parquet")
			So(snapshot.Files[0].Cached, ShouldBeTrue)
			So(requests.Load(), ShouldEqual, 0)
		})
	})
}

func TestClient_DownloadXet(test *testing.T) {
	Convey("Given a Hub server with a Xet-backed file", test, func() {
		defer silenceHubProgress()()

		fileID := strings.Repeat("a", 64)
		xorbHash := strings.Repeat("b", 64)
		xorbData := append(xorbChunk(0, []byte("hello ")), xorbChunk(0, []byte("world"))...)
		var serverURL string

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/api/models/org/repo/revision/main":
				writeJSON(writer, map[string]any{
					"id":  "org/repo",
					"sha": "xetcommit",
					"siblings": []map[string]any{
						{"rfilename": "model.safetensors", "size": 11},
					},
				})
			case "/org/repo/resolve/main/model.safetensors":
				writer.Header().Set("X-Xet-Hash", fileID)
				writer.Header().Set("Location", serverURL+"/legacy")
				writer.WriteHeader(http.StatusFound)
			case "/api/models/org/repo/xet-read-token/main":
				writeJSON(writer, map[string]any{
					"accessToken": "xet_read",
					"exp":         int64(9999999999),
					"casUrl":      serverURL + "/cas",
				})
			case "/cas/v1/reconstructions/" + fileID:
				if request.Header.Get("Authorization") != "Bearer xet_read" {
					http.Error(writer, "bad authorization", http.StatusUnauthorized)
					return
				}

				writeJSON(writer, map[string]any{
					"offset_into_first_range": 0,
					"terms": []map[string]any{
						{
							"hash":            xorbHash,
							"unpacked_length": 11,
							"range":           map[string]any{"start": 0, "end": 2},
						},
					},
					"fetch_info": map[string]any{
						xorbHash: []map[string]any{
							{
								"range":     map[string]any{"start": 0, "end": 2},
								"url":       serverURL + "/xorb",
								"url_range": map[string]any{"start": 0, "end": len(xorbData) - 1},
							},
						},
					},
				})
			case "/xorb":
				expectedRange := fmt.Sprintf("bytes=0-%d", len(xorbData)-1)

				if request.Header.Get("Range") != expectedRange {
					http.Error(writer, "bad range", http.StatusRequestedRangeNotSatisfiable)
					return
				}

				writer.WriteHeader(http.StatusPartialContent)
				_, _ = writer.Write(xorbData)
			default:
				http.NotFound(writer, request)
			}
		}))
		serverURL = server.URL
		defer server.Close()

		client := NewClient(&config.HubConfig{
			Endpoint:   server.URL,
			CacheDir:   test.TempDir(),
			MaxWorkers: 1,
			Xet:        config.HubXetConfig{Active: true},
		})

		Convey("It should reconstruct the file through CAS", func() {
			file, err := client.Download(context.Background(), DownloadRequest{
				RepoID:   "org/repo",
				RepoType: ModelRepo,
				Filename: "model.safetensors",
			})

			So(err, ShouldBeNil)
			So(file.XetHash, ShouldEqual, fileID)

			data, err := os.ReadFile(file.Path)

			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, "hello world")
		})
	})
}

func BenchmarkClient_DownloadDryRun(benchmark *testing.B) {
	defer silenceHubProgress()()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writeJSON(writer, map[string]any{
			"id":  "org/repo",
			"sha": "commit123",
			"siblings": []map[string]any{
				{"rfilename": "config.json", "size": 13},
			},
		})
	}))
	defer server.Close()

	client := NewClient(&config.HubConfig{
		Endpoint:   server.URL,
		CacheDir:   benchmark.TempDir(),
		MaxWorkers: 1,
		Xet:        config.HubXetConfig{Active: true},
	})

	for benchmark.Loop() {
		_, _ = client.Download(context.Background(), DownloadRequest{
			RepoID:   "org/repo",
			RepoType: ModelRepo,
			Filename: "config.json",
			DryRun:   true,
		})
	}
}

func silenceHubProgress() func() {
	previousPublish := qpool.Publish
	qpool.Publish = func(qpool.Event) {}

	return func() {
		qpool.Publish = previousPublish
	}
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func seedHuggingFaceSnapshot(
	test testing.TB,
	cacheDir string,
	repoType RepoType,
	repoID string,
	revision string,
	commit string,
	files map[string][]byte,
	writeInfo bool,
) {
	test.Helper()

	paths := newCachePaths(cacheDir, repoType, repoID)
	filenames := make([]string, 0, len(files))

	for filename := range files {
		filenames = append(filenames, filename)
	}

	sort.Strings(filenames)

	if writeInfo {
		siblings := make([]siblingPayload, 0, len(filenames))

		for _, filename := range filenames {
			siblings = append(siblings, siblingPayload{
				RFilename: filename,
				Size:      int64(len(files[filename])),
			})
		}

		info := repositoryPayload{
			ID:       repoID,
			SHA:      commit,
			Siblings: siblings,
		}

		if err := os.MkdirAll(filepath.Dir(paths.infoFile(revision)), 0o755); err != nil {
			test.Fatalf("mkdir info: %v", err)
		}

		data, err := json.Marshal(info)

		if err != nil {
			test.Fatalf("marshal info: %v", err)
		}

		if err := os.WriteFile(paths.infoFile(revision), data, 0o644); err != nil {
			test.Fatalf("write info: %v", err)
		}
	}

	for _, filename := range filenames {
		blobName := sanitizeIdentity(commit + "-" + filename)
		blobPath := paths.blobFile(blobName)

		if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
			test.Fatalf("mkdir blob: %v", err)
		}

		if err := os.WriteFile(blobPath, files[filename], 0o644); err != nil {
			test.Fatalf("write blob: %v", err)
		}

		snapshotPath := paths.snapshotFile(commit, filename)

		if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
			test.Fatalf("mkdir snapshot: %v", err)
		}

		target, err := filepath.Rel(filepath.Dir(snapshotPath), blobPath)

		if err == nil {
			err = os.Symlink(target, snapshotPath)
		}

		if err == nil {
			continue
		}

		if err := os.WriteFile(snapshotPath, files[filename], 0o644); err != nil {
			test.Fatalf("write snapshot: %v", err)
		}
	}
}
