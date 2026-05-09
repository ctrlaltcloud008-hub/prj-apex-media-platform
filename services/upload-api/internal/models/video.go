package models

import "time"

type UploadRequest struct {
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

type UploadResponse struct {
	VideoID             string   `json:"video_id"`
	UploadURL           string   `json:"upload_url"`
	UploadURLExpiresAt  string   `json:"upload_url_expires_at"`
	MaxFileSizeBytes    int64    `json:"max_file_size_bytes"`
	AllowedContentTypes []string `json:"allowed_content_types"`
}

type UploadStatusResponse struct {
	VideoID   string    `json:"video_id"`
	Status    string    `json:"status"`
	Progress  float64   `json:"progress"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
