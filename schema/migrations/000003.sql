CREATE TABLE object_detection_outputs (
    video_id STRING(36) NOT NULL,
    model STRING(32) NOT NULL,
    object_count INT64 NOT NULL,
    detected_labels JSON NOT NULL,
    frame_detections JSON,
    frames_sampled INT64 NOT NULL,
    gpu_processing_ms INT64,
    gcs_output_uri STRING(512),
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;
