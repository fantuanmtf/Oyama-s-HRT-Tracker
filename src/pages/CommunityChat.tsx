import React, { useEffect, useState } from 'react';
import PageHeader, { PAGE_COLUMN } from '../components/PageHeader';
import { useAuth } from '../contexts/AuthContext';
import { useTranslation } from '../contexts/LanguageContext';
import { apiFetch } from '../services/apiClient';
import ReactMarkdown from 'react-markdown';
import rehypeSanitize from 'rehype-sanitize';
import remarkGfm from 'remark-gfm';

interface Thread {
    id: string;
    nickname_snapshot: string;
    title: string;
    body: string;
    kind: string;
    status: string;
    review_status: string;
    created_at: number;
}

const CommunityChat = ({ onBack }: { onBack: () => void }) => {
    const { t } = useTranslation();
    const { token, user } = useAuth();
    const [threads, setThreads] = useState<Thread[]>([]);
    const [identity, setIdentity] = useState<string | null>(null);
    const [nicknameInput, setNicknameInput] = useState('');
    const [title, setTitle] = useState('');
    const [body, setBody] = useState('');
    const [kind, setKind] = useState<'chat' | 'qa'>('chat');
    const [enabled, setEnabled] = useState<boolean | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [expanded, setExpanded] = useState<Set<string>>(new Set());
    const isAdmin = !!user?.isAdmin;

    const loadFlag = async () => {
        try {
            const res = await apiFetch('/api/chat/flag', { headers: token ? { Authorization: `Bearer ${token}` } : {} });
            const data = (await res.json()) as { enabled?: boolean };
            setEnabled(!!data.enabled);
        } catch {
            setEnabled(false);
        }
    };
    const loadIdentity = async () => {
        if (!token) return;
        try {
            const res = await apiFetch('/api/chat/identity', { headers: token ? { Authorization: `Bearer ${token}` } : {} });
            const data = (await res.json()) as { identity?: { nickname?: string } };
            setIdentity(data.identity?.nickname ?? null);
        } catch {
            setIdentity(null);
        }
    };
    const loadThreads = async () => {
        try {
            const res = await apiFetch(`/api/chat/threads?kind=${kind}`, { headers: token ? { Authorization: `Bearer ${token}` } : {} });
            const data = (await res.json()) as { threads?: Thread[] };
            setThreads(data.threads ?? []);
        } catch {
            setThreads([]);
        }
    };

    useEffect(() => {
        loadFlag();
    }, []);
    useEffect(() => {
        if (token) loadIdentity();
    }, [token]);
    useEffect(() => {
        if (enabled) loadThreads();
    }, [enabled, kind]);

    const saveIdentity = async () => {
        if (!token) return;
        setError(null);
        try {
            const res = await apiFetch('/api/chat/identity', {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
                body: JSON.stringify({ nickname: nicknameInput }),
            });
            if (!res.ok) throw new Error(((await res.json()) as { code?: string }).code ?? 'error');
            const data = (await res.json()) as { identity: { nickname: string } };
            setIdentity(data.identity.nickname);
            setNicknameInput('');
        } catch (e: any) {
            setError(e.message);
        }
    };

    const createThread = async () => {
        if (!token) return;
        setError(null);
        setLoading(true);
        try {
            const res = await apiFetch('/api/chat/threads', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: `Bearer ${token}` } : {}) },
                body: JSON.stringify({ title, body, kind }),
            });
            if (!res.ok) throw new Error(((await res.json()) as { code?: string }).code ?? 'error');
            setTitle('');
            setBody('');
            await loadThreads();
        } catch (e: any) {
            setError(e.message);
        } finally {
            setLoading(false);
        }
    };

    if (enabled === null) return <div className={PAGE_COLUMN}>Loading…</div>;
    if (!enabled) {
        return (
            <div className={PAGE_COLUMN}>
                <PageHeader title={t('chat.title')} onBack={onBack} />
                <p className="text-muted">{t('chat.disabled')}</p>
            </div>
        );
    }

    return (
        <div className={PAGE_COLUMN}>
            <PageHeader title={t('chat.title')} onBack={onBack} />
            {!token && <p className="text-muted">{t('chat.signin_required')}</p>}

            {token && !identity && (
                <div className="my-4 space-y-2">
                    <p>{t('chat.set_nickname')}</p>
                    <input
                        value={nicknameInput}
                        onChange={e => setNicknameInput(e.target.value)}
                        className="w-full border rounded px-2 py-1"
                        placeholder={t('chat.nickname_placeholder')}
                    />
                    <button onClick={saveIdentity} className="px-3 py-1 rounded bg-[var(--accent)] text-white">
                        {t('chat.save_nickname')}
                    </button>
                    {error && <p className="text-red-500 text-sm">{error}</p>}
                </div>
            )}

            {token && identity && (
                <div
                    className="my-4 space-y-2 border rounded p-3"
                    style={isAdmin ? { borderColor: 'var(--accent)' } : undefined}
                >
                    <p className="text-sm text-muted">
                        {t('chat.posting_as')}: {identity}{' '}
                        {isAdmin && <span className="text-xs font-semibold text-[var(--accent)]">ADMIN · UNRESTRICTED</span>}
                    </p>
                    <input
                        value={title}
                        onChange={e => setTitle(e.target.value)}
                        className="w-full border rounded px-2 py-1"
                        placeholder={t('chat.title_placeholder')}
                        style={{ overflowWrap: 'anywhere' }}
                    />
                    <textarea
                        value={body}
                        onChange={e => setBody(e.target.value)}
                        className="w-full border rounded px-2 py-1"
                        rows={isAdmin ? 8 : 4}
                        placeholder={isAdmin ? t('chat.admin_body_placeholder') : t('chat.body_placeholder')}
                        style={{ overflowWrap: 'anywhere', wordBreak: 'break-word' }}
                    />
                    <div className="flex gap-2">
                        <select value={kind} onChange={e => setKind(e.target.value as any)} className="border rounded px-2 py-1">
                            <option value="chat">{t('chat.kind_chat')}</option>
                            <option value="qa">{t('chat.kind_qa')}</option>
                        </select>
                        <button
                            onClick={createThread}
                            disabled={loading}
                            className="px-3 py-1 rounded bg-[var(--accent)] text-white disabled:opacity-50"
                        >
                            {t('chat.post')}
                        </button>
                    </div>
                    {isAdmin && <p className="text-xs text-muted">{t('chat.admin_unrestricted_hint')}</p>}
                    {error && <p className="text-red-500 text-sm">{error}</p>}
                </div>
            )}

            <div className="flex gap-2 my-2">
                <button onClick={() => setKind('chat')} className={`px-2 py-1 rounded ${kind === 'chat' ? 'bg-[var(--accent)] text-white' : 'border'}`}>
                    {t('chat.kind_chat')}
                </button>
                <button onClick={() => setKind('qa')} className={`px-2 py-1 rounded ${kind === 'qa' ? 'bg-[var(--accent)] text-white' : 'border'}`}>
                    {t('chat.kind_qa')}
                </button>
            </div>

            <ul className="space-y-3">
                {threads.map(th => {
                    const isOpen = expanded.has(th.id);
                    return (
                        <li key={th.id} className="border rounded p-3">
                            <div className="flex justify-between text-sm text-muted">
                                <span>{th.nickname_snapshot}</span>
                                <span>{new Date(th.created_at * 1000).toLocaleString()}</span>
                            </div>
                            {isOpen ? (
                                <>
                                    <h3
                                        className="font-semibold"
                                        style={{ overflowWrap: 'anywhere', wordBreak: 'break-word' }}
                                    >
                                        {th.title}
                                    </h3>
                                    {th.review_status === 'pending' && (
                                        <span className="text-xs text-amber-500">{t('chat.pending_review')}</span>
                                    )}
                                    <div
                                        className="prose prose-sm max-w-none"
                                        style={{ overflowWrap: 'anywhere', wordBreak: 'break-word', maxWidth: '100%' }}
                                    >
                                        <ReactMarkdown rehypePlugins={[rehypeSanitize]} remarkPlugins={[remarkGfm]}>
                                            {th.body}
                                        </ReactMarkdown>
                                    </div>
                                    <button
                                        type="button"
                                        onClick={() =>
                                            setExpanded(prev => {
                                                const n = new Set(prev);
                                                n.delete(th.id);
                                                return n;
                                            })
                                        }
                                        className="text-xs text-muted mt-1"
                                    >
                                        {t('chat.collapse')}
                                    </button>
                                </>
                            ) : (
                                <>
                                    <button
                                        type="button"
                                        onClick={() => setExpanded(prev => new Set(prev).add(th.id))}
                                        className="text-left font-semibold w-full"
                                        style={{ overflowWrap: 'anywhere', wordBreak: 'break-word' }}
                                    >
                                        {th.title}
                                    </button>
                                    {th.review_status === 'pending' && (
                                        <span className="text-xs text-amber-500">{t('chat.pending_review')}</span>
                                    )}
                                </>
                            )}
                        </li>
                    );
                })}
            </ul>
        </div>
    );
};

export default CommunityChat;