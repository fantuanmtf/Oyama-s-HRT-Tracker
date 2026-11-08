# Community Chat (preview)

Design for the opt-in community chat / Q&A feature. Default **off**; an admin
enables it from the admin panel. All strings go through `t()`.

## 0. Product constraints

- Privacy-first: chat data is **not** health data. Never attach dose, lab,
  weight, supply, or account details to a post. Only the public nickname,
  timestamps, and post body are stored.
- Pure Markdown, max 1000 chars per post for non-admins; admins unlimited.
- No attachments, no images (Markdown image syntax is not rendered).
- Links are allowed for non-admins only if they are not darknet / crypto /
  phishing patterns; admins are exempt from all content filters.
- Default UI collapses posts into tags (compact preview); expand on click.
- Chat service is a feature flag (`chat_enabled`), default false.

## 1. Data model (migration 0015)

Tables:

- `chat_feature_flags` (key PK, value, updated_at) — `chat_enabled`.
- `public_identities` (user_id PK, nickname UNIQUE, created_at, updated_at).
- `chat_threads` (id PK, author_identity_id, title, body, kind 'chat'|'qa',
  status 'open'|'closed'|'deleted', pinned INTEGER, expires_at, created_at,
  updated_at, review_status 'auto'|'pending'|'approved'|'rejected').
- `chat_posts` (id PK, thread_id FK, author_identity_id, body, parent_post_id,
  review_status, created_at, updated_at, edited_count, last_edited_at).
- `chat_applications` (id PK, user_id FK, status 'pending'|'approved'|'rejected'|'expired',
  created_at, reviewed_at, reviewed_by, note).
- `chat_reports` (id PK, thread_id, post_id, reporter_user_id, reason, created_at,
  status 'open'|'resolved'|'dismissed').
- `chat_restrictions` (id PK, user_id FK, kind 'mute'|'ban', reason_code,
  expires_at, created_at, created_by, note).
- `chat_admins` (user_id PK, granted_by, granted_at) — DB-based admin role,
  capped at 5 incl. env admin.
- `chat_trusted_accounts` (user_id PK, granted_by, granted_at, revoked_at).
- `chat_notifications` (id PK, user_id FK, type, payload_json, read_at, created_at,
  expires_at).
- `chat_audit_log` (id PK, actor_user_id, action, target_type, target_id, detail, created_at).
- `tor_exit_ips` (ip TEXT PK, fetched_at).

FKs ON DELETE CASCADE where user-scoped.

## 2. Eligibility

- Account age >= 30 days AND client-reported HRT days >= 30.
- HRT check is client-reported (server cannot read encrypted dose records),
  same as community vote.
- Users not eligible are read-only (browse, search). No posting/replying/liking.
- Eligible users must submit an application (`chat_applications`) before posting.
- Approved users may post. Tor sessions: every post goes to `pending_review`
  regardless of trusted status (admins exempt).

## 3. Public identity

- Each user configures one nickname (2-24 chars, unique, no profanity basic
  filter). Used for all chat/Q&A posts and likes.
- Posts store `nickname_snapshot` at creation time (immutable per post).
- Rename cooldown 7 days.
- Likes are deduped by user_id but displayed by nickname.

## 4. Trusted accounts (免审)

- Granted manually by an admin. Badge shown.
- Trusted posts skip the review queue but still pass content filters
  (darknet/crypto/CSAM keywords) — filters are safety, not moderation.
- One confirmed violation (admin-marked) permanently revokes trusted status
  (`revoked_at` set, irreversible; one appeal allowed).
- Trusted accounts that trigger anomaly (e.g. >5 posts in 10 min) are
  auto-demoted to review.

## 5. Tor handling

- At login, query the cached Tor exit list (refreshed every 12h from
  https://check.torproject.org/torbulkexitlist). Match → session marked
  `is_tor` (silent, not disclosed to user).
- Tor sessions: all posts `review_status='pending'`, published only after admin
  approval.
- Admins are exempt even over Tor (audited).

## 6. Content filtering (non-admins)

Reject or route to review:
- Darknet addresses: regex `\.(onion|i2p)\b`, `USK@`, Freenet prefixes.
  → reject (400) for non-admins.
- Long unbroken token (>40 chars) with digit/symbol ratio > 60% → review.
- Profanity / CSAM keywords → reject + report.
- Markdown images `![...](...)` → stripped (not rendered).
- Max 1000 chars (source length).

Admins bypass all filters and length cap.

## 7. Restrictions

Kinds: mute (temporary) / ban (permanent). Durations: 30m, 1h, 6h, 12h, 1d,
7d, 30d, permanent. Reason codes: spam, harassment, selfharm, illicit,
crypto, csam, evasion, other. Auto-escalation: 3 mutes → ban. One appeal
allowed per restriction.

## 8. TTL & cleanup

- Chat threads: `expires_at = created_at + 7 days`, swept hourly.
- Q&A threads: no TTL; author can Close (soft-delete after 24h); admins can
  pin/archive (exempt from TTL).
- Notifications: 30 days; read ones 7 days.
- Cleanup runs in `scheduled()` (every hour added to cron) + lazy sweep on read.

## 9. Notifications (SSE)

- Endpoint `GET /api/chat/notifications/stream` returns `text/event-stream`.
- Client reconnects every 25s with `Last-Event-ID`.
- Events: `admin_broadcast`, `reply`, `like`, `review_approved`,
  `review_rejected`, `restriction`, `trusted_granted`, `trusted_revoked`.
- Likes are batched per thread (3+ → single summary event).
- Admin broadcast: max 5/day, target group all/tor/trusted.

## 10. Admin

- Max 5 admins (incl. env admin). Promote/demote routes audited.
- Admin panel: feature flag toggle, review queue, applications, restrictions,
  trusted grants, broadcasts, reports, pinned Q&A.

## 11. API routes (summary)

Public (auth optional for read):
- GET /api/chat/threads?kind=&cursor=&q=
- GET /api/chat/threads/:id
- GET /api/chat/threads/:id/posts

Authenticated user:
- GET/PUT /api/chat/identity
- POST /api/chat/applications
- GET /api/chat/applications/me
- POST /api/chat/threads
- POST /api/chat/threads/:id/posts
- POST /api/chat/threads/:id/close
- POST /api/chat/threads/:id/like
- GET /api/chat/notifications
- GET /api/chat/notifications/stream (SSE)
- POST /api/chat/notifications/read

Admin:
- GET/PUT /api/admin/chat/flag
- GET /api/admin/chat/review
- POST /api/admin/chat/review/:id
- GET /api/admin/chat/applications
- POST /api/admin/chat/applications/:id
- POST /api/admin/chat/restrict
- POST /api/admin/chat/trust
- POST /api/admin/chat/broadcast
- GET/POST /api/admin/chat/admins
- GET /api/admin/chat/reports
- POST /api/admin/chat/reports/:id

## 12. Frontend

- New ViewKey `communityChat`; nav entry (sidebar) + notification bell.
- Pages: CommunityChat (list), ThreadDetail, ChatSettings/Identity, Notifications.
- Markdown render: react-markdown + rehydrate-sanitize + remark-gfm; images disabled.
- Posts collapsed into tag preview by default.
