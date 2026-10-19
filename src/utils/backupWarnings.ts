/**
 * Confirmation copy for complete-backup actions.
 *
 * The bundle deliberately carries no source-account identifier, and the
 * website never encrypts it. These helpers keep the account target and the
 * plaintext warning explicit and testable without adding fields to the format.
 *
 * Everything here is what a person reads, so it says what will happen in
 * their language and leaves the record-level detail (ids, validator messages)
 * to the console.
 */
import type { Lang } from '../i18n/translations';
import { LOCALE_MAP } from './helpers';
import {
    COMPLETE_BACKUP_FORMAT_VERSION,
    PARTIAL_WRITE_MESSAGE,
    type CompleteBackupMergeResult,
    type CompleteBackupPreview,
} from './backupBundle';
import { MODE_KEYS } from './syncMerge';

export type Translate = (key: string) => string;

/** The subset of the signed-in user needed to name the import target. */
export interface BackupTargetUser {
    username?: string | null;
}

export type CompleteBackupTarget = { kind: 'account'; username: string } | { kind: 'local' };

/**
 * Resolve the account currently receiving the merge. A blank or missing
 * username is treated as the signed-out local store, never as an account match.
 */
export function resolveCompleteBackupTarget(user: BackupTargetUser | null | undefined): CompleteBackupTarget {
    const username = typeof user?.username === 'string' ? user.username.trim() : '';
    return username ? { kind: 'account', username } : { kind: 'local' };
}

export interface CompleteBackupAccountWarning {
    target: CompleteBackupTarget;
    lines: string[];
}

/**
 * Name the target account and state the format limitation before a merge.
 * There is no source-account field to compare, so the warning promises no
 * cross-account detection.
 */
export function describeCompleteBackupAccountWarning(
    user: BackupTargetUser | null | undefined,
    t: Translate,
): CompleteBackupAccountWarning {
    const target = resolveCompleteBackupTarget(user);
    const targetName =
        target.kind === 'account'
            ? t('import.bundle_target_account').replace('{name}', target.username)
            : t('import.bundle_target_local');
    return { target, lines: [t('import.bundle_into').replace('{target}', targetName)] };
}

export interface CompleteBackupExportWarning {
    title: string;
    message: string;
    confirmLabel: string;
}

/** Explicit plaintext-health-data confirmation before the bundle download. */
export function describeCompleteBackupExportWarning(t: Translate): CompleteBackupExportWarning {
    return {
        title: t('export.complete_confirm_title'),
        message: t('export.complete_confirm_message'),
        confirmLabel: t('export.complete_confirm_btn'),
    };
}

const RECORD_CATEGORIES = ['events', 'labResults', 'doseTemplates', 'quickDoses', 'supplies', 'plan'] as const;

/**
 * What a person calls a preview path: `transfem.events`, `transmasc.deletions`,
 * `weight`. A path this build has no name for is shown as it is rather than as
 * a missing translation key.
 */
export function describeBundlePath(path: string, t: Translate): string {
    const [first, second] = path.split('.');
    const categoryKey = `import.bundle_cat_${second ?? first}`;
    const category = t(categoryKey);
    if (category === categoryKey) return path;
    if (second === undefined) return category;
    const modeKey = `mode.${first}`;
    const mode = t(modeKey);
    return mode === modeKey ? category : `${mode} ${category}`;
}

/** The parts that were not merged, named once each, joined the reader's way. */
export function describeBlockedParts(paths: string[], t: Translate): string {
    const names = [...new Set(paths.map(path => describeBundlePath(path, t)))];
    return t('import.bundle_skipped_parts').replace('{list}', names.join(t('import.bundle_sep')));
}

const formatBackupDate = (iso: string, lang: Lang): string => {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return iso;
    return date.toLocaleString(LOCALE_MAP[lang] || 'en-US', { dateStyle: 'medium', timeStyle: 'short' });
};

/**
 * The confirmation shown before a complete bundle is merged: when it was made,
 * where it goes, what it holds, and what the merge will change.
 */
export function describeCompleteBackupPreview(
    preview: CompleteBackupPreview,
    { t, lang, user }: { t: Translate; lang: Lang; user: BackupTargetUser | null | undefined },
): string {
    const intro = [
        t('import.bundle_from').replace('{date}', preview.createdAt ? formatBackupDate(preview.createdAt, lang) : ''),
        ...describeCompleteBackupAccountWarning(user, t).lines,
    ];

    const contents: string[] = [];
    for (const mode of MODE_KEYS) {
        const counts = preview.counts[mode];
        const parts = RECORD_CATEGORIES.filter(category => counts[category] > 0).map(category =>
            t('import.bundle_count')
                .replace('{name}', t(`import.bundle_cat_${category}`))
                .replace('{n}', String(counts[category])),
        );
        if (!parts.length) continue;
        contents.push(
            t('import.bundle_mode_line')
                .replace('{mode}', t(`mode.${mode}`))
                .replace('{list}', parts.join(t('import.bundle_sep'))),
        );
    }
    if (!contents.length) contents.push(t('import.bundle_empty'));

    const { added, updated, removed } = preview.stats;
    const changes = [
        t('import.bundle_changes')
            .replace('{added}', String(added))
            .replace('{updated}', String(updated))
            .replace('{removed}', String(removed)),
    ];
    if (removed > 0) changes.push(t('import.bundle_removed_note'));
    const { weight, pkParams, calibration } = preview.scalars;
    if (weight && (weight.action === 'added' || weight.action === 'updated')) {
        changes.push(t('import.bundle_weight').replace('{kg}', String(weight.incoming)));
    }
    if (pkParams && (pkParams.action === 'added' || pkParams.action === 'updated')) changes.push(t('import.bundle_pk'));
    if (calibration?.action === 'overwrite') changes.push(t('import.bundle_calibration'));
    if (preview.blocked.length) changes.push(describeBlockedParts(preview.blocked, t));

    return [intro.join('\n'), contents.join('\n'), changes.join('\n')].join('\n\n');
}

/**
 * Why a file was not imported, in one line: made by a newer version of the app
 * (update first), or not a complete backup this app can read.
 */
export function describeUnreadableCompleteBackup(preview: CompleteBackupPreview, t: Translate): string {
    const newer = preview.formatVersion !== undefined && preview.formatVersion > COMPLETE_BACKUP_FORMAT_VERSION;
    return t(newer ? 'import.bundle_unknown_version' : 'import.bundle_invalid');
}

/** What to tell the person after a merge, by what it actually did. */
export function describeCompleteBackupResult(
    result: CompleteBackupMergeResult,
    outcome: 'applied' | 'settings-only' | 'nothing',
    t: Translate,
): string {
    const lines =
        outcome === 'applied'
            ? [
                  t('import.bundle_merge_success')
                      .replace('{added}', String(result.added))
                      .replace('{updated}', String(result.updated))
                      .replace('{removed}', String(result.removed)),
              ]
            : [t(outcome === 'settings-only' ? 'import.bundle_settings_only' : 'import.bundle_nothing')];
    if (result.blocked.length) lines.push(describeBlockedParts(result.blocked, t));
    return lines.join('\n');
}

/**
 * After a merge threw. The writes are rolled back first, so the device is
 * unchanged unless that rollback failed too.
 */
export function describeCompleteBackupFailure(err: unknown, t: Translate): string {
    const partial = err instanceof Error && err.message === PARTIAL_WRITE_MESSAGE;
    return t(partial ? 'import.bundle_merge_partial' : 'import.bundle_merge_failed');
}
