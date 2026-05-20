package validation

import "testing"

func TestParseObjectPath(t *testing.T) {
	tests := []struct {
		name      string
		objectID  string
		wantUser  string
		wantVideo string
		wantErr   bool
	}{
		{name: "source filename object", objectID: "user-456/video-123/source.mp4", wantUser: "user-456", wantVideo: "video-123"},
		{name: "actual filename object", objectID: "user-456/video-123/my-video.mp4", wantUser: "user-456", wantVideo: "video-123"},
		{name: "missing filename", objectID: "user-456/video-123/", wantErr: true},
		{name: "nested filename path", objectID: "user-456/video-123/nested/source.mp4", wantErr: true},
		{name: "missing video id", objectID: "user-456//source.mp4", wantErr: true},
		{name: "not enough parts", objectID: "user-456/video-123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUser, gotVideo, err := ParseObjectPath(tt.objectID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseObjectPath(%q) error = nil, want error", tt.objectID)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseObjectPath(%q) error = %v", tt.objectID, err)
			}
			if gotUser != tt.wantUser {
				t.Fatalf("ParseObjectPath(%q) user = %q, want %q", tt.objectID, gotUser, tt.wantUser)
			}
			if gotVideo != tt.wantVideo {
				t.Fatalf("ParseObjectPath(%q) video = %q, want %q", tt.objectID, gotVideo, tt.wantVideo)
			}
		})
	}
}
