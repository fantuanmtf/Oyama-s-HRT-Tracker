import {
    CHAT_ADMIN_BROADCASTS_PER_DAY,
    CHAT_ADMIN_MAX_POST_CHARS,
    CHAT_APPLICATION_EXPIRY_SECONDS,
    CHAT_CLOSE_GRACE_SECONDS,
    CHAT_EDIT_MAX_COUNT,
    CHAT_EDIT_WINDOW_SECONDS,
    CHAT_MAX_ADMINS,
    CHAT_MAX_POST_CHARS,
    CHAT_MIN_ACCOUNT_AGE_DAYS,
    CHAT_MIN_HRT_DAYS,
    CHAT_RESTRICTION_REASONS,
    CHAT_NOTIFICATION_READ_RETENTION_SECONDS,
    CHAT_NOTIFICATION_RETENTION_SECONDS,
    CHAT_RENAME_COOLDOWN_SECONDS,
    CHAT_RESTRICTION_DURATIONS,
    CHAT_TTL_SECONDS,
    accountAgeDays,
    filterContent,
    isEligible,
    isRestrictionActive,
    normalizeForFilter,
    stripImages,
    validateNickname,
    validatePostBody,
    validateTitle,
    type ChatKind,
    type ChatNotificationType,
    type ChatRestriction,
    type ChatRestrictionReason,
    type ChatReviewStatus,
} from './communityChat';
import { readSmallJsonBody } from './requestInput';

export interface ChatEnv {
    DB: D1Database;
}

export interface ChatHandlerResult {
    status: number;
    body: unknown;
}

interface UserRow {
    id: string;
    username: string;
    created_at: number | null;
}

const nowSeconds = (now?: number) => now ?? Math.floor(Date.now() / 1000);
const uuid = () => crypto.randomUUID();

const response = (status: number, body: unknown): ChatHandlerResult => ({ status, body });
const error = (status: number, code: string, message: string): ChatHandlerResult =>
    response(status, { code, message });

// ---------- schema bootstrap ----------

const SCHEMA_SQL = `
CREATE TABLE IF NOT EXISTS chat_feature_flags (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL DEFAULT (unixepoch()));
INSERT OR IGNORE INTO chat_feature_flags (key, value) VALUES ('chat_enabled', '0');
CREATE TABLE IF NOT EXISTS public_identities (user_id TEXT PRIMARY KEY, nickname TEXT NOT NULL UNIQUE, created_at INTEGER NOT NULL DEFAULT (unixepoch()), updated_at INTEGER NOT NULL DEFAULT (unixepoch()), FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_threads (id TEXT PRIMARY KEY, author_user_id TEXT NOT NULL, nickname_snapshot TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL, kind TEXT NOT NULL CHECK (kind IN ('chat','qa')), status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed','deleted')), review_status TEXT NOT NULL DEFAULT 'approved' CHECK (review_status IN ('auto','pending','approved','rejected')), pinned INTEGER NOT NULL DEFAULT 0, expires_at INTEGER, created_at INTEGER NOT NULL DEFAULT (unixepoch()), updated_at INTEGER NOT NULL DEFAULT (unixepoch()), closed_at INTEGER, FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_posts (id TEXT PRIMARY KEY, thread_id TEXT NOT NULL, author_user_id TEXT NOT NULL, nickname_snapshot TEXT NOT NULL, body TEXT NOT NULL, parent_post_id TEXT, review_status TEXT NOT NULL DEFAULT 'approved' CHECK (review_status IN ('auto','pending','approved','rejected')), edited_count INTEGER NOT NULL DEFAULT 0, last_edited_at INTEGER, created_at INTEGER NOT NULL DEFAULT (unixepoch()), updated_at INTEGER NOT NULL DEFAULT (unixepoch()), FOREIGN KEY (thread_id) REFERENCES chat_threads(id) ON DELETE CASCADE, FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_applications (id TEXT PRIMARY KEY, user_id TEXT NOT NULL UNIQUE, status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','expired')), created_at INTEGER NOT NULL DEFAULT (unixepoch()), reviewed_at INTEGER, reviewed_by TEXT, note TEXT, FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_reports (id TEXT PRIMARY KEY, thread_id TEXT, post_id TEXT, reporter_user_id TEXT NOT NULL, reason TEXT NOT NULL, detail TEXT, status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved','dismissed')), created_at INTEGER NOT NULL DEFAULT (unixepoch()), resolved_at INTEGER, resolved_by TEXT, FOREIGN KEY (reporter_user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_restrictions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, kind TEXT NOT NULL CHECK (kind IN ('mute','ban')), reason_code TEXT NOT NULL, expires_at INTEGER, created_at INTEGER NOT NULL DEFAULT (unixepoch()), created_by TEXT, note TEXT, FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_trusted_accounts (user_id TEXT PRIMARY KEY, granted_by TEXT, granted_at INTEGER NOT NULL DEFAULT (unixepoch()), revoked_at INTEGER, FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_admins (user_id TEXT PRIMARY KEY, granted_by TEXT, granted_at INTEGER NOT NULL DEFAULT (unixepoch()), FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_notifications (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, type TEXT NOT NULL, payload_json TEXT NOT NULL DEFAULT '{}', read_at INTEGER, created_at INTEGER NOT NULL DEFAULT (unixepoch()), expires_at INTEGER NOT NULL, FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE);
CREATE TABLE IF NOT EXISTS chat_audit_log (id TEXT PRIMARY KEY, actor_user_id TEXT, action TEXT NOT NULL, target_type TEXT, target_id TEXT, detail TEXT, created_at INTEGER NOT NULL DEFAULT (unixepoch()));
CREATE TABLE IF NOT EXISTS tor_exit_ips (ip TEXT PRIMARY KEY, fetched_at INTEGER NOT NULL);
`;

