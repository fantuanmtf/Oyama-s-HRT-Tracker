import { describe, expect, it } from 'vitest';
import { TRANSLATIONS } from '../src/i18n/translations';
import {
    describeCompleteBackupAccountWarning,
    describeCompleteBackupExportWarning,
    resolveCompleteBackupTarget,
} from '../src/utils/backupWarnings';

/** Read the shipped copy so the test checks real warning text, not a stub. */
const translate =
    (lang: 'en' | 'zh') =>
    (key: string): string =>
        (TRANSLATIONS[lang] as Record<string, string>)[key] ?? key;

describe('complete-backup confirmation copy', () => {
    it('names the signed-in target account and states that the format has no source identifier', () => {
        const warning = describeCompleteBackupAccountWarning({ username: '  synthetic-user  ' }, translate('en'));

        expect(warning.target).toEqual({ kind: 'account', username: 'synthetic-user' });
        expect(warning.lines[0]).toBe('Target account for this merge: synthetic-user');
        expect(warning.lines[1]).toContain('does not record which account');
        expect(warning.lines[1]).toContain('different account');
        expect(warning.lines[1]).toContain('cannot detect');
    });

    it('falls back to the local device when signed out or the username is blank', () => {
        for (const user of [null, undefined, {}, { username: '   ' }, { username: null }]) {
            expect(resolveCompleteBackupTarget(user)).toEqual({ kind: 'local' });
            const warning = describeCompleteBackupAccountWarning(user, translate('en'));
            expect(warning.lines[0]).toBe('Target account for this merge: this device (not signed in)');
        }
    });

    it('warns in Chinese without claiming the bundle carries a source account', () => {
        const warning = describeCompleteBackupAccountWarning({ username: '合成账户' }, translate('zh'));

        expect(warning.lines[0]).toBe('本次合并的目标账户： 合成账户');
        expect(warning.lines[1]).toContain('不记录它来自哪个账户');
        expect(warning.lines[1]).toContain('无法自动识别');
    });

    it('requires a plaintext-health-data confirmation and recommends the offline Tools CLI', () => {
        const warning = describeCompleteBackupExportWarning(translate('en'));

        expect(warning.title).toBe('Download unencrypted health data?');
        expect(warning.confirmLabel).toBe('Download plaintext');
        expect(warning.message).toContain('unencrypted plaintext JSON');
        expect(warning.message).toContain('health records');
        expect(warning.message).toContain('offline Tools CLI (hrt-cold encrypt)');
        expect(warning.message).toContain('This website does not encrypt this file');
        expect(warning.message.toLowerCase()).not.toContain('encrypted by this website');
    });

    it('has the plaintext confirmation in Chinese as well', () => {
        const warning = describeCompleteBackupExportWarning(translate('zh'));

        expect(warning.title).toBe('下载未加密的健康数据？');
        expect(warning.confirmLabel).toBe('下载纯文本备份');
        expect(warning.message).toContain('未加密的纯文本 JSON');
        expect(warning.message).toContain('hrt-cold encrypt');
        expect(warning.message).toContain('本网站不会加密此文件');
    });
});
