/**
 * Complete-device backup bundle: an unencrypted, versioned JSON envelope.
 *
 * The website owns only this data layer. It does not encrypt, decrypt, hold
 * keys, or claim that a bundle is a cold-backup encryption format. Encryption
 * and decryption belong to the separate offline Tools workflow.
 *
 * The bundle is deliberately separate from the v2 records export used for cloud
 * sync. It carries the local-only groups (quick doses, supplies, plan),
 * calibration state, and an allowlisted set of portable preferences. Import is
 * merge-only: category-level validation runs before any write, and a category
 * with an unresolved same-id conflict is blocked rather than guessed. Replace
 * remains BLOCKED because localStorage has no proven multi-key transaction.
 */
import {
    BODY_WEIGHT_KG_MAX,
    BODY_WEIGHT_KG_MIN,
    DOSE_MG_MAX,
    EVENT_TIME_H_MAX,
    EVENT_TIME_H_MIN,
    Ester,
    Route,
    isLabAssay,
    isPlausibleBodyWeightKG,
    normalizeCalibrationMethod,
    sanitizePKParams,
    type CalibrationHistoryMode,
    type CalibrationMethod,
    type DoseEvent,
    type LabResult,
    type PKCustomParams,
} from '../../logic';
import type { AppTheme } from '../constants';
import type { Lang } from '../i18n/translations';
import { type PlanItem } from './plan';
import { type Pack, type Supply } from './regimen';
import {
    loadJSONFromStorage,
    modeKeyFor,
    nsFor,
    readTombstonesFromStorage,
    type DoseTemplate,
    type QuickDose,
    type StorageLike,
} from './backupState';
import { MODE_KEYS, emptyTombstones, type ModeKey, type Tombstones } from './syncMerge';

export const COMPLETE_BACKUP_FORMAT = 'oyama-hrt-complete-backup';
export const COMPLETE_BACKUP_FORMAT_VERSION = 1;

export type PortablePreferenceKey =
    'app-theme' | 'hrt-lang' | 'app-show-prediction' | 'app-hide-site-label' | 'hrt-supply-lead-days';

export const PORTABLE_PREFERENCE_KEYS: readonly PortablePreferenceKey[] = [
    'app-theme',
    'hrt-lang',
    'app-show-prediction',
    'app-hide-site-label',
    'hrt-supply-lead-days',
];

/**
 * Only confirmed keys leave the device. `app-auto-backup`, `app-dev-mode`,
 * onboarding/site-notice flags, and every credential stay excluded by
 * construction.
 */
export interface PortablePreferences {
    'app-theme'?: AppTheme;
    'hrt-lang'?: Lang;
    'app-show-prediction'?: boolean;
    'app-hide-site-label'?: boolean;
    'hrt-supply-lead-days'?: number;
}

export interface CompleteBackupMode {
    events?: DoseEvent[];
    labResults?: LabResult[];
    doseTemplates?: DoseTemplate[];
    deletions?: Tombstones;
    quickDoses?: QuickDose[];
    supplies?: Supply[];
    plan?: PlanItem[];
}

export interface CompleteBackupCalibration {
    method: CalibrationMethod;
    historyMode: CalibrationHistoryMode;
}

export interface CompleteBackupBundle {
    format: typeof COMPLETE_BACKUP_FORMAT;
    formatVersion: typeof COMPLETE_BACKUP_FORMAT_VERSION;
    createdAt: string;
    mode: ModeKey;
    modes: Record<ModeKey, CompleteBackupMode>;
    weight?: number;
    weightUpdatedAt?: number;
    /** Missing = the bundle says nothing; null = explicitly cleared. */
    pkParams?: PKCustomParams | null;
    pkParamsUpdatedAt?: number;
    calibration: CompleteBackupCalibration;
    preferences: PortablePreferences;
}

export interface CompleteBackupActive {
    events: DoseEvent[];
    labResults: LabResult[];
    doseTemplates: DoseTemplate[];
    quickDoses: QuickDose[];
    supplies: Supply[];
    plan: PlanItem[];
    weight: number;
    pkParams: PKCustomParams | null;
    calibrationMethod: CalibrationMethod;
    calibrationHistoryMode: CalibrationHistoryMode;
}

export interface CompleteBackupContext {
    storage: StorageLike;
    owner: string;
    mode: ModeKey;
    active: CompleteBackupActive;
}

export type BundleCategory =
    'events' | 'labResults' | 'doseTemplates' | 'deletions' | 'quickDoses' | 'supplies' | 'plan';

export interface BundleCounts {
    events: number;
    labResults: number;
    doseTemplates: number;
    deletions: number;
    quickDoses: number;
    supplies: number;
    plan: number;
}

export interface CompleteBackupPreview {
    /** False only when the envelope itself cannot be read. */
    ok: boolean;
    errors: string[];
    formatVersion?: number;
    createdAt?: string;
    sourceMode?: ModeKey;
    currentMode: ModeKey;
    counts: Record<ModeKey, BundleCounts>;
    /** Category paths the file does not mention at all. */
    missing: string[];
    /** Category paths that will not be written. */
    blocked: string[];
    issues: string[];
    conflicts: string[];
    stats: { added: number; updated: number; skipped: number };
    preferences: {
        values: PortablePreferences;
        ignored: string[];
        invalid: string[];
    };
    calibration?: CompleteBackupCalibration;
    weight?: number;
    weightUpdatedAt?: number;
    pkParams?: PKCustomParams | null;
    pkParamsUpdatedAt?: number;
}

export interface CompleteBackupMergeResult {
    applied: string[];
    blocked: string[];
    skipped: string[];
    added: number;
    updated: number;
    /** Merged arrays for each mode; only applied categories are present. */
    modes: Partial<Record<ModeKey, CompleteBackupMode>>;
    weight?: number;
    /** Present only when the bundle supplied a value; null means cleared. */
    pkParams?: PKCustomParams | null;
    calibration?: CompleteBackupCalibration;
    /** Allowlisted values only. The caller applies them to React state. */
    preferences: PortablePreferences;
}

