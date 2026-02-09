package onprem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/media-vault-sync/internal/core/services"
)

type HTTPCloudClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPCloudClient(baseURL string, client *http.Client) *HTTPCloudClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPCloudClient{
		baseURL:    baseURL,
		httpClient: client,
	}
}

func (c *HTTPCloudClient) PostUserAlbums(ctx context.Context, req services.UserAlbumsRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/useralbums", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPCloudClient) PostAlbumManifestUpload(ctx context.Context, req services.AlbumManifestUploadRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/albummanifestupload", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPCloudClient) PostVideoUpload(ctx context.Context, req services.VideoUploadRequest) error {
	url := fmt.Sprintf("%s/v1/album/%s/videoupload", c.baseURL, req.AlbumUID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(req.Data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("X-Provider-ID", req.ProviderID)
	httpReq.Header.Set("X-Database-ID", req.DatabaseID)
	httpReq.Header.Set("X-User-ID", req.UserID)
	httpReq.Header.Set("X-Video-UID", req.VideoUID)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("video not in manifest")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
