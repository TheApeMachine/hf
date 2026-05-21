package dataset

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	fiberclient "github.com/gofiber/fiber/v3/client"
)

const defaultDatasetServer = "https://datasets-server.huggingface.co"

type datasetServer struct {
	baseURL string
	client  *fiberclient.Client
}

func newDatasetServer() *datasetServer {
	client := fiberclient.New()
	client.SetStreamResponseBody(true)

	return &datasetServer{
		baseURL: defaultDatasetServer,
		client:  client,
	}
}

type parquetFilesResponse struct {
	ParquetFiles []parquetFileEntry `json:"parquet_files"`
	Partial      bool               `json:"partial"`
}

type parquetFileEntry struct {
	Dataset  string `json:"dataset"`
	Config   string `json:"config"`
	Split    string `json:"split"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

type configNamesResponse struct {
	ConfigNames []string `json:"config_names"`
}

type infoResponse struct {
	DatasetInfo datasetInfoPayload `json:"dataset_info"`
}

type datasetInfoPayload struct {
	Features map[string]any `json:"features"`
	Splits   map[string]any `json:"splits"`
}

func (server *datasetServer) parquetFiles(
	ctx context.Context,
	repoID string,
	config string,
	split string,
	token string,
) ([]parquetFileEntry, error) {
	query := url.Values{}
	query.Set("dataset", repoID)
	query.Set("config", config)
	query.Set("split", split)

	requestURL := server.baseURL + "/parquet?" + query.Encode()

	resp, err := server.client.Get(requestURL, fiberclient.Config{
		Ctx:    ctx,
		Header: authHeader(token),
	})

	if err != nil {
		return nil, fmt.Errorf("dataset: parquet files: %w", err)
	}

	defer resp.Close()

	if resp.StatusCode() == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, httpStatusError("dataset: parquet files", resp.StatusCode(), resp.String())
	}

	var payload parquetFilesResponse

	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return nil, fmt.Errorf("dataset: decode parquet files: %w", err)
	}

	return payload.ParquetFiles, nil
}

func (server *datasetServer) configNames(
	ctx context.Context,
	repoID string,
	token string,
) ([]string, error) {
	query := url.Values{}
	query.Set("dataset", repoID)

	requestURL := server.baseURL + "/config-names?" + query.Encode()

	resp, err := server.client.Get(requestURL, fiberclient.Config{
		Ctx:    ctx,
		Header: authHeader(token),
	})

	if err != nil {
		return nil, fmt.Errorf("dataset: config names: %w", err)
	}

	defer resp.Close()

	if resp.StatusCode() != http.StatusOK {
		return nil, httpStatusError("dataset: config names", resp.StatusCode(), resp.String())
	}

	var payload configNamesResponse

	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return nil, fmt.Errorf("dataset: decode config names: %w", err)
	}

	return payload.ConfigNames, nil
}

func (server *datasetServer) features(
	ctx context.Context,
	repoID string,
	config string,
	token string,
) (Features, error) {
	query := url.Values{}
	query.Set("dataset", repoID)
	query.Set("config", config)

	requestURL := server.baseURL + "/info?" + query.Encode()

	resp, err := server.client.Get(requestURL, fiberclient.Config{
		Ctx:    ctx,
		Header: authHeader(token),
	})

	if err != nil {
		return Features{}, fmt.Errorf("dataset: info: %w", err)
	}

	defer resp.Close()

	if resp.StatusCode() != http.StatusOK {
		return Features{}, httpStatusError("dataset: info", resp.StatusCode(), resp.String())
	}

	var payload infoResponse

	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return Features{}, fmt.Errorf("dataset: decode info: %w", err)
	}

	columns := make([]string, 0, len(payload.DatasetInfo.Features))

	for column := range payload.DatasetInfo.Features {
		columns = append(columns, column)
	}

	return Features{Columns: columns}, nil
}

func authHeader(token string) map[string]string {
	token = strings.TrimSpace(token)

	if token == "" {
		return nil
	}

	return map[string]string{
		"Authorization": "Bearer " + token,
	}
}

func httpStatusError(prefix string, status int, body string) error {
	message := strings.TrimSpace(body)

	if message == "" {
		return fmt.Errorf("%s: HTTP %d", prefix, status)
	}

	return fmt.Errorf("%s: HTTP %d: %s", prefix, status, message)
}

func normalizeConfig(config string, available []string) (string, error) {
	config = strings.TrimSpace(config)

	if config != "" {
		return config, nil
	}

	if len(available) == 1 {
		return available[0], nil
	}

	if len(available) == 0 {
		return defaultConfig, nil
	}

	for _, name := range available {
		if name == defaultConfig {
			return defaultConfig, nil
		}
	}

	return "", fmt.Errorf("dataset: config is required; available configs: %s", strings.Join(available, ", "))
}

func readAllLimited(reader io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(reader, limit)
	return io.ReadAll(limited)
}