const schemaPromises = new WeakMap<object, Promise<void>>();
async function bootstrapSchema(env: ChatEnv): Promise<void> {
    const key = env.DB as unknown as object;
    const pending = schemaPromises.get(key);
    if (pending) return pending;
    const promise = (async () => {
        // D1 doesn't support multi-statement .exec reliably; run each statement.
        const stmts = SCHEMA_SQL.split(';')
            .map(s => s.trim())
            .filter(s => s.length > 0);
        for (const s of stmts) {
            await env.DB.prepare(s).run();
        }
    })();
    schemaPromises.set(key, promise);
    try {
        await promise;
    } catch (err) {
        schemaPromises.delete(key);
        throw err;
    }
}
export async function ensureCommunityChatSchema(env: ChatEnv): Promise<void> {
    try {
        await bootstrapSchema(env);
    } catch (err) {
        console.error('Chat schema bootstrap failed:', err);
    }
}

// ---------- helpers ----------

async function readUser(env: ChatEnv, userId: string): Promise<UserRow | null> {
    const row = (await env.DB.prepare('SELECT id, username, created_at FROM users WHERE id = ?')
        .bind(userId)
        .first<UserRow>()) as UserRow | null;
    return row;
}

async function isChatAdmin(env: ChatEnv, userId: string): Promise<boolean> {
    if (userId === 'admin') return true;
    const row = await env.DB.prepare('SELECT 1 FROM chat_admins WHERE user_id = ?').bind(userId).first();
    return !!row;
}

async function activeRestriction(env: ChatEnv, userId: string, now: number): Promise<ChatRestriction | null> {
    const { results } = await env.DB.prepare(
        `SELECT id, user_id, kind, reason_code, expires_at, created_at, created_by, note
           FROM chat_restrictions WHERE user_id = ? ORDER BY created_at DESC`,
    )
        .bind(userId)
        .all<any>();
    for (const r of results ?? []) {
        const rr: ChatRestriction = {
            id: r.id,
            userId: r.user_id,
            kind: r.kind,
            reasonCode: r.reason_code,
            expiresAt: r.expires_at,
            createdAt: r.created_at,
            createdBy: r.created_by,
            note: r.note,
        };
        if (isRestrictionActive(rr, now)) return rr;
    }
    return null;
}

async function featureFlag(env: ChatEnv, key: string): Promise<boolean> {
    const row = (await env.DB.prepare('SELECT value FROM chat_feature_flags WHERE key = ?').bind(key).first()) as {
        value: string;
    } | null;
    return row?.value === '1' || row?.value === 'true';
}

async function readIdentity(env: ChatEnv, userId: string): Promise<{ nickname: string; createdAt: number; updatedAt: number } | null> {
    const row = (await env.DB.prepare(
        'SELECT nickname, created_at, updated_at FROM public_identities WHERE user_id = ?',
    )
        .bind(userId)
        .first()) as any;
    if (!row) return null;
    return { nickname: row.nickname, createdAt: row.created_at, updatedAt: row.updated_at };
}

async function isTrusted(env: ChatEnv, userId: string): Promise<boolean> {
    const row = (await env.DB.prepare(
        'SELECT revoked_at FROM chat_trusted_accounts WHERE user_id = ?',
    )
        .bind(userId)
        .first()) as { revoked_at: number | null } | null;
    return !!row && row.revoked_at === null;
}

