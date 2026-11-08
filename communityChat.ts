/**
 * Community chat constants, types, and pure helpers.
 *
 * Shared by the Worker, the page, and tests so rules cannot drift.
 */

export const CHAT_MAX_POST_CHARS = 1000;
export const CHAT_ADMIN_MAX_POST_CHARS = 1_000_000;
export const CHAT_TITLE_MAX_CHARS = 120;
export const CHAT_NICKNAME_MIN = 2;
export const CHAT_NICKNAME_MAX = 24;
export const CHAT_EDIT_WINDOW_SECONDS = 30 * 60;
export const CHAT_EDIT_MAX_COUNT = 5;
export const CHAT_TTL_SECONDS = 7 * 24 * 3600;
export const CHAT_CLOSE_GRACE_SECONDS = 24 * 3600;
export const CHAT_NOTIFICATION_RETENTION_SECONDS = 30 * 24 * 3600;
export const CHAT_NOTIFICATION_READ_RETENTION_SECONDS = 7 * 24 * 3600;
export const CHAT_APPLICATION_EXPIRY_SECONDS = 7 * 24 * 3600;
export const CHAT_RENAME_COOLDOWN_SECONDS = 7 * 24 * 3600;
export const CHAT_MAX_ADMINS = 5;
export const CHAT_ADMIN_BROADCASTS_PER_DAY = 5;
export const CHAT_TOR_REFRESH_SECONDS = 12 * 3600;
export const CHAT_MIN_ACCOUNT_AGE_DAYS = 30;
export const CHAT_MIN_HRT_DAYS = 30;

export const CHAT_RESTRICTION_DURATIONS: Record<string, number> = {
    '30m': 30 * 60,
    '1h': 3600,
    '6h': 6 * 3600,
    '12h': 12 * 3600,
    '1d': 24 * 3600,
    '7d': 7 * 24 * 3600,
    '30d': 30 * 24 * 3600,
    permanent: 0,
};

export const CHAT_RESTRICTION_REASONS = [
    'spam',
    'harassment',
    'selfharm',
    'illicit',
    'crypto',
    'csam',
    'evasion',
    'other',
] as const;
export type ChatRestrictionReason = (typeof CHAT_RESTRICTION_REASONS)[number];

export type ChatKind = 'chat' | 'qa';
export type ChatThreadStatus = 'open' | 'closed' | 'deleted';
export type ChatReviewStatus = 'auto' | 'pending' | 'approved' | 'rejected';
export type ChatRestrictionKind = 'mute' | 'ban';
export type ChatNotificationType =
    | 'admin_broadcast'
    | 'reply'
    | 'like'
    | 'review_approved'
    | 'review_rejected'
    | 'restriction'
    | 'trusted_granted'
    | 'trusted_revoked';

export interface ChatThread {
    id: string;
    authorUserId: string;
    nicknameSnapshot: string;
    title: string;
    body: string;
    kind: ChatKind;
    status: ChatThreadStatus;
    reviewStatus: ChatReviewStatus;
    pinned: boolean;
    expiresAt: number | null;
    createdAt: number;
    updatedAt: number;
    closedAt: number | null;
}

export interface ChatPost {
    id: string;
    threadId: string;
    authorUserId: string;
    nicknameSnapshot: string;
    body: string;
    parentPostId: string | null;
    reviewStatus: ChatReviewStatus;
    editedCount: number;
    lastEditedAt: number | null;
    createdAt: number;
    updatedAt: number;
}

export interface ChatIdentity {
    userId: string;
    nickname: string;
    createdAt: number;
    updatedAt: number;
}

export interface ChatRestriction {
    id: string;
    userId: string;
    kind: ChatRestrictionKind;
    reasonCode: ChatRestrictionReason;
    expiresAt: number | null;
    createdAt: number;
    createdBy: string | null;
    note: string | null;
}

export interface ChatNotification {
    id: string;
    userId: string;
    type: ChatNotificationType;
    payload: Record<string, unknown>;
    readAt: number | null;
    createdAt: number;
    expiresAt: number;
}

// ---------- pure helpers ----------

