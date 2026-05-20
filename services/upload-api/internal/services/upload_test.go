package services

import "testing"

func TestBuildObjectPathUsesActualFileName(t *testing.T) {
	svc := &uploadService{}

	got := svc.buildObjectPath("user-456", "video-123", "my-video.mp4")
	want := "user-456/video-123/my-video.mp4"
	if got != want {
		t.Fatalf("buildObjectPath() = %q, want %q", got, want)
	}
}
