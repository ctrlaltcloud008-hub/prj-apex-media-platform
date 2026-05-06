package profile

import (
	"testing"

	"github.com/ctrlaltcloud008-hub/prj-apex-media-platform/services/ingestion/internal/metadata"
)

func TestSelectTranscodeProfile(t *testing.T) {
	tests := []struct {
		name        string
		video       *metadata.VideoMetadata
		want        string
		wantErr     bool
	}{
		{
			name:    "nil metadata",
			video:   nil,
			wantErr: true,
		},
		{
			name: "invalid dimensions",
			video: &metadata.VideoMetadata{
				Width:  0,
				Height: 1080,
			},
			wantErr: true,
		},
		{
			name: "4k sdr",
			video: &metadata.VideoMetadata{
				Width:  3840,
				Height: 2160,
				FPS:    30,
			},
			want: profile4k,
		},
		{
			name: "1080 hdr",
			video: &metadata.VideoMetadata{
				Width:  1920,
				Height: 1080,
				FPS:    30,
				IsHDR:  true,
			},
			want: profile1080pHDR,
		},
		{
			name: "hdr wins over 60fps when both variants are unavailable together",
			video: &metadata.VideoMetadata{
				Width:  3840,
				Height: 2160,
				FPS:    60,
				IsHDR:  true,
			},
			want: profile4kHDR,
		},
		{
			name: "1080 60fps",
			video: &metadata.VideoMetadata{
				Width:  1920,
				Height: 1080,
				FPS:    59.94,
			},
			want: profile1080p60,
		},
		{
			name: "720 60fps",
			video: &metadata.VideoMetadata{
				Width:  1280,
				Height: 720,
				FPS:    60,
			},
			want: profile720p60,
		},
		{
			name: "480 tier",
			video: &metadata.VideoMetadata{
				Width:  854,
				Height: 450,
				FPS:    30,
			},
			want: profile480p,
		},
		{
			name: "360 tier remains 360 even at high fps",
			video: &metadata.VideoMetadata{
				Width:  640,
				Height: 360,
				FPS:    60,
			},
			want: profile360p,
		},
		{
			name: "portrait video uses short edge",
			video: &metadata.VideoMetadata{
				Width:  720,
				Height: 1280,
				FPS:    30,
			},
			want: profile720p,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectTranscodeProfile(tt.video)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("SelectTranscodeProfile() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("SelectTranscodeProfile() = %q, want %q", got, tt.want)
			}
		})
	}
}