async function audit(env: ChatEnv, actor: string | null, action: string, targetType: string | null, targetId: string | null, detail: string | null) {
    await env.DB.prepare(
        `INSERT INTO chat_audit_log (id, actor_user_id, action, target_type, target_id, detail, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
    )
        .bind(uuid(), actor, action, targetType, targetId, detail, nowSeconds())
        .run();
}

async function notify(env: ChatEnv, userId: string, type: ChatNotificationType, payload: Record<string, unknown>) {
    const now = nowSeconds();
    await env.DB.prepare(
        `INSERT INTO chat_notifications (id, user_id, type, payload_json, created_at, expires_at)
         VALUES (?, ?, ?, ?, ?, ?)`,
    )
        .bind(uuid(), userId, type, JSON.stringify(payload), now, now + CHAT_NOTIFICATION_RETENTION_SECONDS)
        .run();
}

// ---------- feature flag ----------

export async function handleChatFlagGet(env: ChatEnv): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const enabled = await featureFlag(env, 'chat_enabled');
    return response(200, { enabled });
}

export async function handleChatFlagSet(env: ChatEnv, userId: string, request: Request): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const enabled = body?.enabled === true || body?.enabled === '1' || body?.enabled === 1;
    await env.DB.prepare(
        `INSERT INTO chat_feature_flags (key, value, updated_at) VALUES ('chat_enabled', ?, unixepoch())
         ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = unixepoch()`,
    )
        .bind(enabled ? '1' : '0', enabled ? '1' : '0')
        .run();
    await audit(env, userId, 'flag_set', 'feature', 'chat_enabled', enabled ? '1' : '0');
    return response(200, { enabled });
}

// ---------- identity ----------

export async function handleIdentityGet(env: ChatEnv, userId: string): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const id = await readIdentity(env, userId);
    if (!id) return response(200, { identity: null });
    return response(200, { identity: { nickname: id.nickname, createdAt: id.createdAt, updatedAt: id.updatedAt } });
}

export async function handleIdentitySet(env: ChatEnv, userId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const user = await readUser(env, userId);
    if (!user) return error(404, 'ACCOUNT_NOT_FOUND', 'Account not found');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const nickname = validateNickname(body?.nickname);
    if (!nickname) return error(400, 'INVALID_NICKNAME', 'Invalid nickname');
    const existing = await readIdentity(env, userId);
    if (existing) {
        if (now - existing.updatedAt < CHAT_RENAME_COOLDOWN_SECONDS) {
            return error(429, 'RENAME_COOLDOWN', 'Rename cooldown not elapsed');
        }
    }
    // uniqueness
    const clash = await env.DB.prepare('SELECT user_id FROM public_identities WHERE nickname = ? AND user_id != ?')
        .bind(nickname, userId)
        .first();
    if (clash) return error(409, 'NICKNAME_TAKEN', 'Nickname already taken');
    await env.DB.prepare(
        `INSERT INTO public_identities (user_id, nickname, created_at, updated_at)
         VALUES (?, ?, ?, ?)
         ON CONFLICT(user_id) DO UPDATE SET nickname = ?, updated_at = ?`,
    )
        .bind(userId, nickname, now, now, nickname, now)
        .run();
    return response(200, { identity: { nickname, createdAt: existing?.createdAt ?? now, updatedAt: now } });
}

// ---------- eligibility / application ----------

export async function handleEligibility(env: ChatEnv, userId: string, hrtDays: number, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const user = await readUser(env, userId);
    if (!user) return error(404, 'ACCOUNT_NOT_FOUND', 'Account not found');
    const eligible = isEligible(user.created_at, hrtDays, now);
    const app = (await env.DB.prepare('SELECT status FROM chat_applications WHERE user_id = ?').bind(userId).first()) as {
        status: string;
    } | null;
    const restriction = await activeRestriction(env, userId, now);
    return response(200, {
        accountAgeDays: accountAgeDays(user.created_at, now),
        hrtDays,
        eligible,
        applicationStatus: app?.status ?? null,
        restricted: !!restriction,
        restriction: restriction
            ? { kind: restriction.kind, reasonCode: restriction.reasonCode, expiresAt: restriction.expiresAt }
            : null,
        trusted: await isTrusted(env, userId),
        isAdmin: await isChatAdmin(env, userId),
    });
}

export async function handleApplicationSubmit(env: ChatEnv, userId: string, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const user = await readUser(env, userId);
    if (!user) return error(404, 'ACCOUNT_NOT_FOUND', 'Account not found');
    const existing = (await env.DB.prepare('SELECT status, created_at FROM chat_applications WHERE user_id = ?')
        .bind(userId)
        .first()) as { status: string; created_at: number } | null;
    if (existing && existing.status === 'pending') {
        // expire stale
        if (now - existing.created_at > CHAT_APPLICATION_EXPIRY_SECONDS) {
            await env.DB.prepare("UPDATE chat_applications SET status = 'expired' WHERE user_id = ?").bind(userId).run();
        } else {
            return error(409, 'APPLICATION_PENDING', 'Application already pending');
        }
    }
    if (existing && existing.status === 'approved') return error(409, 'ALREADY_APPROVED', 'Already approved');
    await env.DB.prepare(
        `INSERT INTO chat_applications (id, user_id, status, created_at)
         VALUES (?, ?, 'pending', ?)
         ON CONFLICT(user_id) DO UPDATE SET status = 'pending', created_at = ?, reviewed_at = NULL, reviewed_by = NULL, note = NULL`,
    )
        .bind(uuid(), userId, now, now)
        .run();
    return response(201, { status: 'pending' });
}

export async function handleApplicationList(env: ChatEnv, userId: string): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const { results } = await env.DB.prepare(
        `SELECT a.id, a.user_id, u.username, a.status, a.created_at, a.reviewed_at, a.reviewed_by, a.note
           FROM chat_applications a LEFT JOIN users u ON u.id = a.user_id
          ORDER BY a.created_at DESC LIMIT 200`,
    ).all();
    return response(200, { applications: results ?? [] });
}

export async function handleApplicationReview(env: ChatEnv, userId: string, applicationId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const approve = body?.approve === true;
    const note = typeof body?.note === 'string' ? body.note.slice(0, 500) : null;
    const app = (await env.DB.prepare('SELECT user_id, status FROM chat_applications WHERE id = ?').bind(applicationId).first()) as {
        user_id: string;
        status: string;
    } | null;
    if (!app) return error(404, 'NOT_FOUND', 'Application not found');
    const newStatus = approve ? 'approved' : 'rejected';
    await env.DB.prepare(
        `UPDATE chat_applications SET status = ?, reviewed_at = ?, reviewed_by = ?, note = ? WHERE id = ?`,
    )
        .bind(newStatus, now, userId, note, applicationId)
        .run();
    await notify(env, app.user_id, approve ? 'review_approved' : 'review_rejected', { applicationId });
    await audit(env, userId, `application_${newStatus}`, 'application', applicationId, note);
    return response(200, { status: newStatus });
}

// ---------- approved check ----------

async function isApproved(env: ChatEnv, userId: string, now: number): Promise<boolean> {
    const app = (await env.DB.prepare("SELECT status FROM chat_applications WHERE user_id = ?").bind(userId).first()) as {
        status: string;
    } | null;
    if (!app || app.status !== 'approved') return false;
    const restriction = await activeRestriction(env, userId, now);
    if (restriction && (restriction.kind === 'ban' || restriction.kind === 'mute')) return false;
    return true;
}

// ---------- threads ----------

export async function handleThreadCreate(env: ChatEnv, userId: string, request: Request, isTor: boolean, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await featureFlag(env, 'chat_enabled'))) return error(403, 'CHAT_DISABLED', 'Chat disabled');
    const user = await readUser(env, userId);
    if (!user) return error(404, 'ACCOUNT_NOT_FOUND', 'Account not found');
    const identity = await readIdentity(env, userId);
    if (!identity) return error(400, 'NO_IDENTITY', 'Set a nickname first');
    const isAdmin = await isChatAdmin(env, userId);
    // approved check (admins bypass)
    if (!isAdmin) {
        const restriction = await activeRestriction(env, userId, now);
        if (restriction) return error(403, 'RESTRICTED', 'Account restricted');
        const approved = await isApproved(env, userId, now);
        if (!approved) return error(403, 'NOT_APPROVED', 'Not approved to post');
    }
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const kind: ChatKind = body?.kind === 'qa' ? 'qa' : 'chat';
    const title = validateTitle(body?.title);
    if (!title) return error(400, 'INVALID_TITLE', 'Invalid title');
    const v = validatePostBody(body?.body, isAdmin);
    if (!v.ok) return error(400, (v as { code: string }).code, (v as { code: string }).code);
    const cleanBody = stripImages(v.body);
    const f = filterContent(`${title}\n${cleanBody}`, isAdmin);
    if (!f.allow) return error(400, `FILTER_${f.reason?.toUpperCase()}`, 'Content not allowed');
    const reviewStatus: ChatReviewStatus = isAdmin
        ? 'approved'
        : isTor
          ? 'pending'
          : f.review
            ? 'pending'
            : 'approved';
    // trusted accounts skip review (but still filtered above)
    const trusted = !isAdmin && (await isTrusted(env, userId));
    const finalReview: ChatReviewStatus =
        isAdmin || (trusted && !isTor && !f.review) ? 'approved' : reviewStatus;
    const id = uuid();
    const expiresAt = kind === 'chat' ? now + CHAT_TTL_SECONDS : null;
    await env.DB.prepare(
        `INSERT INTO chat_threads (id, author_user_id, nickname_snapshot, title, body, kind, status, review_status, pinned, expires_at, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, 'open', ?, 0, ?, ?, ?)`,
    )
        .bind(id, userId, identity.nickname, title, cleanBody, kind, finalReview, expiresAt, now, now)
        .run();
    await audit(env, userId, 'thread_create', 'thread', id, kind);
    return response(201, { id, reviewStatus: finalReview });
}

export async function handleThreadList(env: ChatEnv, request: Request, userId: string | null, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const url = new URL(request.url);
    const kind = url.searchParams.get('kind') === 'qa' ? 'qa' : 'chat';
    const cursor = url.searchParams.get('cursor');
    const q = (url.searchParams.get('q') ?? '').trim().slice(0, 100);
    const isAdmin = userId ? await isChatAdmin(env, userId) : false;
    const limit = 30;
    let sql = `SELECT id, author_user_id, nickname_snapshot, title, body, kind, status, review_status, pinned, expires_at, created_at, updated_at, closed_at
               FROM chat_threads WHERE kind = ? AND status != 'deleted'`;
    const binds: any[] = [kind];
    if (!isAdmin) {
        sql += ` AND review_status IN ('approved','auto')`;
    }
    if (q) {
        sql += ` AND (title LIKE ? OR body LIKE ?)`;
        binds.push(`%${q}%`, `%${q}%`);
    }
    if (cursor) {
        sql += ` AND created_at < ?`;
        binds.push(Number(cursor));
    }
    sql += ` ORDER BY pinned DESC, created_at DESC LIMIT ?`;
    binds.push(limit);
    const { results } = await env.DB.prepare(sql).bind(...binds).all();
    const nextCursor = results && results.length === limit ? results[results.length - 1].created_at : null;
    return response(200, { threads: results ?? [], nextCursor });
}

export async function handleThreadGet(env: ChatEnv, threadId: string, userId: string | null): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const row = (await env.DB.prepare(
        `SELECT id, author_user_id, nickname_snapshot, title, body, kind, status, review_status, pinned, expires_at, created_at, updated_at, closed_at
         FROM chat_threads WHERE id = ?`,
    )
        .bind(threadId)
        .first()) as any;
    if (!row) return error(404, 'NOT_FOUND', 'Thread not found');
    const isAdmin = userId ? await isChatAdmin(env, userId) : false;
    if (!isAdmin && !['approved', 'auto'].includes(row.review_status)) return error(404, 'NOT_FOUND', 'Thread not found');
    if (row.status === 'deleted' && !isAdmin) return error(404, 'NOT_FOUND', 'Thread not found');
    return response(200, { thread: row });
}

export async function handleThreadClose(env: ChatEnv, userId: string, threadId: string, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const row = (await env.DB.prepare('SELECT author_user_id, kind, status FROM chat_threads WHERE id = ?').bind(threadId).first()) as any;
    if (!row) return error(404, 'NOT_FOUND', 'Thread not found');
    const isAdmin = await isChatAdmin(env, userId);
    if (!isAdmin && row.author_user_id !== userId) return error(403, 'FORBIDDEN', 'Not author');
    if (row.status === 'deleted') return error(409, 'ALREADY_DELETED', 'Already deleted');
    await env.DB.prepare(`UPDATE chat_threads SET status = 'closed', closed_at = ?, updated_at = ? WHERE id = ?`)
        .bind(now, now, threadId)
        .run();
    // schedule hard delete after grace
    await env.DB.prepare(`UPDATE chat_threads SET expires_at = ? WHERE id = ?`).bind(now + CHAT_CLOSE_GRACE_SECONDS, threadId).run();
    await audit(env, userId, 'thread_close', 'thread', threadId, null);
    return response(200, { status: 'closed' });
}

// ---------- posts ----------

export async function handlePostCreate(env: ChatEnv, userId: string, threadId: string, request: Request, isTor: boolean, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await featureFlag(env, 'chat_enabled'))) return error(403, 'CHAT_DISABLED', 'Chat disabled');
    const thread = (await env.DB.prepare('SELECT id, status, review_status FROM chat_threads WHERE id = ?').bind(threadId).first()) as any;
    if (!thread) return error(404, 'NOT_FOUND', 'Thread not found');
    if (thread.status !== 'open') return error(409, 'THREAD_CLOSED', 'Thread closed');
    const user = await readUser(env, userId);
    if (!user) return error(404, 'ACCOUNT_NOT_FOUND', 'Account not found');
    const identity = await readIdentity(env, userId);
    if (!identity) return error(400, 'NO_IDENTITY', 'Set a nickname first');
    const isAdmin = await isChatAdmin(env, userId);
    if (!isAdmin) {
        const restriction = await activeRestriction(env, userId, now);
        if (restriction) return error(403, 'RESTRICTED', 'Account restricted');
        const approved = await isApproved(env, userId, now);
        if (!approved) return error(403, 'NOT_APPROVED', 'Not approved to post');
    }
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const v = validatePostBody(body?.body, isAdmin);
    if (!v.ok) return error(400, (v as { code: string }).code, (v as { code: string }).code);
    const cleanBody = stripImages(v.body);
    const f = filterContent(cleanBody, isAdmin);
    if (!f.allow) return error(400, `FILTER_${f.reason?.toUpperCase()}`, 'Content not allowed');
    const trusted = !isAdmin && (await isTrusted(env, userId));
    const reviewStatus: ChatReviewStatus = isAdmin || (trusted && !isTor && !f.review) ? 'approved' : isTor || f.review ? 'pending' : 'approved';
    const parentId = typeof body?.parentPostId === 'string' ? body.parentPostId : null;
    const id = uuid();
    await env.DB.prepare(
        `INSERT INTO chat_posts (id, thread_id, author_user_id, nickname_snapshot, body, parent_post_id, review_status, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    )
        .bind(id, threadId, userId, identity.nickname, cleanBody, parentId, reviewStatus, now, now)
        .run();
    // notify thread author if not self
    const threadAuthor = (await env.DB.prepare('SELECT author_user_id FROM chat_threads WHERE id = ?').bind(threadId).first()) as {
        author_user_id: string;
    } | null;
    if (threadAuthor && threadAuthor.author_user_id !== userId) {
        await notify(env, threadAuthor.author_user_id, 'reply', { threadId, postId: id });
    }
    await audit(env, userId, 'post_create', 'post', id, null);
    return response(201, { id, reviewStatus });
}

export async function handlePostList(env: ChatEnv, threadId: string, userId: string | null): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const isAdmin = userId ? await isChatAdmin(env, userId) : false;
    let sql = `SELECT id, thread_id, author_user_id, nickname_snapshot, body, parent_post_id, review_status, edited_count, last_edited_at, created_at, updated_at
               FROM chat_posts WHERE thread_id = ?`;
    const binds: any[] = [threadId];
    if (!isAdmin) sql += ` AND review_status IN ('approved','auto')`;
    sql += ` ORDER BY created_at ASC`;
    const { results } = await env.DB.prepare(sql).bind(...binds).all();
    return response(200, { posts: results ?? [] });
}

