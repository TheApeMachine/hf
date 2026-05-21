package dataset

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOpenJSONLShard_Remote(test *testing.T) {
	Convey("Given a remote JSONL shard", test, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte(`{"text":"streamed"}` + "\n"))
		}))
		defer server.Close()

		rows := collectRows(test, &IterableDataset{
			shards: []shard{{
				url:    server.URL,
				format: formatJSONL,
			}},
		})

		So(rows, ShouldHaveLength, 1)
		So(rows[0]["text"], ShouldEqual, "streamed")
	})
}

func TestLoad_ParquetFilesFromServer(test *testing.T) {
	Convey("Given datasets-server parquet metadata", test, func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/config-names":
				writeJSON(writer, map[string]any{"config_names": []string{"default"}})
			case "/parquet":
				writeJSON(writer, map[string]any{
					"parquet_files": []map[string]any{{
						"dataset":  "org/data",
						"config":   "default",
						"split":    "train",
						"url":      "https://example.com/train.parquet",
						"filename": "train.parquet",
						"size":     123,
					}},
				})
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		datasetServer := newDatasetServer()
		datasetServer.baseURL = server.URL

		files, err := datasetServer.parquetFiles(context.Background(), "org/data", "default", "train", "")

		So(err, ShouldBeNil)
		So(files, ShouldHaveLength, 1)
		So(files[0].URL, ShouldEqual, "https://example.com/train.parquet")
	})
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
