import { describe, expect, it } from 'vitest';
import { DEFAULT_PK_PARAMS, Ester, Route, type PKCustomParams } from '../logic';
import {
    COMPLETE_BACKUP_FORMAT,
    COMPLETE_BACKUP_FORMAT_VERSION,
    buildCompleteBackupBundleFromStorage,
    inspectCompleteBackupBundle,
    mergeCompleteBackupBundleToStorage,
    readPortablePreferences,
    sanitizePortablePreferences,
    type CompleteBackupActive,
    type CompleteBackupBundle,
    type CompleteBackupContext,
} from '../src/utils/backupBundle';
import { modeKeyFor, type StorageLike } from '../src/utils/backupState';
import { type ModeKey } from '../src/utils/syncMerge';

/** Synthetic values only. No fixture contains real health data or secrets. */
const OWNER = 'local';
const CREATED_AT = '2030-01-01T00:00:00.000Z';
const TOMBSTONE_AT = 123;

class MemoryStorage implements StorageLike {
    readonly map = new Map<string, string>();
    getItem(key: string): string | null {
        return this.map.get(key) ?? null;
    }
    setItem(key: string, value: string): void {
        this.map.set(key, value);
    }
    removeItem(key: string): void {
        this.map.delete(key);
    }
    snapshot(): string {
        return JSON.stringify([...this.map.entries()].sort(([a], [b]) => a.localeCompare(b)));
    }
}

const key = (mode: ModeKey, suffix: string) => modeKeyFor(OWNER, mode, suffix);

const event = (id: string, overrides: Record<string, unknown> = {}) => ({
    id,
    route: Route.sublingual,
    timeH: 500_000,
    doseMG: 2,
    ester: Ester.E2,
    extras: {},
    updatedAt: 1_000,
    ...overrides,
});

const tEvent = (id: string, overrides: Record<string, unknown> = {}) =>
    event(id, { route: Route.injection, ester: Ester.TE, ...overrides });

const lab = (id: string, overrides: Record<string, unknown> = {}) => ({
    id,
    timeH: 500_000,
    concValue: 120,
    unit: 'pg/ml',
    updatedAt: 1_000,
    ...overrides,
});

const template = (id: string, overrides: Record<string, unknown> = {}) => ({
    id,
    name: 'Synthetic template',
    route: Route.oral,
    ester: Ester.E2,
    doseMG: 2,
    extras: {},
    createdAt: 1_000,
    updatedAt: 1_000,
    ...overrides,
});

const quickDose = (id: string) => ({
    id,
    route: Route.sublingual,
    ester: Ester.E2,
    value: 2,
    createdAt: 1_000,
});

const plan = (id: string, overrides: Record<string, unknown> = {}) => ({
    id,
    route: Route.sublingual,
    ester: Ester.E2,
    doseMG: 2,
    extras: {},
    everyDays: 1,
    timesPerDay: 2,
    ...overrides,
});

const supply = (id: string, pack: Record<string, unknown>, overrides: Record<string, unknown> = {}) => ({
    id,
    route: pack.kind === 'patch' ? Route.patchApply : Route.injection,
    ester: Ester.E2,
    pack,
    count: 2,
    startedAt: 1_000,
    ...overrides,
});

const packs = () => [
    { kind: 'vial', ml: 5, mgPerMl: 40 },
    { kind: 'pill', mg: 2 },
    { kind: 'pump', mg: 1.25 },
    { kind: 'sachet', mg: 1.5 },
    { kind: 'patch' },
    { kind: 'bulk' },
];

const tombstoneSeed = () => ({
    events: { 'deleted-event': TOMBSTONE_AT },
    labResults: { 'deleted-lab': TOMBSTONE_AT + 1 },
    doseTemplates: { 'deleted-template': TOMBSTONE_AT + 2 },
});