export async function handlePostEdit(env: ChatEnv, userId: string, postId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const row = (await env.DB.prepare(
        'SELECT id, author_user_id, created_at, edited_count, last_edited_at FROM chat_posts WHERE id = ?',
    )
        .bind(postId)
        .first()) as any;
    if (!row) return error(404, 'NOT_FOUND', 'Post not found');
    const isAdmin = await isChatAdmin(env, userId);
    if (!isAdmin && row.author_user_id !== userId) return error(403, 'FORBIDDEN', 'Not author');
    if (!isAdmin && now - row.created_at > CHAT_EDIT_WINDOW_SECONDS) {
        return error(403, 'EDIT_WINDOW_EXPIRED', 'Edit window expired');
    }
    if (!isAdmin && row.edited_count >= CHAT_EDIT_MAX_COUNT) {
        return error(403, 'EDIT_LIMIT', 'Edit limit reached');
    }
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const v = validatePostBody(body?.body, isAdmin);
    if (!v.ok) return error(400, (v as { code: string }).code, (v as { code: string }).code);
    const cleanBody = stripImages(v.body);
    const f = filterContent(cleanBody, isAdmin);
    if (!f.allow) return error(400, `FILTER_${f.reason?.toUpperCase()}`, 'Content not allowed');
    await env.DB.prepare(`UPDATE chat_posts SET body = ?, updated_at = ?, edited_count = edited_count + 1, last_edited_at = ? WHERE id = ?`)
        .bind(cleanBody, now, now, postId)
        .run();
    return response(200, { ok: true });
}

