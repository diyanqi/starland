-- 拾光星岛 · 初始表结构
-- PostgreSQL 13+

CREATE TABLE IF NOT EXISTS photos (
    id          BIGSERIAL PRIMARY KEY,
    url         TEXT        NOT NULL,
    caption     VARCHAR(200) NOT NULL DEFAULT '',
    nickname    VARCHAR(24)  NOT NULL DEFAULT '匿名星友',
    likes       INTEGER      NOT NULL DEFAULT 0 CHECK (likes >= 0),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS photos_created_at_idx ON photos (created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS messages (
    id          BIGSERIAL PRIMARY KEY,
    content     VARCHAR(2000) NOT NULL CHECK (length(btrim(content)) > 0),
    type        VARCHAR(16)   NOT NULL CHECK (type IN ('praise', 'rant', 'thought')),
    nickname    VARCHAR(24)   NOT NULL DEFAULT '匿名星友',
    created_at  TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS messages_created_at_idx ON messages (created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS messages_type_created_at_idx ON messages (type, created_at DESC);

CREATE TABLE IF NOT EXISTS replies (
    id          BIGSERIAL PRIMARY KEY,
    message_id  BIGINT       NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    content     VARCHAR(1000) NOT NULL CHECK (length(btrim(content)) > 0),
    nickname    VARCHAR(24)  NOT NULL DEFAULT '匿名星友',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS replies_message_idx ON replies (message_id, created_at ASC);

-- 点赞去重：同一浏览器（X-Client-Id）对同一张照片只计一次。
CREATE TABLE IF NOT EXISTS photo_likes (
    photo_id    BIGINT      NOT NULL REFERENCES photos (id) ON DELETE CASCADE,
    client_id   VARCHAR(64) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (photo_id, client_id)
);