interface Validation<T> {
    value?: T;
    error?: string;
}

const ARRAY_CATEGORIES = ['events', 'labResults', 'doseTemplates', 'quickDoses', 'supplies', 'plan'] as const;
type ArrayCategory = (typeof ARRAY_CATEGORIES)[number];

const CATEGORY_SUFFIX: Record<ArrayCategory, string> = {
    events: 'events',
    labResults: 'lab-results',
    doseTemplates: 'dose-templates',
    quickDoses: 'quick-doses',
    supplies: 'supplies',
    plan: 'plan',
};

const STAMPED_CATEGORIES: ReadonlySet<ArrayCategory> = new Set(['events', 'labResults', 'doseTemplates']);

const isObject = (value: unknown): value is Record<string, unknown> =>
    !!value && typeof value === 'object' && !Array.isArray(value);

const isFiniteNumber = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value);

const isRoute = (value: unknown): value is Route =>
    typeof value === 'string' && (Object.values(Route) as string[]).includes(value);

const isKnownEster = (value: unknown): value is Ester =>
    typeof value === 'string' && (Object.values(Ester) as string[]).includes(value);

const isDrugCode = (value: unknown): value is Ester => typeof value === 'string' && /^[A-Za-z0-9_]{1,24}$/.test(value);

const isStamp = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && value > 0;

const readStamp = (value: unknown): Validation<number> => {
    if (value === undefined || value === null) return {};
    if (!isStamp(value)) return { error: 'updatedAt must be a positive finite number' };
    return { value };
};

const readStampString = (value: string | null): number | undefined => {
    if (value === null || value.trim() === '') return undefined;
    const n = Number(value);
    return isStamp(n) ? n : undefined;
};

const isIsoDate = (value: unknown): value is string =>
    typeof value === 'string' && value.trim() !== '' && Number.isFinite(Date.parse(value));

const isAppTheme = (value: unknown): value is AppTheme =>
    value === 'light' || value === 'dark' || value === 'system' || value === 'mono';

const isLang = (value: unknown): value is Lang =>
    value === 'zh' ||
    value === 'zh-TW' ||
    value === 'yue' ||
    value === 'en' ||
    value === 'ja' ||
    value === 'ko' ||
    value === 'tr';

const isCalibrationMethodValue = (value: unknown): value is CalibrationMethod =>
    value === 'off' || value === 'ekf' || value === 'ou_kalman' || value === 'mipd';

const isCalibrationHistoryMode = (value: unknown): value is CalibrationHistoryMode =>
    value === 'forward' || value === 'retrospective';

/**
 * A deterministic JSON form for "same timestamp, same content?" checks, so
 * object key order from another device does not look like an edit.
 */
const canonical = (value: unknown): string => {
    if (Array.isArray(value)) return `[${value.map(canonical).join(',')}]`;
    if (isObject(value)) {
        return `{${Object.keys(value)
            .sort()
            .map(key => `${JSON.stringify(key)}:${canonical(value[key])}`)
            .join(',')}}`;
    }
    return JSON.stringify(value);
};

const recordStamp = (record: unknown): number | undefined => {
    if (!isObject(record)) return undefined;
    return readStamp(record.updatedAt).value;
};

// --- Record validators ------------------------------------------------------

const validateEvent = (raw: unknown, path: string): Validation<DoseEvent> => {
    if (!isObject(raw)) return { error: `${path}: record must be an object` };
    if (typeof raw.id !== 'string' || raw.id === '') return { error: `${path}: id must be a non-empty string` };
    if (!isRoute(raw.route)) return { error: `${path}: unknown route` };
    const timeH = raw.timeH;
    if (!isFiniteNumber(timeH) || timeH < EVENT_TIME_H_MIN || timeH > EVENT_TIME_H_MAX)
        return { error: `${path}: timeH is outside the accepted range` };
    if (!isFiniteNumber(raw.doseMG) || raw.doseMG < 0 || raw.doseMG > DOSE_MG_MAX)
        return { error: `${path}: doseMG is invalid` };
    if (!isDrugCode(raw.ester)) return { error: `${path}: ester must be a drug code` };
    const extras = raw.extras === undefined || raw.extras === null ? {} : raw.extras;
    if (!isObject(extras)) return { error: `${path}: extras must be an object` };
    const stamp = readStamp(raw.updatedAt);
    if (stamp.error) return { error: `${path}: ${stamp.error}` };
    const value: DoseEvent = {
        id: raw.id,
        route: raw.route,
        timeH,
        doseMG: raw.doseMG,
        ester: raw.ester,
        extras: extras as DoseEvent['extras'],
    };
    if (stamp.value !== undefined) value.updatedAt = stamp.value;
    return { value };
};

const validateLabResult = (raw: unknown, path: string): Validation<LabResult> => {
    if (!isObject(raw)) return { error: `${path}: record must be an object` };
    if (typeof raw.id !== 'string' || raw.id === '') return { error: `${path}: id must be a non-empty string` };
    if (!isFiniteNumber(raw.timeH)) return { error: `${path}: timeH must be finite` };
    if (!isFiniteNumber(raw.concValue)) return { error: `${path}: concValue must be finite` };
    if (raw.unit !== 'pg/ml' && raw.unit !== 'pmol/l' && raw.unit !== 'ng/dl' && raw.unit !== 'nmol/l')
        return { error: `${path}: unknown lab unit` };
    if (raw.assay !== undefined && raw.assay !== null && !isLabAssay(raw.assay))
        return { error: `${path}: unknown lab assay` };
    const stamp = readStamp(raw.updatedAt);
    if (stamp.error) return { error: `${path}: ${stamp.error}` };
    const value: LabResult = {
        id: raw.id,
        timeH: raw.timeH,
        concValue: raw.concValue,
        unit: raw.unit,
    };
    if (raw.assay !== undefined && raw.assay !== null && isLabAssay(raw.assay)) value.assay = raw.assay;
    if (stamp.value !== undefined) value.updatedAt = stamp.value;
    return { value };
};