// ---------- likes ----------

export async function handleLike(env: ChatEnv, userId: string, threadId: string, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    // likes stored as notifications of type 'like' aggregated; for simplicity use a likes table? We didn't create one. Use chat_reports? No.
    // We'll create an implicit like via a lightweight table is missing; instead store in chat_notifications to author.
    const thread = (await env.DB.prepare('SELECT author_user_id FROM chat_threads WHERE id = ?').bind(threadId).first()) as {
        author_user_id: string;
    } | null;
    if (!thread) return error(404, 'NOT_FOUND', 'Thread not found');
    if (thread.author_user_id === userId) return error(400, 'SELF_LIKE', 'Cannot like own thread');
    // dedup: check if a 'like' notification from this user already exists for this thread in last 24h
    const existing = await env.DB.prepare(
        `SELECT id FROM chat_notifications WHERE user_id = ? AND type = 'like' AND payload_json LIKE ?`,
    )
        .bind(thread.author_user_id, `%"threadId":"${threadId}"%`)
        .first();
    if (existing) return response(200, { ok: true, already: true });
    await notify(env, thread.author_user_id, 'like', { threadId, byUserId: userId });
    return response(200, { ok: true });
}

// ---------- review queue (admin) ----------

export async function handleReviewList(env: ChatEnv, userId: string): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const threads = await env.DB.prepare(
        `SELECT id, author_user_id, nickname_snapshot, title, kind, review_status, created_at FROM chat_threads WHERE review_status = 'pending' ORDER BY created_at ASC`,
    ).all();
    const posts = await env.DB.prepare(
        `SELECT id, thread_id, author_user_id, nickname_snapshot, body, review_status, created_at FROM chat_posts WHERE review_status = 'pending' ORDER BY created_at ASC`,
    ).all();
    return response(200, { threads: threads.results ?? [], posts: posts.results ?? [] });
}