const seedMode = (storage: MemoryStorage, mode: ModeKey, prefix: string) => {
    const isT = mode === 'transmasc';
    const ev = isT ? tEvent(`${prefix}-e1`) : event(`${prefix}-e1`);
    const lb = isT ? lab(`${prefix}-l1`, { unit: 'ng/dl' }) : lab(`${prefix}-l1`);
    storage.setItem(key(mode, 'events'), JSON.stringify([ev]));
    storage.setItem(key(mode, 'lab-results'), JSON.stringify([lb]));
    storage.setItem(key(mode, 'dose-templates'), JSON.stringify([template(`${prefix}-t1`)]));
    storage.setItem(key(mode, 'deletions'), JSON.stringify(tombstoneSeed()));
    storage.setItem(key(mode, 'quick-doses'), JSON.stringify([quickDose(`${prefix}-q1`)]));
    storage.setItem(
        key(mode, 'supplies'),
        JSON.stringify(packs().map((pack, index) => supply(`${prefix}-s${index}`, pack))),
    );
    storage.setItem(key(mode, 'plan'), JSON.stringify([plan(`${prefix}-p1`)]));
};

const seed = (): MemoryStorage => {
    const storage = new MemoryStorage();
    seedMode(storage, 'transfem', 'tf');
    seedMode(storage, 'transmasc', 'tm');
    storage.setItem('hrt-weight', '70');
    storage.setItem('hrt-weight-at', '1111');
    storage.setItem('hrt-pk-params', JSON.stringify({ ...DEFAULT_PK_PARAMS, e2_kClear: 0.42 }));
    storage.setItem('hrt-pk-params-at', '2222');
    storage.setItem('hrt-cal-method', 'ekf');
    storage.setItem('hrt-cal-history-mode', 'forward');
    storage.setItem('app-theme', 'dark');
    storage.setItem('hrt-lang', 'en');
    storage.setItem('app-show-prediction', 'true');
    storage.setItem('app-hide-site-label', 'false');
    storage.setItem('hrt-supply-lead-days', '14');
    storage.setItem('app-auto-backup', 'true');
    storage.setItem('app-dev-mode', 'true');
    storage.setItem('app-onboarded', 'true');
    storage.setItem('site-notice-dismissed', 'true');
    storage.setItem('auth_token', 'synthetic-token');
    storage.setItem('weak_password', 'synthetic-password');
    storage.setItem('login_device_passes', 'synthetic-passes');
    storage.setItem('enc_key', 'synthetic-key');
    return storage;
};

const activeFrom = (storage: MemoryStorage, mode: ModeKey): CompleteBackupActive => {
    const read = <T>(suffix: string): T[] => {
        const raw = storage.getItem(key(mode, suffix));
        return raw ? (JSON.parse(raw) as T[]) : [];
    };
    return {
        events: read('events'),
        labResults: read('lab-results'),
        doseTemplates: read('dose-templates'),
        quickDoses: read('quick-doses'),
        supplies: read('supplies'),
        plan: read('plan'),
        weight: Number(storage.getItem('hrt-weight') ?? '70'),
        pkParams: JSON.parse(storage.getItem('hrt-pk-params') ?? 'null') as PKCustomParams | null,
        calibrationMethod: (storage.getItem('hrt-cal-method') as 'ekf') ?? 'mipd',
        calibrationHistoryMode: (storage.getItem('hrt-cal-history-mode') as 'forward') ?? 'retrospective',
    };
};

const context = (
    storage: MemoryStorage,
    mode: ModeKey = 'transfem',
    active: CompleteBackupActive = activeFrom(storage, mode),
): CompleteBackupContext => ({ storage, owner: OWNER, mode, active });

const build = (storage: MemoryStorage, mode: ModeKey = 'transfem'): CompleteBackupBundle =>
    buildCompleteBackupBundleFromStorage(storage, OWNER, mode, activeFrom(storage, mode), CREATED_AT);