const validateTemplate = (raw: unknown, path: string): Validation<DoseTemplate> => {
    if (!isObject(raw)) return { error: `${path}: record must be an object` };
    if (typeof raw.id !== 'string' || raw.id === '') return { error: `${path}: id must be a non-empty string` };
    if (typeof raw.name !== 'string') return { error: `${path}: name must be a string` };
    if (!isRoute(raw.route)) return { error: `${path}: unknown route` };
    if (!isDrugCode(raw.ester)) return { error: `${path}: ester must be a drug code` };
    if (!isFiniteNumber(raw.doseMG) || raw.doseMG < 0) return { error: `${path}: doseMG is invalid` };
    const extras = raw.extras === undefined || raw.extras === null ? {} : raw.extras;
    if (!isObject(extras)) return { error: `${path}: extras must be an object` };
    if (!isFiniteNumber(raw.createdAt)) return { error: `${path}: createdAt must be finite` };
    const stamp = readStamp(raw.updatedAt);
    if (stamp.error) return { error: `${path}: ${stamp.error}` };
    const value: DoseTemplate = {
        id: raw.id,
        name: raw.name,
        route: raw.route,
        ester: raw.ester,
        doseMG: raw.doseMG,
        extras,
        createdAt: raw.createdAt,
    };
    if (stamp.value !== undefined) value.updatedAt = stamp.value;
    return { value };
};

const validateQuickDose = (raw: unknown, path: string): Validation<QuickDose> => {
    if (!isObject(raw)) return { error: `${path}: record must be an object` };
    if (typeof raw.id !== 'string' || raw.id === '') return { error: `${path}: id must be a non-empty string` };
    if (!isRoute(raw.route)) return { error: `${path}: unknown route` };
    if (!isDrugCode(raw.ester)) return { error: `${path}: ester must be a drug code` };
    if (!isFiniteNumber(raw.value)) return { error: `${path}: value must be finite` };
    if (!isFiniteNumber(raw.createdAt)) return { error: `${path}: createdAt must be finite` };
    return {
        value: {
            id: raw.id,
            route: raw.route,
            ester: raw.ester,
            value: raw.value,
            createdAt: raw.createdAt,
        },
    };
};

/** The production Pack object union. A string pack is never accepted. */
const validatePack = (raw: unknown, path: string): Validation<Pack> => {
    if (!isObject(raw)) return { error: `${path}: pack must be an object, not a string` };
    switch (raw.kind) {
        case 'vial':
            if (!isFiniteNumber(raw.ml) || raw.ml <= 0 || !isFiniteNumber(raw.mgPerMl) || raw.mgPerMl <= 0)
                return { error: `${path}: vial needs positive ml and mgPerMl` };
            return { value: { kind: 'vial', ml: raw.ml, mgPerMl: raw.mgPerMl } };
        case 'pill':
            if (!isFiniteNumber(raw.mg) || raw.mg <= 0) return { error: `${path}: pill needs a positive mg` };
            return { value: { kind: 'pill', mg: raw.mg } };
        case 'pump':
            if (!isFiniteNumber(raw.mg) || raw.mg <= 0) return { error: `${path}: pump needs a positive mg` };
            return { value: { kind: 'pump', mg: raw.mg } };
        case 'sachet':
            if (!isFiniteNumber(raw.mg) || raw.mg <= 0) return { error: `${path}: sachet needs a positive mg` };
            return { value: { kind: 'sachet', mg: raw.mg } };
        case 'patch':
            return { value: { kind: 'patch' } };
        case 'bulk':
            return { value: { kind: 'bulk' } };
        default:
            return { error: `${path}: unknown pack kind` };
    }
};

const validateSupply = (raw: unknown, path: string): Validation<Supply> => {
    if (!isObject(raw)) return { error: `${path}: record must be an object` };
    if (typeof raw.id !== 'string' || raw.id === '') return { error: `${path}: id must be a non-empty string` };
    if (!isRoute(raw.route)) return { error: `${path}: unknown route` };
    if (!isKnownEster(raw.ester)) return { error: `${path}: unknown ester` };
    const pack = validatePack(raw.pack, `${path}.pack`);
    if (pack.value === undefined) return { error: pack.error ?? `${path}: invalid pack` };
    if (!isFiniteNumber(raw.count) || raw.count < 0) return { error: `${path}: count must be finite and >= 0` };
    if (!isFiniteNumber(raw.startedAt) || raw.startedAt <= 0)
        return { error: `${path}: startedAt must be a positive finite epoch-ms` };
    return {
        value: {
            id: raw.id,
            route: raw.route,
            ester: raw.ester,
            pack: pack.value,
            count: raw.count,
            startedAt: raw.startedAt,
        },
    };
};

const validatePlan = (raw: unknown, path: string): Validation<PlanItem> => {
    if (!isObject(raw)) return { error: `${path}: record must be an object` };
    if (typeof raw.id !== 'string' || raw.id === '') return { error: `${path}: id must be a non-empty string` };
    if (!isRoute(raw.route)) return { error: `${path}: unknown route` };
    if (!isKnownEster(raw.ester)) return { error: `${path}: unknown ester` };
    if (!isFiniteNumber(raw.doseMG) || raw.doseMG < 0) return { error: `${path}: doseMG must be finite and >= 0` };
    const extras = raw.extras === undefined || raw.extras === null ? {} : raw.extras;
    if (!isObject(extras)) return { error: `${path}: extras must be an object` };
    const everyDays = raw.everyDays;
    if (!isFiniteNumber(everyDays) || everyDays < 1) return { error: `${path}: everyDays must be finite and >= 1` };
    const timesPerDay = Math.round(Number(raw.timesPerDay));
    if (!(timesPerDay >= 1)) return { error: `${path}: timesPerDay must be >= 1` };
    return {
        value: {
            id: raw.id,
            route: raw.route,
            ester: raw.ester,
            doseMG: raw.doseMG,
            extras: extras as PlanItem['extras'],
            everyDays,
            timesPerDay: everyDays > 1 ? 1 : timesPerDay,
        },
    };
};