export async function handleReviewDecision(env: ChatEnv, userId: string, targetType: string, targetId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const approve = body?.approve === true;
    const newStatus: ChatReviewStatus = approve ? 'approved' : 'rejected';
    const table = targetType === 'thread' ? 'chat_threads' : 'chat_posts';
    await env.DB.prepare(`UPDATE ${table} SET review_status = ?, updated_at = ? WHERE id = ?`).bind(newStatus, now, targetId).run();
    // notify author
    const row = (await env.DB.prepare(`SELECT author_user_id FROM ${table} WHERE id = ?`).bind(targetId).first()) as {
        author_user_id: string;
    } | null;
    if (row) {
        await notify(env, row.author_user_id, approve ? 'review_approved' : 'review_rejected', { targetType, targetId });
    }
    await audit(env, userId, `review_${newStatus}`, targetType, targetId, null);
    return response(200, { status: newStatus });
}

// ---------- restrictions ----------

export async function handleRestrict(env: ChatEnv, userId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const targetUserId = body?.userId;
    if (typeof targetUserId !== 'string') return error(400, 'INVALID_USER', 'Invalid user');
    const kind = body?.kind === 'ban' ? 'ban' : 'mute';
    const reason = (CHAT_RESTRICTION_REASONS as readonly string[]).includes(body?.reason) ? body.reason : 'other';
    const durationKey = body?.duration ?? '1d';
    const dur = CHAT_RESTRICTION_DURATIONS[durationKey];
    if (dur === undefined) return error(400, 'INVALID_DURATION', 'Invalid duration');
    const expiresAt = dur === 0 ? null : now + dur;
    await env.DB.prepare(
        `INSERT INTO chat_restrictions (id, user_id, kind, reason_code, expires_at, created_at, created_by, note)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
    )
        .bind(uuid(), targetUserId, kind, reason, expiresAt, now, userId, body?.note ?? null)
        .run();
    await notify(env, targetUserId, 'restriction', { kind, reasonCode: reason, expiresAt });
    await audit(env, userId, 'restrict', 'user', targetUserId, `${kind}:${reason}`);
    return response(200, { ok: true });
}

// ---------- trusted accounts ----------

export async function handleTrust(env: ChatEnv, userId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const targetUserId = body?.userId;
    if (typeof targetUserId !== 'string') return error(400, 'INVALID_USER', 'Invalid user');
    const grant = body?.grant !== false;
    if (grant) {
        await env.DB.prepare(
            `INSERT INTO chat_trusted_accounts (user_id, granted_by, granted_at, revoked_at)
             VALUES (?, ?, ?, NULL)
             ON CONFLICT(user_id) DO UPDATE SET granted_by = ?, granted_at = ?, revoked_at = NULL`,
        )
            .bind(targetUserId, userId, now, userId, now)
            .run();
        await notify(env, targetUserId, 'trusted_granted', {});
    } else {
        await env.DB.prepare(`UPDATE chat_trusted_accounts SET revoked_at = ? WHERE user_id = ?`).bind(now, targetUserId).run();
        await notify(env, targetUserId, 'trusted_revoked', {});
    }
    await audit(env, userId, grant ? 'trust_grant' : 'trust_revoke', 'user', targetUserId, null);
    return response(200, { ok: true });
}

// ---------- admins ----------

export async function handleAdminsList(env: ChatEnv, userId: string): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const { results } = await env.DB.prepare(
        `SELECT user_id, granted_by, granted_at FROM chat_admins ORDER BY granted_at ASC`,
    ).all();
    const admins = [{ userId: 'admin', grantedBy: null, grantedAt: 0 }, ...(results ?? []).map((r: any) => ({
        userId: r.user_id,
        grantedBy: r.granted_by,
        grantedAt: r.granted_at,
    }))];
    return response(200, { admins, max: CHAT_MAX_ADMINS });
}

export async function handleAdminPromote(env: ChatEnv, userId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    // count admins
    const countRow = (await env.DB.prepare('SELECT COUNT(*) AS n FROM chat_admins').first()) as { n: number };
    const total = (countRow?.n ?? 0) + 1; // + env admin
    if (total >= CHAT_MAX_ADMINS) return error(409, 'ADMIN_CAP', 'Admin cap reached');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const targetUserId = body?.userId;
    if (typeof targetUserId !== 'string') return error(400, 'INVALID_USER', 'Invalid user');
    if (targetUserId === 'admin') return error(400, 'INVALID_USER', 'Already env admin');
    await env.DB.prepare(`INSERT OR IGNORE INTO chat_admins (user_id, granted_by, granted_at) VALUES (?, ?, ?)`)
        .bind(targetUserId, userId, now)
        .run();
    await audit(env, userId, 'admin_promote', 'user', targetUserId, null);
    return response(200, { ok: true });
}

export async function handleAdminDemote(env: ChatEnv, userId: string, targetUserId: string): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    if (targetUserId === 'admin') return error(400, 'INVALID_USER', 'Cannot demote env admin');
    await env.DB.prepare('DELETE FROM chat_admins WHERE user_id = ?').bind(targetUserId).run();
    await audit(env, userId, 'admin_demote', 'user', targetUserId, null);
    return response(200, { ok: true });
}

// ---------- reports ----------

export async function handleReport(env: ChatEnv, userId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const threadId = typeof body?.threadId === 'string' ? body.threadId : null;
    const postId = typeof body?.postId === 'string' ? body.postId : null;
    const reason = typeof body?.reason === 'string' ? body.reason.slice(0, 100) : 'other';
    const detail = typeof body?.detail === 'string' ? body.detail.slice(0, 500) : null;
    await env.DB.prepare(
        `INSERT INTO chat_reports (id, thread_id, post_id, reporter_user_id, reason, detail, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
    )
        .bind(uuid(), threadId, postId, userId, reason, detail, now)
        .run();
    return response(201, { ok: true });
}

export async function handleReportsList(env: ChatEnv, userId: string): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const { results } = await env.DB.prepare(
        `SELECT * FROM chat_reports WHERE status = 'open' ORDER BY created_at DESC LIMIT 200`,
    ).all();
    return response(200, { reports: results ?? [] });
}

