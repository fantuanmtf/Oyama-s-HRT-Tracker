-- Community chat / Q&A (preview feature, default off).
--
-- Tables are created here; the Worker also bootstraps them lazily (see
-- communityChatServer.ts) so a deploy that ships before the migration is
-- applied does not 500. Migration remains canonical.

CREATE TABLE IF NOT EXISTS chat_feature_flags (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

INSERT OR IGNORE INTO chat_feature_flags (key, value) VALUES ('chat_enabled', '0');

CREATE TABLE IF NOT EXISTS public_identities (
    user_id TEXT PRIMARY KEY,
    nickname TEXT NOT NULL UNIQUE,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_public_identities_nickname ON public_identities(nickname);

CREATE TABLE IF NOT EXISTS chat_threads (
    id TEXT PRIMARY KEY,
    author_user_id TEXT NOT NULL,
    nickname_snapshot TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('chat','qa')),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed','deleted')),
    review_status TEXT NOT NULL DEFAULT 'approved' CHECK (review_status IN ('auto','pending','approved','rejected')),
    pinned INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    closed_at INTEGER,
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_chat_threads_kind_created ON chat_threads(kind, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_threads_expires ON chat_threads(expires_at);
CREATE INDEX IF NOT EXISTS idx_chat_threads_review ON chat_threads(review_status);

CREATE TABLE IF NOT EXISTS chat_posts (
    id TEXT PRIMARY KEY,
    thread_id TEXT NOT NULL,
    author_user_id TEXT NOT NULL,
    nickname_snapshot TEXT NOT NULL,
    body TEXT NOT NULL,
    parent_post_id TEXT,
    review_status TEXT NOT NULL DEFAULT 'approved' CHECK (review_status IN ('auto','pending','approved','rejected')),
    edited_count INTEGER NOT NULL DEFAULT 0,
    last_edited_at INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (thread_id) REFERENCES chat_threads(id) ON DELETE CASCADE,
    FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_chat_posts_thread ON chat_posts(thread_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chat_posts_review ON chat_posts(review_status);

CREATE TABLE IF NOT EXISTS chat_applications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','expired')),
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    reviewed_at INTEGER,
    reviewed_by TEXT,
    note TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS chat_reports (
    id TEXT PRIMARY KEY,
    thread_id TEXT,
    post_id TEXT,
    reporter_user_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    detail TEXT,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved','dismissed')),
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    resolved_at INTEGER,
    resolved_by TEXT,
    FOREIGN KEY (reporter_user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_chat_reports_status ON chat_reports(status, created_at);

CREATE TABLE IF NOT EXISTS chat_restrictions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('mute','ban')),
    reason_code TEXT NOT NULL,
    expires_at INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    created_by TEXT,
    note TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_chat_restrictions_user ON chat_restrictions(user_id, expires_at);

CREATE TABLE IF NOT EXISTS chat_trusted_accounts (
    user_id TEXT PRIMARY KEY,
    granted_by TEXT,
    granted_at INTEGER NOT NULL DEFAULT (unixepoch()),
    revoked_at INTEGER,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS chat_admins (
    user_id TEXT PRIMARY KEY,
    granted_by TEXT,
    granted_at INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS chat_notifications (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    type TEXT NOT NULL,
    payload_json TEXT NOT NULL DEFAULT '{}',
    read_at INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    expires_at INTEGER NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_chat_notifications_user ON chat_notifications(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_notifications_expires ON chat_notifications(expires_at);

CREATE TABLE IF NOT EXISTS chat_audit_log (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    detail TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX IF NOT EXISTS idx_chat_audit_created ON chat_audit_log(created_at DESC);

CREATE TABLE IF NOT EXISTS tor_exit_ips (
    ip TEXT PRIMARY KEY,
    fetched_at INTEGER NOT NULL
);