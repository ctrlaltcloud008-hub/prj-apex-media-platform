package domain

import "time"

type CreateUploadRequest struct {
	Filename      string
	ContentType   string
	FileSizeBytes int64
}

type CreateUploadResult struct {
	VideoID            string
	UploadURL          string
	UploadURLExpiresAt time.Time
	MaxFileSizeBytes   int64
}