describe('complete backup bundle data layer', () => {
    it('round-trips both modes, all Supply.pack branches, calibration, scalars, and preferences', () => {
        const source = seed();
        const bundle = build(source);

        expect(bundle.format).toBe(COMPLETE_BACKUP_FORMAT);
        expect(bundle.formatVersion).toBe(COMPLETE_BACKUP_FORMAT_VERSION);
        expect(bundle.createdAt).toBe(CREATED_AT);
        expect(bundle.mode).toBe('transfem');
        expect(bundle.modes.transfem.events?.[0].id).toBe('tf-e1');
        expect(bundle.modes.transmasc.events?.[0].id).toBe('tm-e1');
        expect(bundle.modes.transfem.supplies?.map(s => s.pack.kind)).toEqual([
            'vial',
            'pill',
            'pump',
            'sachet',
            'patch',
            'bulk',
        ]);
        expect(bundle.modes.transfem.deletions?.events).toEqual({ 'deleted-event': TOMBSTONE_AT });
        expect(bundle.calibration).toEqual({ method: 'ekf', historyMode: 'forward' });
        expect(bundle.weight).toBe(70);
        expect(bundle.weightUpdatedAt).toBe(1111);
        expect(bundle.pkParams?.e2_kClear).toBe(0.42);
        expect(bundle.pkParamsUpdatedAt).toBe(2222);
        expect(bundle.preferences).toEqual({
            'app-theme': 'dark',
            'hrt-lang': 'en',
            'app-show-prediction': true,
            'app-hide-site-label': false,
            'hrt-supply-lead-days': 14,
        });

        const serialized = JSON.stringify(bundle);
        for (const forbidden of [
            'auth_token',
            'weak_password',
            'login_device_passes',
            'enc_key',
            'app-auto-backup',
            'app-dev-mode',
            'app-onboarded',
            'site-notice-dismissed',
        ]) {
            expect(serialized).not.toContain(forbidden);
        }

        const target = new MemoryStorage();
        const preview = inspectCompleteBackupBundle(bundle, context(target));
        expect(preview.ok).toBe(true);
        expect(preview.conflicts).toEqual([]);
        expect(preview.counts.transfem.supplies).toBe(6);
        expect(preview.counts.transfem.quickDoses).toBe(1);
        expect(preview.counts.transfem.plan).toBe(1);
        expect(preview.counts.transfem.deletions).toBe(3);

        const result = mergeCompleteBackupBundleToStorage(bundle, context(target));
        expect(result.blocked).toEqual([]);
        expect(result.added).toBe(22);
        expect(result.modes.transfem.events?.[0].id).toBe('tf-e1');
        expect(result.modes.transmasc.events?.[0].id).toBe('tm-e1');
        expect(JSON.parse(target.getItem(key('transfem', 'supplies'))!)).toHaveLength(6);
        expect(JSON.parse(target.getItem(key('transmasc', 'supplies'))!)).toHaveLength(6);
        expect(JSON.parse(target.getItem(key('transfem', 'supplies'))!)[4].pack).toEqual({ kind: 'patch' });
        expect(JSON.parse(target.getItem(key('transfem', 'quick-doses'))!)[0].id).toBe('tf-q1');
        expect(JSON.parse(target.getItem(key('transfem', 'plan'))!)[0]).toMatchObject({
            everyDays: 1,
            timesPerDay: 2,
        });
        expect(JSON.parse(target.getItem(key('transfem', 'deletions'))!).events).toEqual({
            'deleted-event': TOMBSTONE_AT,
        });
        expect(target.getItem('hrt-cal-method')).toBe('ekf');
        expect(target.getItem('hrt-cal-history-mode')).toBe('forward');
        expect(target.getItem('hrt-weight')).toBe('70');
        expect(target.getItem('hrt-pk-params-at')).toBe('2222');
        expect(result.preferences).toEqual(bundle.preferences);
        expect(target.getItem('app-auto-backup')).toBeNull();
        expect(target.getItem('auth_token')).toBeNull();
    });

    it('rejects string and malformed Supply.pack without writing that category', () => {
        const source = seed();
        const bundle = build(source);
        const broken = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
        (broken.modes.transfem.supplies![0] as { pack: unknown }).pack = 'vial';

        const target = new MemoryStorage();
        const preview = inspectCompleteBackupBundle(broken, context(target));
        expect(preview.ok).toBe(true);
        expect(preview.blocked).toContain('transfem.supplies');
        expect(preview.issues.join(' ')).toContain('pack must be an object');

        const result = mergeCompleteBackupBundleToStorage(broken, context(target));
        expect(result.blocked).toContain('transfem.supplies');
        expect(target.getItem(key('transfem', 'supplies'))).toBeNull();
        // Other categories still land.
        expect(target.getItem(key('transfem', 'events'))).not.toBeNull();
        expect(result.modes.transmasc.supplies).toHaveLength(6);

        for (const pack of [{ kind: 'unknown' }, { kind: 'pill' }, { kind: 'vial', ml: 0, mgPerMl: 40 }]) {
            const invalid = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
            (invalid.modes.transfem.supplies![0] as { pack: unknown }).pack = pack;
            expect(inspectCompleteBackupBundle(invalid, context(new MemoryStorage())).blocked).toContain(
                'transfem.supplies',
            );
        }
    });

    it('blocks duplicate ids inside the backup and reports invalid records', () => {
        const source = seed();
        const bundle = build(source);

        const duplicate = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
        duplicate.modes.transfem.supplies!.push({ ...duplicate.modes.transfem.supplies![0] });
        const duplicatePreview = inspectCompleteBackupBundle(duplicate, context(new MemoryStorage()));
        expect(duplicatePreview.blocked).toContain('transfem.supplies');
        expect(duplicatePreview.issues.join(' ')).toContain('duplicate id');

        const merged = mergeCompleteBackupBundleToStorage(duplicate, context(new MemoryStorage()));
        expect(merged.blocked).toContain('transfem.supplies');
        expect(merged.modes.transfem.supplies).toBeUndefined();

        const invalid = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
        (invalid.modes.transfem.events![0] as { route: unknown }).route = 'not-a-route';
        const invalidPreview = inspectCompleteBackupBundle(invalid, context(new MemoryStorage()));
        expect(invalidPreview.blocked).toContain('transfem.events');
        expect(invalidPreview.issues.join(' ')).toContain('unknown route');
        const invalidMerge = mergeCompleteBackupBundleToStorage(invalid, context(new MemoryStorage()));
        expect(invalidMerge.modes.transfem.events).toBeUndefined();
        expect(invalidMerge.modes.transfem.labResults).toHaveLength(1);
    });

    it('aborts a category when the same id exists without a reliable updatedAt', () => {
        const source = seed();
        const bundle = build(source);
        const local = new MemoryStorage();
        const localEvent = event('tf-e1');
        delete (localEvent as { updatedAt?: number }).updatedAt;
        local.setItem(key('transfem', 'events'), JSON.stringify([localEvent]));
        local.setItem(key('transfem', 'supplies'), JSON.stringify([supply('tf-s0', packs()[0])]));
        local.setItem(key('transfem', 'lab-results'), JSON.stringify([]));
        local.setItem(key('transfem', 'dose-templates'), JSON.stringify([]));
        local.setItem(key('transfem', 'quick-doses'), JSON.stringify([]));
        local.setItem(key('transfem', 'plan'), JSON.stringify([]));

        const preview = inspectCompleteBackupBundle(bundle, context(local));
        expect(preview.blocked).toContain('transfem.events');
        expect(preview.blocked).toContain('transfem.supplies');
        expect(preview.conflicts.join(' ')).toContain('without a reliable updatedAt');
        expect(preview.conflicts.join(' ')).toContain('no updatedAt');

        const result = mergeCompleteBackupBundleToStorage(bundle, context(local));
        expect(result.blocked).toContain('transfem.events');
        expect(result.blocked).toContain('transfem.supplies');
        expect(JSON.parse(local.getItem(key('transfem', 'events'))!)).toHaveLength(1);
        expect(JSON.parse(local.getItem(key('transfem', 'supplies'))!)).toHaveLength(1);
        expect(result.modes.transfem.events).toBeUndefined();
        expect(result.modes.transmasc.events?.[0].id).toBe('tm-e1');
    });

    it('applies newer timestamps and skips older ones for stamped records', () => {
        const source = seed();
        const bundle = build(source);
        const local = new MemoryStorage();
        local.setItem(key('transfem', 'events'), JSON.stringify([event('tf-e1', { doseMG: 1 })]));
        local.setItem(key('transfem', 'lab-results'), JSON.stringify([lab('tf-l1', { concValue: 100 })]));
        local.setItem(key('transfem', 'dose-templates'), JSON.stringify([]));
        local.setItem(key('transfem', 'quick-doses'), JSON.stringify([]));
        local.setItem(key('transfem', 'supplies'), JSON.stringify([]));
        local.setItem(key('transfem', 'plan'), JSON.stringify([]));

        const newer = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
        newer.modes.transfem.events![0].updatedAt = 2_000;
        newer.modes.transfem.events![0].doseMG = 3;
        newer.modes.transfem.labResults![0].updatedAt = 500;

        const preview = inspectCompleteBackupBundle(newer, context(local));
        expect(preview.blocked).not.toContain('transfem.events');
        expect(preview.blocked).not.toContain('transfem.labResults');
        expect(preview.stats.updated).toBe(1);
        expect(preview.stats.skipped).toBe(1);

        mergeCompleteBackupBundleToStorage(newer, context(local));
        expect(JSON.parse(local.getItem(key('transfem', 'events'))!)[0].doseMG).toBe(3);
        expect(JSON.parse(local.getItem(key('transfem', 'lab-results'))!)[0].concValue).toBe(100);
    });

    it('rejects unknown formats/versions before any write', () => {
        const storage = seed();
        const bundle = build(storage);
        const before = storage.snapshot();

        for (const broken of [
            { ...bundle, format: 'something-else' },
            { ...bundle, formatVersion: 999 },
            { ...bundle, createdAt: 'not-a-date' },
        ]) {
            const preview = inspectCompleteBackupBundle(broken, context(new MemoryStorage()));
            expect(preview.ok).toBe(false);
            expect(preview.errors.length).toBeGreaterThan(0);
            expect(() => mergeCompleteBackupBundleToStorage(broken, context(storage))).toThrow(
                /Invalid complete backup/,
            );
        }
        expect(storage.snapshot()).toBe(before);
    });

    it('distinguishes empty arrays from missing categories', () => {
        const source = seed();
        const bundle = build(source);
        const missing = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
        delete missing.modes.transfem.quickDoses;
        const missingPreview = inspectCompleteBackupBundle(missing, context(new MemoryStorage()));
        expect(missingPreview.missing).toContain('transfem.quickDoses');
        expect(missingPreview.counts.transfem.quickDoses).toBe(0);

        const empty = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
        empty.modes.transfem.quickDoses = [];
        const emptyPreview = inspectCompleteBackupBundle(empty, context(new MemoryStorage()));
        expect(emptyPreview.missing).not.toContain('transfem.quickDoses');
        expect(emptyPreview.counts.transfem.quickDoses).toBe(0);

        const local = new MemoryStorage();
        local.setItem(key('transfem', 'quick-doses'), JSON.stringify([quickDose('local-q')]));
        const result = mergeCompleteBackupBundleToStorage(empty, context(local));
        expect(result.applied).toContain('transfem.quickDoses');
        expect(JSON.parse(local.getItem(key('transfem', 'quick-doses'))!)[0].id).toBe('local-q');
    });

    it('exports and applies only the confirmed preference whitelist', () => {
        const sanitized = sanitizePortablePreferences({
            'app-theme': 'dark',
            'hrt-lang': 'en',
            'app-show-prediction': true,
            'app-hide-site-label': false,
            'hrt-supply-lead-days': 14,
            'app-auto-backup': true,
            'app-dev-mode': true,
            'app-onboarded': true,
            'site-notice-dismissed': true,
            auth_token: 'synthetic-token',
            enc_key: 'synthetic-key',
            'app-theme-extra': 'invalid',
        });
        expect(sanitized.values).toEqual({
            'app-theme': 'dark',
            'hrt-lang': 'en',
            'app-show-prediction': true,
            'app-hide-site-label': false,
            'hrt-supply-lead-days': 14,
        });
        expect(sanitized.ignored).toEqual(
            expect.arrayContaining([
                'app-auto-backup',
                'app-dev-mode',
                'app-onboarded',
                'site-notice-dismissed',
                'auth_token',
                'enc_key',
                'app-theme-extra',
            ]),
        );
        expect(sanitized.invalid).toEqual([]);
        expect(sanitizePortablePreferences({ 'hrt-supply-lead-days': 0 }).invalid).toEqual(['hrt-supply-lead-days']);

        const storage = new MemoryStorage();
        storage.setItem('app-theme', 'mono');
        storage.setItem('hrt-lang', 'tr');
        storage.setItem('app-show-prediction', 'false');
        storage.setItem('app-hide-site-label', 'true');
        storage.setItem('hrt-supply-lead-days', '30');
        storage.setItem('app-auto-backup', 'false');
        storage.setItem('auth_token', 'synthetic-token');
        expect(readPortablePreferences(storage)).toEqual({
            'app-theme': 'mono',
            'hrt-lang': 'tr',
            'app-show-prediction': false,
            'app-hide-site-label': true,
            'hrt-supply-lead-days': 30,
        });

        const bundle = build(seed());
        const imported = JSON.parse(JSON.stringify(bundle)) as CompleteBackupBundle;
        imported.preferences = {
            ...imported.preferences,
            auth_token: 'synthetic-token',
            'app-auto-backup': true,
        } as CompleteBackupBundle['preferences'];
        const target = new MemoryStorage();
        const result = mergeCompleteBackupBundleToStorage(imported, context(target));
        expect(result.preferences).not.toHaveProperty('auth_token');
        expect(result.preferences).not.toHaveProperty('app-auto-backup');
        expect(target.getItem('auth_token')).toBeNull();
        expect(target.getItem('app-auto-backup')).toBeNull();
    });

    it('keeps credentials out of the exported JSON even when storage is seeded with them', () => {
        const bundle = build(seed());
        const serialized = JSON.stringify(bundle);
        expect(serialized).not.toContain('synthetic-token');
        expect(serialized).not.toContain('synthetic-password');
        expect(serialized).not.toContain('synthetic-passes');
        expect(serialized).not.toContain('synthetic-key');
        expect(Object.keys(bundle.preferences).sort()).toEqual(
            ['app-hide-site-label', 'app-show-prediction', 'app-theme', 'hrt-lang', 'hrt-supply-lead-days'].sort(),
        );
    });

    it('does not write anything when the envelope version is unknown', () => {
        const storage = new MemoryStorage();
        const result = inspectCompleteBackupBundle(
            { format: COMPLETE_BACKUP_FORMAT, formatVersion: COMPLETE_BACKUP_FORMAT_VERSION + 1 },
            context(storage),
        );
        expect(result.ok).toBe(false);
        expect(result.errors.join(' ')).toContain('Unsupported backup format version');
        expect(storage.map.size).toBe(0);
    });
});