/** Normalize for matching: NFKC + strip zero-width + lowercase. */
export function normalizeForFilter(input: string): string {
    return input
        .normalize('NFKC')
        .replace(/[\u200b-\u200f\u202a-\u202e\u2060-\u2064\ufeff]/g, '')
        .toLowerCase();
}

const DARKNET_PATTERNS = [
    /\.onion\b/i,
    /\.i2p\b/i,
    /usk@/i,
    /freenet:/i,
    /freenet\//i,
];

export function containsDarknet(text: string): boolean {
    const n = normalizeForFilter(text);
    return DARKNET_PATTERNS.some(re => re.test(n));
}

/** Heuristic: a single token >40 chars with digit/symbol ratio > 0.6. */
export function looksLikeEncodedOrAddress(text: string): boolean {
    const tokens = text.split(/\s+/);
    for (const tok of tokens) {
        if (tok.length < 40) continue;
        let special = 0;
        for (const ch of tok) {
            if (/[0-9]/.test(ch) || /[^a-zA-Z]/.test(ch)) special++;
        }
        if (special / tok.length > 0.6) return true;
    }
    return false;
}

const CSAM_KEYWORDS = ['cp ', 'csam', 'loli', 'jailbait', 'underage porn'];
export function containsCsamKeyword(text: string): boolean {
    const n = normalizeForFilter(text);
    return CSAM_KEYWORDS.some(k => n.includes(k));
}

export function validateNickname(raw: unknown): string | null {
    if (typeof raw !== 'string') return null;
    const t = raw.trim();
    if (t.length < CHAT_NICKNAME_MIN || t.length > CHAT_NICKNAME_MAX) return null;
    if (!/^[\w\u4e00-\u9fff\u3040-\u30ff\uac00-\ud7af.\- ]+$/u.test(t)) return null;
    return t;
}

export function validatePostBody(raw: unknown, isAdmin: boolean): { ok: true; body: string } | { ok: false; code: string } {
    if (typeof raw !== 'string') return { ok: false, code: 'INVALID_BODY' };
    const body = raw.trim();
    if (!body) return { ok: false, code: 'EMPTY_BODY' };
    const cap = isAdmin ? CHAT_ADMIN_MAX_POST_CHARS : CHAT_MAX_POST_CHARS;
    if (body.length > cap) return { ok: false, code: 'TOO_LONG' };
    return { ok: true, body };
}

export function validateTitle(raw: unknown): string | null {
    if (typeof raw !== 'string') return null;
    const t = raw.trim();
    if (!t || t.length > CHAT_TITLE_MAX_CHARS) return null;
    return t;
}

/** Strip Markdown image syntax from a body (defence in depth). */
export function stripImages(text: string): string {
    return text.replace(/!\[[^\]]*\]\([^)]*\)/g, '');
}

export interface FilterResult {
    allow: boolean;
    review: boolean; // true => route to pending review
    reason?: string;
}

/** Content filter for non-admins. Admins bypass (pass allow=true, review=false). */
export function filterContent(text: string, isAdmin: boolean): FilterResult {
    if (isAdmin) return { allow: true, review: false };
    const combined = normalizeForFilter(text);
    if (containsCsamKeyword(combined)) return { allow: false, review: false, reason: 'csam' };
    if (containsDarknet(text)) return { allow: false, review: false, reason: 'darknet' };
    if (looksLikeEncodedOrAddress(text)) return { allow: true, review: true, reason: 'encoded' };
    return { allow: true, review: false };
}

export function isRestrictionActive(r: { kind: string; expiresAt: number | null }, now: number): boolean {
    if (r.kind === 'ban') return r.expiresAt === null || r.expiresAt > now;
    return r.expiresAt !== null && r.expiresAt > now;
}

export function accountAgeDays(createdAt: number | null, now: number): number {
    if (!createdAt) return 0;
    return Math.max(0, Math.floor((now - createdAt) / 86400));
}

export function isEligible(accountCreatedAt: number | null, hrtDays: number, now: number): boolean {
    return accountAgeDays(accountCreatedAt, now) >= CHAT_MIN_ACCOUNT_AGE_DAYS && hrtDays >= CHAT_MIN_HRT_DAYS;
}