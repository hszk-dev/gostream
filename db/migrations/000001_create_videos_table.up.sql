CREATE TABLE videos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL,
    original_url TEXT,
    hls_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_videos_user_id ON videos(user_id);
CREATE INDEX idx_videos_status ON videos(status);

COMMENT ON TABLE videos IS 'Stores video metadata and processing state';
COMMENT ON COLUMN videos.status IS 'Video processing status: PENDING_UPLOAD, PROCESSING, READY, FAILED';
COMMENT ON COLUMN videos.original_url IS 'Object storage path to the original uploaded video';
COMMENT ON COLUMN videos.hls_url IS 'Object storage path to the HLS manifest (.m3u8) after transcoding';