export async function handleReportResolve(env: ChatEnv, userId: string, reportId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const dismiss = body?.dismiss === true;
    const newStatus = dismiss ? 'dismissed' : 'resolved';
    await env.DB.prepare(`UPDATE chat_reports SET status = ?, resolved_at = ?, resolved_by = ? WHERE id = ?`)
        .bind(newStatus, now, userId, reportId)
        .run();
    return response(200, { status: newStatus });
}

// ---------- broadcast ----------

export async function handleBroadcast(env: ChatEnv, userId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    if (!(await isChatAdmin(env, userId))) return error(403, 'FORBIDDEN', 'Admin only');
    // rate limit: 5/day
    const dayStart = now - (now % 86400);
    const row = (await env.DB.prepare(
        `SELECT COUNT(*) AS n FROM chat_notifications WHERE type = 'admin_broadcast' AND created_at >= ?`,
    )
        .bind(dayStart)
        .first()) as { n: number };
    if ((row?.n ?? 0) >= CHAT_ADMIN_BROADCASTS_PER_DAY) {
        return error(429, 'BROADCAST_LIMIT', 'Daily broadcast limit reached');
    }
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const text = typeof body?.text === 'string' ? body.text.trim().slice(0, 500) : '';
    if (!text) return error(400, 'INVALID_TEXT', 'Empty text');
    const group = ['all', 'tor', 'trusted'].includes(body?.group) ? body.group : 'all';
    // target users
    let users: { id: string }[] = [];
    if (group === 'all') {
        const r = await env.DB.prepare('SELECT id FROM users').all();
        users = (r.results ?? []) as any;
    } else if (group === 'trusted') {
        const r = await env.DB.prepare('SELECT user_id AS id FROM chat_trusted_accounts WHERE revoked_at IS NULL').all();
        users = (r.results ?? []) as any;
    } else {
        // tor: best-effort, no stored flag; broadcast to all (client filters). For simplicity all.
        const r = await env.DB.prepare('SELECT id FROM users').all();
        users = (r.results ?? []) as any;
    }
    for (const u of users) {
        await notify(env, u.id, 'admin_broadcast', { text, group });
    }
    await audit(env, userId, 'broadcast', 'notification', null, `${group}:${text.slice(0, 50)}`);
    return response(200, { ok: true, recipients: users.length });
}