// The table deliberately erases each validator's concrete record type; the
// category key is what keeps the arrays separated.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const VALIDATORS: Record<ArrayCategory, (raw: unknown, path: string) => Validation<any>> = {
    events: validateEvent,
    labResults: validateLabResult,
    doseTemplates: validateTemplate,
    quickDoses: validateQuickDose,
    supplies: validateSupply,
    plan: validatePlan,
};

const validateArray = <T>(
    raw: unknown,
    path: string,
    validator: (value: unknown, path: string) => Validation<T>,
): { values: T[]; errors: string[] } => {
    if (!Array.isArray(raw)) return { values: [], errors: [`${path} must be an array`] };
    const values: T[] = [];
    const errors: string[] = [];
    raw.forEach((item, index) => {
        const result = validator(item, `${path}[${index}]`);
        if (result.value !== undefined) values.push(result.value);
        else errors.push(result.error ?? `${path}[${index}]: invalid record`);
    });
    return { values, errors };
};

const validateTombstoneMap = (raw: unknown, path: string): { value: Record<string, number>; errors: string[] } => {
    if (raw === undefined || raw === null) return { value: {}, errors: [] };
    if (!isObject(raw)) return { value: {}, errors: [`${path} must be an object`] };
    const value: Record<string, number> = {};
    const errors: string[] = [];
    for (const [id, at] of Object.entries(raw)) {
        if (id === '') errors.push(`${path}: tombstone id must not be empty`);
        else if (!isStamp(at)) errors.push(`${path}.${id}: tombstone time must be a positive finite number`);
        else value[id] = at;
    }
    return { value, errors };
};

const validateTombstones = (raw: unknown, path: string): { value: Tombstones; errors: string[] } => {
    if (raw === undefined || raw === null) return { value: emptyTombstones(), errors: [] };
    if (!isObject(raw)) return { value: emptyTombstones(), errors: [`${path} must be an object`] };
    const events = validateTombstoneMap(raw.events, `${path}.events`);
    const labResults = validateTombstoneMap(raw.labResults, `${path}.labResults`);
    const doseTemplates = validateTombstoneMap(raw.doseTemplates, `${path}.doseTemplates`);
    return {
        value: { events: events.value, labResults: labResults.value, doseTemplates: doseTemplates.value },
        errors: [...events.errors, ...labResults.errors, ...doseTemplates.errors],
    };
};

const validateCalibration = (raw: unknown): { value?: CompleteBackupCalibration; errors: string[] } => {
    if (!isObject(raw)) return { errors: ['calibration must be an object'] };
    const errors: string[] = [];
    let method: CalibrationMethod | undefined;
    if (raw.method === 'average' || raw.method === 'adaptive' || isCalibrationMethodValue(raw.method)) {
        method = normalizeCalibrationMethod(raw.method as string);
    } else {
        errors.push('calibration.method is invalid');
    }
    let historyMode: CalibrationHistoryMode | undefined;
    if (isCalibrationHistoryMode(raw.historyMode)) historyMode = raw.historyMode;
    else errors.push('calibration.historyMode is invalid');
    if (!method || !historyMode) return { errors };
    return { value: { method, historyMode }, errors };
};

export function sanitizePortablePreferences(raw: unknown): {
    values: PortablePreferences;
    ignored: string[];
    invalid: string[];
} {
    const values: PortablePreferences = {};
    const ignored: string[] = [];
    const invalid: string[] = [];
    if (raw === undefined || raw === null) return { values, ignored, invalid };
    if (!isObject(raw)) {
        invalid.push('preferences');
        return { values, ignored, invalid };
    }
    for (const [key, value] of Object.entries(raw)) {
        switch (key) {
            case 'app-theme':
                if (isAppTheme(value)) values['app-theme'] = value;
                else invalid.push(key);
                break;
            case 'hrt-lang':
                if (isLang(value)) values['hrt-lang'] = value;
                else invalid.push(key);
                break;
            case 'app-show-prediction':
                if (typeof value === 'boolean') values['app-show-prediction'] = value;
                else invalid.push(key);
                break;
            case 'app-hide-site-label':
                if (typeof value === 'boolean') values['app-hide-site-label'] = value;
                else invalid.push(key);
                break;
            case 'hrt-supply-lead-days':
                if (isFiniteNumber(value) && value > 0 && value <= 3650) values['hrt-supply-lead-days'] = value;
                else invalid.push(key);
                break;
            default:
                // Unconfirmed, device-local, behavioural, and credential keys are
                // reported but never applied.
                ignored.push(key);
                break;
        }
    }
    return { values, ignored, invalid };
}

export function readPortablePreferences(storage: StorageLike): PortablePreferences {
    const values: PortablePreferences = {};
    const theme = storage.getItem('app-theme');
    if (isAppTheme(theme)) values['app-theme'] = theme;
    const lang = storage.getItem('hrt-lang');
    if (isLang(lang)) values['hrt-lang'] = lang;
    const prediction = storage.getItem('app-show-prediction');
    if (prediction === 'true' || prediction === 'false') values['app-show-prediction'] = prediction === 'true';
    const hideSiteLabel = storage.getItem('app-hide-site-label');
    if (hideSiteLabel === 'true' || hideSiteLabel === 'false') values['app-hide-site-label'] = hideSiteLabel === 'true';
    const leadDays = storage.getItem('hrt-supply-lead-days');
    if (leadDays !== null) {
        const n = Number(leadDays);
        if (isFiniteNumber(n) && n > 0 && n <= 3650) values['hrt-supply-lead-days'] = n;
    }
    return values;
}

