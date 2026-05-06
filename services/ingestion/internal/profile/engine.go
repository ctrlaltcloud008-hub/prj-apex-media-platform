package profile

import (
	"fmt"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/metadata"
)

const (
	profile360p      = "360p"
	profile480p      = "480p"
	profile720p      = "720p"
	profile1080p     = "1080p"
	profile4k        = "4k"
	profile720p60    = "720p-60fps"
	profile1080p60   = "1080p-60fps"
	profile4k60      = "4k-60fps"
	profile1080pHDR  = "1080p-hdr"
	profile4kHDR     = "4k-hdr"
	highFPSThreshold = 50.0
)

func SelectTranscodeProfile(videoMetadata *metadata.VideoMetadata) (string, error) {
	if videoMetadata == nil {
		return "", fmt.Errorf("video metadata is nil")
	}

	if videoMetadata.Width <= 0 || videoMetadata.Height <= 0 {
		return "", fmt.Errorf("video dimensions must be positive")
	}

	baseProfile := selectBaseProfile(videoMetadata.Width, videoMetadata.Height)

	if videoMetadata.IsHDR {
		switch baseProfile {
		case profile4k:
			return profile4kHDR, nil
		case profile1080p:
			return profile1080pHDR, nil
		}
	}

	if videoMetadata.FPS >= highFPSThreshold {
		switch baseProfile {
		case profile4k:
			return profile4k60, nil
		case profile1080p:
			return profile1080p60, nil
		case profile720p:
			return profile720p60, nil
		}
	}

	return baseProfile, nil
}

func selectBaseProfile(width, height int) string {
	shortEdge := min(width, height)

	switch {
	case shortEdge >= 2160:
		return profile4k
	case shortEdge >= 1080:
		return profile1080p
	case shortEdge >= 720:
		return profile720p
	case shortEdge >= 450:
		return profile480p
	default:
		return profile360p
	}
}
