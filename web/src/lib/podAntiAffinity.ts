export const POD_ANTI_AFFINITY_FIELD_PATH = 'spec.template.spec.affinity.podAntiAffinity' as const;

export const POD_ANTI_AFFINITY_MODES = ['preferred', 'required'] as const;
export type PodAntiAffinityMode = (typeof POD_ANTI_AFFINITY_MODES)[number];

export const POD_ANTI_AFFINITY_OPERATORS = [
    'In',
    'NotIn',
    'Exists',
    'DoesNotExist',
] as const;
export type PodAntiAffinityOperator = (typeof POD_ANTI_AFFINITY_OPERATORS)[number];

const DEFAULT_POD_ANTI_AFFINITY_MODE: PodAntiAffinityMode = 'required';
const DEFAULT_POD_ANTI_AFFINITY_OPERATOR: PodAntiAffinityOperator = 'In';
const DEFAULT_POD_ANTI_AFFINITY_WEIGHT = 100;
const DEFAULT_POD_ANTI_AFFINITY_KEY = 'shepherd.io/service-id';
const DEFAULT_POD_ANTI_AFFINITY_TOPOLOGY_KEY = 'kubernetes.io/hostname';
export const DEFAULT_POD_ANTI_AFFINITY_VALUE = '__SHEPHERD_SERVICE_ID__';

export interface PodAntiAffinityRule {
    mode: PodAntiAffinityMode;
    key: string;
    operator: PodAntiAffinityOperator;
    values: string[];
    topologyKey: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
    return value !== null && typeof value === 'object' && !Array.isArray(value);
}

export function operatorUsesValues(operator: PodAntiAffinityOperator): boolean {
    return operator === 'In' || operator === 'NotIn';
}

function normalizePodAntiAffinityValues(values: string[]): string[] {
    return Array.from(
        new Set(
            values
                .map((value) => value.trim())
                .filter((value) => value.length > 0),
        ),
    );
}

export function parsePodAntiAffinityValuesText(value: string): string[] {
    return normalizePodAntiAffinityValues(value.split(/[\n,]+/));
}

export function stringifyPodAntiAffinityValues(values: string[]): string {
    return normalizePodAntiAffinityValues(values).join(', ');
}

export function createDefaultPodAntiAffinityRule(
    overrides: Partial<PodAntiAffinityRule> = {},
): PodAntiAffinityRule {
    const operator = overrides.operator ?? DEFAULT_POD_ANTI_AFFINITY_OPERATOR;
    const fallbackValues = operatorUsesValues(operator) ? [DEFAULT_POD_ANTI_AFFINITY_VALUE] : [];

    return {
        mode: overrides.mode ?? DEFAULT_POD_ANTI_AFFINITY_MODE,
        key: overrides.key ?? DEFAULT_POD_ANTI_AFFINITY_KEY,
        operator,
        values: normalizePodAntiAffinityValues(overrides.values ?? fallbackValues),
        topologyKey: overrides.topologyKey ?? DEFAULT_POD_ANTI_AFFINITY_TOPOLOGY_KEY,
    };
}

function isPodAntiAffinityOperator(value: unknown): value is PodAntiAffinityOperator {
    return typeof value === 'string' && POD_ANTI_AFFINITY_OPERATORS.includes(value as PodAntiAffinityOperator);
}

export function buildPodAntiAffinity(rule: PodAntiAffinityRule): Record<string, unknown> {
    const normalizedValues = operatorUsesValues(rule.operator)
        ? normalizePodAntiAffinityValues(rule.values)
        : [];

    const matchExpression: Record<string, unknown> = {
        key: rule.key,
        operator: rule.operator,
    };
    if (normalizedValues.length > 0) {
        matchExpression.values = normalizedValues;
    }

    const podAffinityTerm = {
        labelSelector: {
            matchExpressions: [matchExpression],
        },
        topologyKey: rule.topologyKey,
    };

    if (rule.mode === 'required') {
        return {
            requiredDuringSchedulingIgnoredDuringExecution: [podAffinityTerm],
        };
    }

    return {
        preferredDuringSchedulingIgnoredDuringExecution: [
            {
                weight: DEFAULT_POD_ANTI_AFFINITY_WEIGHT,
                podAffinityTerm,
            },
        ],
    };
}

export function buildServiceSpreadPodAntiAffinity(
    overrides: Partial<PodAntiAffinityRule> = {},
): Record<string, unknown> {
    return buildPodAntiAffinity(createDefaultPodAntiAffinityRule(overrides));
}

export function parsePodAntiAffinityRule(value: unknown): PodAntiAffinityRule | null {
    if (!isRecord(value)) {
        return null;
    }

    const preferredRules = value.preferredDuringSchedulingIgnoredDuringExecution;
    if (Array.isArray(preferredRules) && preferredRules.length > 0) {
        const preferred = preferredRules[0];
        if (!isRecord(preferred)) {
            return null;
        }
        return parsePodAntiAffinityTerm(preferred.podAffinityTerm, 'preferred');
    }

    const requiredRules = value.requiredDuringSchedulingIgnoredDuringExecution;
    if (Array.isArray(requiredRules) && requiredRules.length > 0) {
        return parsePodAntiAffinityTerm(requiredRules[0], 'required');
    }

    return null;
}

function parsePodAntiAffinityTerm(
    value: unknown,
    mode: PodAntiAffinityMode,
): PodAntiAffinityRule | null {
    if (!isRecord(value)) {
        return null;
    }

    const labelSelector = value.labelSelector;
    if (!isRecord(labelSelector)) {
        return null;
    }
    const matchExpressions = labelSelector.matchExpressions;
    if (!Array.isArray(matchExpressions) || matchExpressions.length === 0) {
        return null;
    }

    const firstExpression = matchExpressions[0];
    if (!isRecord(firstExpression)) {
        return null;
    }

    const operator = isPodAntiAffinityOperator(firstExpression.operator)
        ? firstExpression.operator
        : DEFAULT_POD_ANTI_AFFINITY_OPERATOR;
    const values = Array.isArray(firstExpression.values)
        ? firstExpression.values.filter((item): item is string => typeof item === 'string')
        : [];

    return createDefaultPodAntiAffinityRule({
        mode,
        key: typeof firstExpression.key === 'string' ? firstExpression.key : undefined,
        operator,
        values,
        topologyKey: typeof value.topologyKey === 'string' ? value.topologyKey : undefined,
    });
}