const cleanActiveRecords = <T>(items: unknown[], validator: (value: unknown, path: string) => Validation<T>): T[] => {
    const values: T[] = [];
    items.forEach((item, index) => {
        const result = validator(item, `[${index}]`);
        if (result.value !== undefined) values.push(result.value);
    });
    return values;
};

const readStoredRecords = <T>(storage: StorageLike, owner: string, mode: ModeKey, category: ArrayCategory): T[] => {
    const raw = loadJSONFromStorage<unknown>(storage, modeKeyFor(owner, mode, CATEGORY_SUFFIX[category]), []);
    if (!Array.isArray(raw)) return [];
    return cleanActiveRecords(raw, VALIDATORS[category]);
};

/**
 * Build a complete bundle from the current store, overlaying the active mode's
 * in-memory arrays (which can be newer than the persisted copy).
 */
export function buildCompleteBackupBundleFromStorage(
    storage: StorageLike,
    owner: string,
    activeMode: ModeKey,
    active: CompleteBackupActive,
    createdAt = new Date().toISOString(),
): CompleteBackupBundle {
    const activeArrays: Record<ArrayCategory, unknown[]> = {
        events: active.events,
        labResults: active.labResults,
        doseTemplates: active.doseTemplates,
        quickDoses: active.quickDoses,
        supplies: active.supplies,
        plan: active.plan,
    };

    const readMode = (mode: ModeKey): CompleteBackupMode => {
        const block: CompleteBackupMode = {};
        for (const category of ARRAY_CATEGORIES) {
            block[category] =
                mode === activeMode
                    ? cleanActiveRecords(activeArrays[category], VALIDATORS[category])
                    : readStoredRecords(storage, owner, mode, category);
        }
        block.deletions = readTombstonesFromStorage(storage, owner, mode);
        return block;
    };

    const weightKey = nsFor(owner, 'weight');
    const weightUpdatedAt = readStampString(storage.getItem(nsFor(owner, 'weight-at')));
    const weight =
        storage.getItem(weightKey) !== null && isPlausibleBodyWeightKG(active.weight) ? active.weight : undefined;

    const pkKey = nsFor(owner, 'pk-params');
    const pkUpdatedAt = readStampString(storage.getItem(nsFor(owner, 'pk-params-at')));
    let pkParams: PKCustomParams | null | undefined;
    if (storage.getItem(pkKey) !== null || active.pkParams !== null) {
        const saved = loadJSONFromStorage<unknown>(storage, pkKey, null);
        pkParams = sanitizePKParams(saved ?? active.pkParams);
    } else if (pkUpdatedAt !== undefined) {
        // The key is removed when the user clears overrides; the timestamp is
        // the evidence that this was an explicit clear, not an old device.
        pkParams = null;
    }

    return {
        format: COMPLETE_BACKUP_FORMAT,
        formatVersion: COMPLETE_BACKUP_FORMAT_VERSION,
        createdAt,
        mode: activeMode,
        modes: { transfem: readMode('transfem'), transmasc: readMode('transmasc') },
        ...(weight !== undefined ? { weight } : {}),
        ...(weightUpdatedAt !== undefined ? { weightUpdatedAt } : {}),
        ...(pkParams !== undefined ? { pkParams } : {}),
        ...(pkUpdatedAt !== undefined ? { pkParamsUpdatedAt: pkUpdatedAt } : {}),
        calibration: {
            method: active.calibrationMethod,
            historyMode: active.calibrationHistoryMode,
        },
        preferences: readPortablePreferences(storage),
    };
}

interface ParsedBundle {
    errors: string[];
    createdAt?: string;
    formatVersion?: number;
    sourceMode?: ModeKey;
    modes: Partial<Record<ModeKey, CompleteBackupMode>>;
    categoryErrors: Record<string, string[]>;
    categoryCounts: Record<string, number>;
    missing: string[];
    calibration?: CompleteBackupCalibration;
    preferences: { values: PortablePreferences; ignored: string[]; invalid: string[] };
    weight?: number;
    weightUpdatedAt?: number;
    pkParams?: PKCustomParams | null;
    pkParamsUpdatedAt?: number;
}

