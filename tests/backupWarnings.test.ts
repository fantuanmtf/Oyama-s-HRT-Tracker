import { describe, expect, it } from 'vitest';
import { TRANSLATIONS } from '../src/i18n/translations';
import {
    PARTIAL_WRITE_MESSAGE,
    type BundleCounts,
    type CompleteBackupMergeResult,
    type CompleteBackupPreview,
} from '../src/utils/backupBundle';
import {
    describeBundlePath,
    describeCompleteBackupAccountWarning,
    describeCompleteBackupExportWarning,
    describeCompleteBackupFailure,
    describeCompleteBackupPreview,
    describeCompleteBackupResult,
    describeUnreadableCompleteBackup,
    resolveCompleteBackupTarget,
} from '../src/utils/backupWarnings';

/** Read the shipped copy so the test checks real warning text, not a stub. */
const translate =
    (lang: 'en' | 'zh') =>
    (key: string): string =>
        (TRANSLATIONS[lang] as Record<string, string>)[key] ?? key;

const counts = (overrides: Partial<BundleCounts> = {}): BundleCounts => ({
    events: 0,
    labResults: 0,
    doseTemplates: 0,
    deletions: 0,
    quickDoses: 0,
    supplies: 0,
    plan: 0,
    ...overrides,
});

/** A synthetic preview with nothing in it; each test adds what it checks. */
const preview = (overrides: Partial<CompleteBackupPreview> = {}): CompleteBackupPreview => ({
    ok: true,
    errors: [],
    formatVersion: 1,
    createdAt: '2030-01-01T00:00:00.000Z',
    sourceMode: 'transfem',
    currentMode: 'transfem',
    targetOwner: 'local',
    counts: { transfem: counts(), transmasc: counts() },
    missing: [],
    blocked: [],
    issues: [],
    conflicts: [],
    stats: { added: 0, updated: 0, skipped: 0, removed: 0 },
    preferences: { values: {}, ignored: [], invalid: [] },
    deletionImpact: [],
    scalars: {},
    ...overrides,
});

const mergeResult = (overrides: Partial<CompleteBackupMergeResult> = {}): CompleteBackupMergeResult => ({
    applied: [],
    blocked: [],
    skipped: [],
    added: 0,
    updated: 0,
    removed: 0,
    modes: {},
    preferences: {},
    ...overrides,
});

describe('complete-backup confirmation copy', () => {
    it('names the signed-in target account and asks the reader to check the backup is theirs', () => {
        const warning = describeCompleteBackupAccountWarning({ username: '  synthetic-user  ' }, translate('en'));

        expect(warning.target).toEqual({ kind: 'account', username: 'synthetic-user' });
        expect(warning.lines).toEqual([
            'Merges into: account synthetic-user. Backups do not record which account they came from, so check that this one is yours.',
        ]);
    });

    it('falls back to the local device when signed out or the username is blank', () => {
        for (const user of [null, undefined, {}, { username: '   ' }, { username: null }]) {
            expect(resolveCompleteBackupTarget(user)).toEqual({ kind: 'local' });
            const warning = describeCompleteBackupAccountWarning(user, translate('en'));
            expect(warning.lines[0]).toContain('Merges into: this device (not signed in).');
        }
    });

    it('says the same in Chinese without claiming the bundle carries a source account', () => {
        const warning = describeCompleteBackupAccountWarning({ username: '合成账户' }, translate('zh'));

        expect(warning.lines).toEqual(['会合并到：账户 合成账户。备份不记得自己来自哪个账户，请确认它是你的。']);
    });

    it('asks before downloading an unencrypted file and names no tool that is not shipped', () => {
        for (const lang of ['en', 'zh'] as const) {
            const warning = describeCompleteBackupExportWarning(translate(lang));
            expect(warning.message).not.toMatch(/hrt-cold|CLI|JSON/);
            expect(warning.title).not.toBe('export.complete_confirm_title');
        }
        const en = describeCompleteBackupExportWarning(translate('en'));
        expect(en.title).toBe('Download an unencrypted backup?');
        expect(en.confirmLabel).toBe('Download');
        expect(en.message).toContain('not encrypted');

        const zh = describeCompleteBackupExportWarning(translate('zh'));
        expect(zh.title).toBe('下载未加密的备份？');
        expect(zh.message).toContain('没有加密');
    });
});

