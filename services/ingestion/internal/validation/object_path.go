package validation

import (
	"fmt"
	"strings"
)

func ParseObjectPath(objectID string) (string, string, error) {
	parts := strings.SplitN(objectID, "/", 3)
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid object path: %s", objectID)
	}

	userID := parts[0]
	videoID := parts[1]
	filename := parts[2]

	if filename == "" {
		return "", "", fmt.Errorf("filename is empty in object path: %s", objectID)
	}
	if strings.Contains(filename, "/") {
		return "", "", fmt.Errorf("filename contains nested path in object path: %s", objectID)
	}

	if userID == "" || videoID == "" {
		return "", "", fmt.Errorf("userID or videoID is empty in object path: %s", objectID)
	}

	return userID, videoID, nil
}