const parseBundle = (raw: unknown): ParsedBundle => {
    const errors: string[] = [];
    const categoryErrors: Record<string, string[]> = {};
    const categoryCounts: Record<string, number> = {};
    const missing: string[] = [];
    const modes: Partial<Record<ModeKey, CompleteBackupMode>> = {};
    const preferences = sanitizePortablePreferences(undefined);

    if (!isObject(raw)) {
        return {
            errors: ['Backup root must be an object'],
            modes,
            categoryErrors,
            categoryCounts,
            missing,
            preferences,
        };
    }
    if (raw.format !== COMPLETE_BACKUP_FORMAT) errors.push('Unknown backup format');
    const formatVersion = raw.formatVersion;
    if (formatVersion !== COMPLETE_BACKUP_FORMAT_VERSION) {
        errors.push(
            typeof formatVersion === 'number'
                ? `Unsupported backup format version: ${formatVersion}`
                : 'Missing backup format version',
        );
    }
    if (!isIsoDate(raw.createdAt)) errors.push('createdAt must be an ISO date string');
    if (raw.mode !== 'transfem' && raw.mode !== 'transmasc') errors.push('mode must be transfem or transmasc');
    if (!isObject(raw.modes)) {
        errors.push('modes must be an object');
    }

    // A version we cannot read is not previewed further.
    if (errors.length > 0) {
        return {
            errors,
            createdAt: isIsoDate(raw.createdAt) ? raw.createdAt : undefined,
            formatVersion: typeof formatVersion === 'number' ? formatVersion : undefined,
            sourceMode: raw.mode === 'transfem' || raw.mode === 'transmasc' ? raw.mode : undefined,
            modes,
            categoryErrors,
            categoryCounts,
            missing,
            preferences,
        };
    }

    const modesRaw = raw.modes as Record<string, unknown>;
    for (const mode of MODE_KEYS) {
        const blockRaw = modesRaw[mode];
        if (!isObject(blockRaw)) {
            errors.push(`modes.${mode} must be an object`);
            continue;
        }
        const block: CompleteBackupMode = {};
        for (const category of ARRAY_CATEGORIES) {
            const path = `${mode}.${category}`;
            if (blockRaw[category] === undefined) {
                missing.push(path);
                continue;
            }
            categoryCounts[path] = Array.isArray(blockRaw[category]) ? blockRaw[category].length : 0;
            const result = validateArray(blockRaw[category], path, VALIDATORS[category]);
            if (result.errors.length > 0) {
                categoryErrors[path] = result.errors;
                continue;
            }
            block[category] = result.values;
        }
        if (blockRaw.deletions === undefined) {
            missing.push(`${mode}.deletions`);
        } else {
            if (isObject(blockRaw.deletions)) {
                categoryCounts[`${mode}.deletions`] = ['events', 'labResults', 'doseTemplates'].reduce(
                    (total, kind) => {
                        const map = blockRaw.deletions[kind];
                        return total + (isObject(map) ? Object.keys(map).length : 0);
                    },
                    0,
                );
            }
            const tombstones = validateTombstones(blockRaw.deletions, `${mode}.deletions`);
            if (tombstones.errors.length > 0) categoryErrors[`${mode}.deletions`] = tombstones.errors;
            else block.deletions = tombstones.value;
        }
        // `planItems` is accepted as a read alias for `plan`; the writer always
        // emits `plan`.
        if (blockRaw.plan === undefined && blockRaw.planItems !== undefined) {
            const index = missing.indexOf(`${mode}.plan`);
            if (index >= 0) missing.splice(index, 1);
            categoryCounts[`${mode}.plan`] = Array.isArray(blockRaw.planItems) ? blockRaw.planItems.length : 0;
            const result = validateArray(blockRaw.planItems, `${mode}.planItems`, validatePlan);
            if (result.errors.length > 0) categoryErrors[`${mode}.plan`] = result.errors;
            else block.plan = result.values;
        }
        modes[mode] = block;
    }

    let calibration: CompleteBackupCalibration | undefined;
    if (raw.calibration === undefined) {
        missing.push('calibration');
        categoryErrors.calibration = ['calibration is missing'];
    } else {
        const result = validateCalibration(raw.calibration);
        if (result.value) calibration = result.value;
        else categoryErrors.calibration = result.errors;
    }

    if (raw.preferences !== undefined) {
        preferences.values = sanitizePortablePreferences(raw.preferences).values;
        preferences.ignored = sanitizePortablePreferences(raw.preferences).ignored;
        preferences.invalid = sanitizePortablePreferences(raw.preferences).invalid;
    } else {
        missing.push('preferences');
    }

    let weight: number | undefined;
    let weightUpdatedAt: number | undefined;
    if (raw.weight !== undefined) {
        if (isFiniteNumber(raw.weight) && raw.weight >= BODY_WEIGHT_KG_MIN && raw.weight <= BODY_WEIGHT_KG_MAX)
            weight = raw.weight;
        else categoryErrors.weight = ['weight must be a plausible body weight in kg'];
    } else {
        missing.push('weight');
    }
    if (raw.weightUpdatedAt !== undefined) {
        const stamp = readStamp(raw.weightUpdatedAt);
        if (stamp.error) categoryErrors.weight = [...(categoryErrors.weight ?? []), 'weightUpdatedAt is invalid'];
        else weightUpdatedAt = stamp.value;
    }

    let pkParams: PKCustomParams | null | undefined;
    let pkParamsUpdatedAt: number | undefined;
    if (raw.pkParams !== undefined) {
        if (raw.pkParams === null) {
            pkParams = null;
        } else {
            const sanitized = sanitizePKParams(raw.pkParams);
            if (sanitized === null) categoryErrors.pkParams = ['pkParams contains no usable numeric values'];
            else pkParams = sanitized;
        }
    } else {
        missing.push('pkParams');
    }
    if (raw.pkParamsUpdatedAt !== undefined) {
        const stamp = readStamp(raw.pkParamsUpdatedAt);
        if (stamp.error) categoryErrors.pkParams = [...(categoryErrors.pkParams ?? []), 'pkParamsUpdatedAt is invalid'];
        else pkParamsUpdatedAt = stamp.value;
    }

    return {
        errors,
        createdAt: raw.createdAt as string,
        formatVersion: raw.formatVersion as number,
        sourceMode: raw.mode as ModeKey,
        modes,
        categoryErrors,
        categoryCounts,
        missing,
        calibration,
        preferences,
        weight,
        weightUpdatedAt,
        pkParams,
        pkParamsUpdatedAt,
    };
};

const zeroCounts = (): BundleCounts => ({
    events: 0,
    labResults: 0,
    doseTemplates: 0,
    deletions: 0,
    quickDoses: 0,
    supplies: 0,
    plan: 0,
});

const countTombstones = (t: Tombstones): number =>
    Object.keys(t.events).length + Object.keys(t.labResults).length + Object.keys(t.doseTemplates).length;

const localArrayFor = (context: CompleteBackupContext, mode: ModeKey, category: ArrayCategory): unknown[] => {
    if (mode === context.mode) return context.active[category];
    const raw = loadJSONFromStorage<unknown>(
        context.storage,
        modeKeyFor(context.owner, mode, CATEGORY_SUFFIX[category]),
        [],
    );
    return Array.isArray(raw) ? raw : [];
};

const duplicateIds = (records: { id: string }[]): string[] => {
    const seen = new Set<string>();
    const duplicates = new Set<string>();
    for (const record of records) {
        if (seen.has(record.id)) duplicates.add(record.id);
        else seen.add(record.id);
    }
    return [...duplicates];
};

