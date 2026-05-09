ALTER TABLE videos ADD COLUMN request_id STRING(36);

CREATE INDEX idx_videos_user_request_id ON videos(user_id, request_id);
