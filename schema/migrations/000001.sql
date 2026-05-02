CREATE TABLE videos (
    video_id STRING(36) NOT NULL,
    user_id STRING(36) NOT NULL,
    status STRING(20) NOT NULL,
    source_bucket STRING(256),
    source_object STRING(512),
    gcs_generation INT64 NOT NULL,
    mime_type STRING(64),
    file_size_bytes INT64,
    duration_ms INT64,
    source_width INT64,
    source_height INT64,
    source_codec STRING(32),
    source_fps FLOAT64,
    is_hdr BOOL,
    transcode_profile STRING(32),
    transcoder_job_id STRING(256),

    thumbnail_uri STRING(512),
    caption_uri STRING(512),
    moderation_decision STRING(20),

    error_details JSON,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
    updated_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id);


CREATE TABLE saga_events (
    video_id STRING(36) NOT NULL,
    task_type STRING(32) NOT NULL,
    status STRING(20) NOT NULL,
    job_id STRING(256),
    result JSON,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
    updated_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id, task_type),
  INTERLEAVE IN PARENT videos ON DELETE CASCADE;

CREATE TABLE transcode_outputs (
    video_id STRING(36) NOT NULL,
    profile STRING(16) NOT NULL,
    format STRING(8) NOT NULL,
    gcs_uri STRING(512),
    bitrate_kbps INT64,
    status STRING(20) NOT NULL,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id, profile, format),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;

CREATE TABLE transcription_outputs (
    video_id STRING(36) NOT NULL,
    language STRING(8) NOT NULL,
    vtt_gcs_uri STRING(512) NOT NULL,
    srt_gcs_uri STRING(512),
    whisper_model STRING(32) NOT NULL,
    demucs_model STRING(32) NOT NULL,
    word_count INT64,
    segment_count INT64,
    speech_duration_ms INT64,
    avg_confidence FLOAT64,
    source_audio_channels INT64,
    vocal_snr_db FLOAT64,
    gpu_processing_ms INT64,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id, language),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;


CREATE TABLE thumbnail_outputs (
    video_id STRING(36) NOT NULL,
    selected_gcs_uri STRING(512) NOT NULL,
    candidates_gcs_prefix STRING(512),
    candidate_count INT64 NOT NULL,
    selected_frame_ms INT64 NOT NULL,
    selected_yolo_score FLOAT64 NOT NULL,
    scene_count INT64,
    yolo_model STRING(32) NOT NULL,
    agent_used BOOL NOT NULL DEFAULT (false),
    agent_reasoning STRING(2048),
    gpu_processing_ms INT64,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;



CREATE TABLE moderation_outputs (
    video_id STRING(36) NOT NULL,
    decision STRING(20) NOT NULL,
    decision_source STRING(20) NOT NULL,
    max_confidence FLOAT64 NOT NULL,
    flagged_frame_count INT64 NOT NULL,
    total_frame_count INT64 NOT NULL,
    frame_scores JSON NOT NULL,
    label_annotations JSON,
    shot_change_count INT64,
    vi_operation_name STRING(256),
    raw_response_gcs_uri STRING(512),
    
    agent_decision STRING(20),
    agent_confidence FLOAT64,
    agent_reasoning STRING(4098),
    agent_flags JSON,
    agent_escalated BOOL,
    channel_context JSON,
    processing_ms INT64,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;



CREATE TABLE video_lifecycle_events (
    video_id STRING(36) NOT NULL,
    event_seq INT64 NOT NULL,
    from_status STRING(20),
    to_status STRING(20) NOT NULL,
    actor STRING(64) NOT NULL,
    reason STRING(512),
    details JSON,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id, event_seq),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;



CREATE TABLE video_stages (
    video_id STRING(36) NOT NULL,
    stage STRING(20) NOT NULL,
    attempt INT64 NOT NULL,
    started_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
    completed_at TIMESTAMP OPTIONS (allow_commit_timestamp=true),
    duration_ms INT64,
    actor STRING(64) NOT NULL,
    outcome STRING(20),
    error_id STRING(36),
) PRIMARY KEY (video_id, stage, attempt),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;


CREATE TABLE video_failures (
    video_id STRING(36) NOT NULL,
    failure_id STRING(36) NOT NULL,
    stage STRING(20) NOT NULL,
    error_type STRING(20) NOT NULL,
    error_code STRING(20) NOT NULL,
    message STRING(2048) NOT NULL,
    service STRING(64) NOT NULL,
    operation STRING(128),
    retry_count INT64 NOT NULL DEFAULT (0),
    resolved BOOL NOT NULL DEFAULT (false),
    resolved_by STRING(64),
    resolved_at TIMESTAMP OPTIONS (allow_commit_timestamp=true),
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id, failure_id),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;


CREATE TABLE video_recovery_actions (
    video_id STRING(36) NOT NULL,
    action_id STRING(36) NOT NULL,
    sweep_job STRING(64) NOT NULL,
    action_type STRING(20) NOT NULL,
    stage STRING(20),
    details JSON,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id, action_id),
   INTERLEAVE IN PARENT videos ON DELETE CASCADE;


CREATE TABLE video_status_log (
    user_id STRING(36) NOT NULL,
    video_id STRING(36) NOT NULL,
    status STRING(20) NOT NULL,
    message STRING(1024),
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
) PRIMARY KEY (video_id, created_at DESC);


CREATE TABLE outbox (
    entry_id STRING(36) NOT NULL,
    shard_id INT64 NOT NULL,
    video_id STRING(36) NOT NULL,
    topic STRING(128) NOT NULL,
    payload JSON NOT NULL,
    status STRING(16) NOT NULL,
    created_at TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp=true),
    published_at TIMESTAMP,
) PRIMARY KEY (entry_id);


CREATE INDEX idx_videos_user_status ON videos(user_id, status, created_at DESC);
CREATE INDEX idx_videos_status_updated ON videos(status, updated_at DESC);
CREATE INDEX ifx_outbound_pending ON outbox (shard_id, status, created_at);
CREATE INDEX idx_lifecycle_recent ON video_lifecycle_events(created_at DESC);
CREATE INDEX idx_stages_timing ON video_stages(started_at, completed_at DESC) WHERE completed_at IS NOT NULL;
CREATE INDEX idx_failures_unresolved ON video_failures(resolved, created_at DESC);
CREATE INDEX idx_failures_by_code ON video_failures(error_code, created_at DESC);
CREATE INDEX idx_saga_active_tasks ON saga_events(task_type, status);
CREATE INDEX idx_recovery_by_job ON video_recovery_actions(sweep_job, created_at DESC);



ALTER TABLE video_lifecycle_events ADD ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY));
ALTER TABLE video_stages ADD ROW DELETION POLICY (OLDER_THAN(started_at, INTERVAL 365 DAY));
ALTER TABLE video_failures ADD ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY));
ALTER TABLE video_recovery_actions ADD ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 90 DAY));
ALTER TABLE video_status_log ADD ROW DELETION POLICY (OLDER_THAN(created_at, INTERVAL 7 DAY));