/**
 * Validate a bundle and compare it with the current store without writing
 * anything. Counts and conflicts are what the confirmation dialog shows.
 */
export function inspectCompleteBackupBundle(raw: unknown, context: CompleteBackupContext): CompleteBackupPreview {
    const parsed = parseBundle(raw);
    const counts: Record<ModeKey, BundleCounts> = { transfem: zeroCounts(), transmasc: zeroCounts() };
    const blocked = new Set<string>();
    const issues: string[] = [];
    const conflicts: string[] = [];
    let added = 0;
    let updated = 0;
    let skipped = 0;

    for (const [path, errors] of Object.entries(parsed.categoryErrors)) {
        blocked.add(path);
        issues.push(...errors);
    }

    for (const mode of MODE_KEYS) {
        const block = parsed.modes[mode];
        if (!block) continue;
        for (const category of ARRAY_CATEGORIES) {
            const path = `${mode}.${category}`;
            const records = block[category];
            counts[mode][category] = parsed.categoryCounts[path] ?? records?.length ?? 0;
            if (!records) continue;
            if (blocked.has(path)) continue;

            const duplicates = duplicateIds(records as { id: string }[]);
            if (duplicates.length) {
                blocked.add(path);
                issues.push(`${path}: duplicate id ${duplicates.join(', ')}`);
                continue;
            }

            const local = localArrayFor(context, mode, category);
            const localById = new Map<string, unknown>();
            for (const record of local) {
                if (!isObject(record) || typeof record.id !== 'string') continue;
                if (localById.has(record.id)) {
                    blocked.add(path);
                    issues.push(`${path}: local data already contains duplicate id ${record.id}`);
                    break;
                }
                localById.set(record.id, record);
            }
            if (blocked.has(path)) continue;

            for (const record of records as { id: string }[]) {
                const existing = localById.get(record.id);
                if (!existing) {
                    added++;
                    continue;
                }
                if (!STAMPED_CATEGORIES.has(category)) {
                    conflicts.push(`${path}: id ${record.id} already exists and this category has no updatedAt`);
                    blocked.add(path);
                    continue;
                }
                const incomingStamp = recordStamp(record);
                const localStamp = recordStamp(existing);
                if (incomingStamp === undefined || localStamp === undefined) {
                    conflicts.push(`${path}: id ${record.id} exists on both sides without a reliable updatedAt`);
                    blocked.add(path);
                } else if (incomingStamp > localStamp) {
                    updated++;
                } else if (incomingStamp < localStamp) {
                    skipped++;
                } else if (canonical(record) === canonical(existing)) {
                    skipped++;
                } else {
                    conflicts.push(`${path}: id ${record.id} has equal updatedAt but different content`);
                    blocked.add(path);
                }
            }
        }

        const tombstones = block.deletions;
        if (tombstones)
            counts[mode].deletions = parsed.categoryCounts[`${mode}.deletions`] ?? countTombstones(tombstones);
    }

    if (parsed.weight !== undefined && !blocked.has('weight')) {
        const localWeight = context.storage.getItem(nsFor(context.owner, 'weight'));
        const localStamp = readStampString(context.storage.getItem(nsFor(context.owner, 'weight-at')));
        if (localWeight !== null) {
            if (parsed.weightUpdatedAt === undefined || localStamp === undefined) {
                if (Number(localWeight) !== parsed.weight) {
                    conflicts.push('weight exists on both sides without a reliable timestamp');
                    blocked.add('weight');
                }
            } else if (parsed.weightUpdatedAt < localStamp) {
                skipped++;
            } else if (parsed.weightUpdatedAt > localStamp) {
                updated++;
            }
        }
    }

    if (parsed.pkParams !== undefined && !blocked.has('pkParams')) {
        const localPk = context.storage.getItem(nsFor(context.owner, 'pk-params'));
        const localStamp = readStampString(context.storage.getItem(nsFor(context.owner, 'pk-params-at')));
        if (localPk !== null) {
            if (parsed.pkParamsUpdatedAt === undefined || localStamp === undefined) {
                let same: boolean;
                try {
                    same = canonical(JSON.parse(localPk)) === canonical(parsed.pkParams);
                } catch {
                    same = false;
                }
                if (!same) {
                    conflicts.push('pkParams exists on both sides without a reliable timestamp');
                    blocked.add('pkParams');
                }
            } else if (parsed.pkParamsUpdatedAt < localStamp) {
                skipped++;
            } else if (parsed.pkParamsUpdatedAt > localStamp) {
                updated++;
            }
        }
    }

    return {
        ok: parsed.errors.length === 0,
        errors: parsed.errors,
        formatVersion: parsed.formatVersion,
        createdAt: parsed.createdAt,
        sourceMode: parsed.sourceMode,
        currentMode: context.mode,
        counts,
        missing: parsed.missing,
        blocked: [...blocked],
        issues,
        conflicts,
        stats: { added, updated, skipped },
        preferences: parsed.preferences,
        calibration: parsed.calibration,
        weight: parsed.weight,
        weightUpdatedAt: parsed.weightUpdatedAt,
        pkParams: parsed.pkParams,
        pkParamsUpdatedAt: parsed.pkParamsUpdatedAt,
    };
}

const mergeTombstoneMaps = (local: Record<string, number>, incoming: Record<string, number>) => {
    const out = { ...local };
    for (const [id, at] of Object.entries(incoming)) out[id] = Math.max(out[id] ?? 0, at);
    return out;
};

/**
 * Merge the safe categories. A blocked category is left exactly as it was.
 * This is merge-only and intentionally does not claim replace/all-or-nothing:
 * localStorage has no multi-key transaction.
 */
