package onprem

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
)

type HTTPVideoSender struct {
	receiverURL string
	providerID  string
	httpClient  *http.Client
}

func NewHTTPVideoSender(receiverURL, providerID string, client *http.Client) *HTTPVideoSender {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPVideoSender{
		receiverURL: receiverURL,
		providerID:  providerID,
		httpClient:  client,
	}
}

func (s *HTTPVideoSender) SendVideo(ctx context.Context, databaseID, albumUID, videoUID string, data []byte) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.receiverURL+"/receive-video", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("X-Provider-ID", s.providerID)
	httpReq.Header.Set("X-Database-ID", databaseID)
	httpReq.Header.Set("X-Album-UID", albumUID)
	httpReq.Header.Set("X-Video-UID", videoUID)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