describe('complete-backup preview copy', () => {
    it('lists what the backup holds by mode, skipping empty categories', () => {
        const text = describeCompleteBackupPreview(
            preview({
                counts: {
                    transfem: counts({ events: 12, labResults: 3, deletions: 4 }),
                    transmasc: counts({ supplies: 1 }),
                },
                stats: { added: 15, updated: 0, skipped: 0, removed: 0 },
            }),
            { t: translate('en'), lang: 'en', user: null },
        );

        const [intro, contents, changes] = text.split('\n\n');
        expect(intro).toContain('Complete backup from ');
        expect(intro).toContain('2030');
        expect(intro).toContain('Merges into: this device (not signed in).');
        expect(contents).toBe('Feminizing HRT: doses 12, lab results 3\nMasculinizing HRT: supplies 1');
        expect(changes).toBe('15 added, 0 updated, 0 removed.');
    });

    it('reads as plain sentences: no mode keys, method ids, ISO dates or record ids', () => {
        const text = describeCompleteBackupPreview(
            preview({
                counts: { transfem: counts({ events: 1 }), transmasc: counts() },
                stats: { added: 0, updated: 1, skipped: 0, removed: 2 },
                blocked: ['transmasc.events', 'transmasc.deletions', 'weight'],
                conflicts: ['weight has equal updatedAt but different content'],
                issues: ['transfem.events[3]: unknown route'],
                scalars: {
                    weight: { action: 'conflict', current: 70, incoming: 62 },
                    pkParams: { action: 'updated', current: null, incoming: null },
                    calibration: {
                        action: 'overwrite',
                        current: { method: 'mipd', historyMode: 'retrospective' },
                        incoming: { method: 'ekf', historyMode: 'forward' },
                    },
                },
            }),
            { t: translate('zh'), lang: 'zh', user: { username: 'synthetic-user' } },
        );

        expect(text).toContain('会合并到：账户 synthetic-user。');
        expect(text).toContain('女性化 HRT：用药记录 1');
        expect(text).toContain('新增 0 条，更新 1 条，删除 2 条。');
        expect(text).toContain('删除的是在备份里已经删掉的记录。');
        expect(text).toContain('药物参数换成备份里的。');
        expect(text).toContain('校准设置换成备份里的，下一步会再问你一次。');
        expect(text).toContain('这些没有合并：男性化 HRT 用药记录、男性化 HRT 删除记录、体重。');
        // A weight in conflict is not merged, so the preview does not promise it.
        expect(text).not.toContain('体重改成');
        for (const jargon of ['transfem', 'transmasc', 'mipd', 'ekf', 'updatedAt', 'unknown route', '2030-01-01T']) {
            expect(text).not.toContain(jargon);
        }
    });

    it('says the weight it will set and that an empty backup holds no records', () => {
        const text = describeCompleteBackupPreview(
            preview({ scalars: { weight: { action: 'updated', current: 70, incoming: 62 } } }),
            { t: translate('en'), lang: 'en', user: null },
        );

        expect(text).toContain('The backup holds no records.');
        expect(text).toContain('Weight becomes 62 kg.');
    });

    it('names preview paths for people and keeps an unknown one as it is', () => {
        const t = translate('en');
        expect(describeBundlePath('transfem.labResults', t)).toBe('Feminizing HRT lab results');
        expect(describeBundlePath('pkParams', t)).toBe('drug parameters');
        expect(describeBundlePath('transfem.somethingNew', t)).toBe('transfem.somethingNew');
    });
});

describe('complete-backup outcome copy', () => {
    it('tells an unreadable file from one made by a newer version', () => {
        const t = translate('en');
        expect(describeUnreadableCompleteBackup(preview({ ok: false, formatVersion: 2 }), t)).toBe(
            'This complete backup comes from a newer version. Update the app first.',
        );
        for (const formatVersion of [undefined, 0, 1]) {
            expect(describeUnreadableCompleteBackup(preview({ ok: false, formatVersion }), t)).toBe(
                'This is not a complete backup file this app can read.',
            );
        }
    });

    it('reports what a merge did, and the parts it left alone', () => {
        const t = translate('zh');
        expect(
            describeCompleteBackupResult(
                mergeResult({ added: 3, updated: 1, removed: 0, blocked: ['transfem.plan'] }),
                'applied',
                t,
            ),
        ).toBe('合并好了：新增 3 条，更新 1 条，删除 0 条。\n这些没有合并：女性化 HRT 用药方案。');
        expect(describeCompleteBackupResult(mergeResult(), 'settings-only', t)).toBe(
            '只导入了设置，备份里没有能合并的记录。',
        );
        expect(describeCompleteBackupResult(mergeResult({ blocked: ['weight'] }), 'nothing', t)).toBe(
            '备份里没有能合并的内容。\n这些没有合并：体重。',
        );
    });

    it('says the device is unchanged unless the rollback failed too', () => {
        const t = translate('en');
        expect(describeCompleteBackupFailure(new Error('synthetic storage write failure'), t)).toBe(
            'The import failed. Nothing on this device was changed.',
        );
        expect(describeCompleteBackupFailure(new Error(PARTIAL_WRITE_MESSAGE), t)).toBe(
            'The import did not finish, and part of it may already have been written.',
        );
    });
});