export function mergeCompleteBackupBundleToStorage(
    raw: unknown,
    context: CompleteBackupContext,
): CompleteBackupMergeResult {
    const preview = inspectCompleteBackupBundle(raw, context);
    if (!preview.ok) throw new Error(`Invalid complete backup: ${preview.errors.join('; ')}`);
    const parsed = parseBundle(raw);
    const blocked = new Set(preview.blocked);
    const result: CompleteBackupMergeResult = {
        applied: [],
        blocked: [...blocked],
        skipped: [],
        added: 0,
        updated: 0,
        modes: {},
        preferences: preview.preferences.values,
    };

    for (const mode of MODE_KEYS) {
        const block = parsed.modes[mode];
        if (!block) continue;
        const modeResult: CompleteBackupMode = {};

        for (const category of ARRAY_CATEGORIES) {
            const path = `${mode}.${category}`;
            const incoming = block[category] as { id: string }[] | undefined;
            if (!incoming || blocked.has(path)) continue;

            const local = localArrayFor(context, mode, category) as { id: string }[];
            const localById = new Map<string, { id: string }>();
            for (const record of local) {
                if (isObject(record) && typeof record.id === 'string')
                    localById.set(record.id, record as { id: string });
            }

            const merged: { id: string }[] = [...local];
            for (const record of incoming) {
                const existing = localById.get(record.id);
                if (!existing) {
                    merged.push(record);
                    result.added++;
                    continue;
                }
                if (!STAMPED_CATEGORIES.has(category)) continue;
                const incomingStamp = recordStamp(record);
                const localStamp = recordStamp(existing);
                if (incomingStamp !== undefined && localStamp !== undefined && incomingStamp > localStamp) {
                    const index = merged.findIndex(item => item.id === record.id);
                    if (index >= 0) merged[index] = record;
                    result.updated++;
                }
            }

            context.storage.setItem(modeKeyFor(context.owner, mode, CATEGORY_SUFFIX[category]), JSON.stringify(merged));
            (modeResult as Record<string, unknown>)[category] = merged;
            result.applied.push(path);
        }

        // Tombstones are unioned (explicit deletions are never guessed). A
        // record the bundle restores clears any local tombstone for that id.
        let tombstones = readTombstonesFromStorage(context.storage, context.owner, mode);
        if (block.deletions && !blocked.has(`${mode}.deletions`)) {
            tombstones = {
                events: mergeTombstoneMaps(tombstones.events, block.deletions.events),
                labResults: mergeTombstoneMaps(tombstones.labResults, block.deletions.labResults),
                doseTemplates: mergeTombstoneMaps(tombstones.doseTemplates, block.deletions.doseTemplates),
            };
        }
        let tombstoneChanged = block.deletions !== undefined && !blocked.has(`${mode}.deletions`);
        for (const category of STAMPED_CATEGORIES) {
            const incoming = block[category] as { id: string }[] | undefined;
            if (!incoming || blocked.has(`${mode}.${category}`)) continue;
            const kind = category;
            for (const record of incoming) {
                if (tombstones[kind][record.id] !== undefined) {
                    delete tombstones[kind][record.id];
                    tombstoneChanged = true;
                }
            }
        }
        if (tombstoneChanged) {
            // Preserve the bundle's tombstones exactly. The sync writer prunes by
            // age; a restore must not silently drop a deletion it was given.
            context.storage.setItem(modeKeyFor(context.owner, mode, 'deletions'), JSON.stringify(tombstones));
            modeResult.deletions = tombstones;
            result.applied.push(`${mode}.deletions`);
        }

        result.modes[mode] = modeResult;
    }

    // Weight: only a newer reliable stamp may overwrite an existing value.
    if (preview.weight !== undefined && !blocked.has('weight')) {
        const localWeight = context.storage.getItem(nsFor(context.owner, 'weight'));
        const localStamp = readStampString(context.storage.getItem(nsFor(context.owner, 'weight-at')));
        const incomingStamp = preview.weightUpdatedAt;
        const canApply =
            localWeight === null ||
            (incomingStamp !== undefined && (localStamp === undefined || incomingStamp > localStamp));
        if (canApply) {
            context.storage.setItem(nsFor(context.owner, 'weight'), String(preview.weight));
            if (incomingStamp !== undefined)
                context.storage.setItem(nsFor(context.owner, 'weight-at'), String(incomingStamp));
            else context.storage.removeItem(nsFor(context.owner, 'weight-at'));
            result.weight = preview.weight;
            result.applied.push('weight');
        } else {
            result.skipped.push('weight');
        }
    }

    // PK overrides: same rule. `null` is an explicit clear.
    if (preview.pkParams !== undefined && !blocked.has('pkParams')) {
        const localPk = context.storage.getItem(nsFor(context.owner, 'pk-params'));
        const localStamp = readStampString(context.storage.getItem(nsFor(context.owner, 'pk-params-at')));
        const incomingStamp = preview.pkParamsUpdatedAt;
        const canApply =
            localPk === null ||
            (incomingStamp !== undefined && (localStamp === undefined || incomingStamp > localStamp));
        if (canApply) {
            if (preview.pkParams === null) context.storage.removeItem(nsFor(context.owner, 'pk-params'));
            else context.storage.setItem(nsFor(context.owner, 'pk-params'), JSON.stringify(preview.pkParams));
            if (incomingStamp !== undefined)
                context.storage.setItem(nsFor(context.owner, 'pk-params-at'), String(incomingStamp));
            else context.storage.removeItem(nsFor(context.owner, 'pk-params-at'));
            result.pkParams = preview.pkParams;
            result.applied.push('pkParams');
        } else {
            result.skipped.push('pkParams');
        }
    }

    if (preview.calibration && !blocked.has('calibration')) {
        context.storage.setItem(nsFor(context.owner, 'cal-method'), preview.calibration.method);
        context.storage.setItem(nsFor(context.owner, 'cal-history-mode'), preview.calibration.historyMode);
        result.calibration = preview.calibration;
        result.applied.push('calibration');
    } else if (blocked.has('calibration')) {
        result.skipped.push('calibration');
    }

    return result;
}
