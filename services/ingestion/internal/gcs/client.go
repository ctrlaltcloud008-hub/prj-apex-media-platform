package gcs

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
)

type Client struct {
	gc *storage.Client
}

func NewClient(ctx context.Context) (*Client, error) {

	gc, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return &Client{
		gc: gc,
	}, nil
}

func (c *Client) CheckObjectExists(ctx context.Context, bucketName, objectName string, size int64) (bool, error) {

	bucket := c.gc.Bucket(bucketName)
	obj := bucket.Object(objectName)
	attr, err := obj.Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false, nil
		}
		return false, fmt.Errorf("error checking object existence: %w", err)
	}

	if attr.Size != size {
		return false, fmt.Errorf("object size mismatch: expected %d, got %d", size, attr.Size)
	}

	return true, nil
}

func (c *Client) GenerateSignedURL(ctx context.Context, bucketName, objectName string) (string, error) {

	opts := &storage.SignedURLOptions{
		Scheme:  storage.SigningSchemeV4,
		Method:  "GET",
		Expires: time.Now().Add(15 * time.Minute),
	}

	u, err := c.gc.Bucket(bucketName).SignedURL(objectName, opts)
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return u, nil
}

func (c *Client) Close() error {
	return c.gc.Close()
}
