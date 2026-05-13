// Package storage provides a client for interacting with Supabase Storage.
// It uses the Supabase Storage REST API to upload and delete files.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a Supabase Storage REST API client.
type Client struct {
	supabaseURL    string
	serviceRoleKey string
	httpClient     *http.Client
}

// New creates a new Supabase Storage Client.
// supabaseURL is the base Supabase project URL (e.g. https://xyz.supabase.co).
// serviceRoleKey is the service role key used for authenticated storage operations.
func New(supabaseURL, serviceRoleKey string) *Client {
	return &Client{
		supabaseURL:    strings.TrimRight(supabaseURL, "/"),
		serviceRoleKey: serviceRoleKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// UploadFile uploads data to the given bucket under the given filename.
// It returns the public URL of the uploaded file.
// contentType should be the MIME type of the file (e.g. "image/jpeg").
func (c *Client) UploadFile(ctx context.Context, bucket, filename string, data []byte, contentType string) (string, error) {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", c.supabaseURL, bucket, filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("storage: create upload request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "false")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage: upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("storage: upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Construct the public URL for the uploaded file.
	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", c.supabaseURL, bucket, filename)
	return publicURL, nil
}

// DeleteFile deletes a file from the given bucket.
func (c *Client) DeleteFile(ctx context.Context, bucket, filename string) error {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", c.supabaseURL, bucket, filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("storage: create delete request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.serviceRoleKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("storage: delete failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ExtractFilenameFromURL extracts the filename (last path segment) from a
// Supabase Storage public URL. Returns an empty string if the URL is malformed.
func ExtractFilenameFromURL(publicURL, bucket string) string {
	// Public URL format: {supabaseURL}/storage/v1/object/public/{bucket}/{filename}
	prefix := "/storage/v1/object/public/" + bucket + "/"
	idx := strings.Index(publicURL, prefix)
	if idx == -1 {
		return ""
	}
	return publicURL[idx+len(prefix):]
}
