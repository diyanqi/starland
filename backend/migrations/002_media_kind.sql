-- 拾光星岛 · 照片支持实况/短视频
-- PostgreSQL 13+

ALTER TABLE photos
    ADD COLUMN IF NOT EXISTS kind VARCHAR(8) NOT NULL DEFAULT 'image'
    CHECK (kind IN ('image', 'video'));
