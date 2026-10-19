/**
 * Confirmation copy for complete-backup actions.
 *
 * The bundle deliberately carries no source-account identifier, and the
 * website never encrypts it. These helpers keep the account target and the
 * plaintext warning explicit and testable without adding fields to the format.
 */

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
    const targetName = target.kind === 'account' ? target.username : t('import.bundle_target_local');
    return {
        target,
        lines: [`${t('import.bundle_target_account')} ${targetName}`, t('import.bundle_account_warning')],
    };
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