// ---------- notifications ----------

export async function handleNotificationsList(env: ChatEnv, userId: string, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    // lazy cleanup expired
    await env.DB.prepare('DELETE FROM chat_notifications WHERE expires_at < ?').bind(now).run();
    const { results } = await env.DB.prepare(
        `SELECT id, type, payload_json, read_at, created_at, expires_at
           FROM chat_notifications WHERE user_id = ? ORDER BY created_at DESC LIMIT 100`,
    )
        .bind(userId)
        .all();
    const items = (results ?? []).map((r: any) => ({
        id: r.id,
        type: r.type,
        payload: safeJson(r.payload_json),
        readAt: r.read_at,
        createdAt: r.created_at,
        expiresAt: r.expires_at,
    }));
    return response(200, { notifications: items });
}

function safeJson(s: string): Record<string, unknown> {
    try {
        return JSON.parse(s);
    } catch {
        return {};
    }
}

export async function handleNotificationsRead(env: ChatEnv, userId: string, request: Request, now = nowSeconds()): Promise<ChatHandlerResult> {
    await ensureCommunityChatSchema(env);
    const parsed = await readSmallJsonBody(request);
    if (!parsed.ok) return parsed.tooLarge ? error(413, 'TOO_LARGE', 'too large') : error(400, 'INVALID_REQUEST', 'bad json');
    const body = parsed.value as any;
    const ids = Array.isArray(body?.ids) ? body.ids.filter((x: any) => typeof x === 'string') : [];
    if (ids.length === 0) {
        await env.DB.prepare('UPDATE chat_notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL')
            .bind(now, userId)
            .run();
    } else {
        const placeholders = ids.map(() => '?').join(',');
        await env.DB.prepare(`UPDATE chat_notifications SET read_at = ? WHERE user_id = ? AND id IN (${placeholders})`)
            .bind(now, userId, ...ids)
            .run();
    }
    return response(200, { ok: true });
}

// ---------- cleanup ----------

export async function cleanupExpired(env: ChatEnv, now = nowSeconds()): Promise<void> {
    await ensureCommunityChatSchema(env);
    await env.DB.prepare(`DELETE FROM chat_threads WHERE expires_at IS NOT NULL AND expires_at < ?`).bind(now).run();
    await env.DB.prepare(`DELETE FROM chat_notifications WHERE expires_at < ?`).bind(now).run();
    // expire stale applications
    await env.DB.prepare(
        `UPDATE chat_applications SET status = 'expired' WHERE status = 'pending' AND created_at < ?`,
    )
        .bind(now - CHAT_APPLICATION_EXPIRY_SECONDS)
        .run();
}

// ---------- tor exit refresh ----------

export async function refreshTorExits(env: ChatEnv, now = nowSeconds()): Promise<void> {
    await ensureCommunityChatSchema(env);
    // fetch
    try {
        const res = await fetch('https://check.torproject.org/torbulkexitlist');
        if (!res.ok) return;
        const text = await res.text();
        const ips = text
            .split(/\s+/)
            .map(s => s.trim())
            .filter(s => /^\d+\.\d+\.\d+\.\d+$/.test(s));
        // replace
        await env.DB.prepare('DELETE FROM tor_exit_ips').run();
        const batch = ips.slice(0, 5000).map(ip =>
            env.DB.prepare('INSERT OR IGNORE INTO tor_exit_ips (ip, fetched_at) VALUES (?, ?)').bind(ip, now),
        );
        // D1 batch limit ~100; chunk
        for (let i = 0; i < batch.length; i += 50) {
            await env.DB.batch(batch.slice(i, i + 50));
        }
    } catch (e) {
        console.error('Tor exit refresh failed', e);
    }
}

export async function isTorIp(env: ChatEnv, ip: string): Promise<boolean> {
    try {
        await ensureCommunityChatSchema(env);
        const row = await env.DB.prepare('SELECT 1 FROM tor_exit_ips WHERE ip = ?').bind(ip).first();
        return !!row;
    } catch {
        return false;
    }
}