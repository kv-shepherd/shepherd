import type { TFunction } from 'i18next';

import type { InstanceSize, Template } from '@/features/vm-management/types';

export const SYSTEM_LABEL_OS_ANY = 'os:any';
const SYSTEM_LABEL_OS_LINUX = 'os:linux';
const SYSTEM_LABEL_OS_WINDOWS = 'os:windows';

type SystemLabel =
  | typeof SYSTEM_LABEL_OS_ANY
  | typeof SYSTEM_LABEL_OS_LINUX
  | typeof SYSTEM_LABEL_OS_WINDOWS;

export const SYSTEM_LABEL_OPTIONS: Array<{ value: SystemLabel; labelKey: string; color: string }> = [
  { value: SYSTEM_LABEL_OS_ANY, labelKey: 'systemLabels.os_any', color: 'default' },
  { value: SYSTEM_LABEL_OS_LINUX, labelKey: 'systemLabels.os_linux', color: 'green' },
  { value: SYSTEM_LABEL_OS_WINDOWS, labelKey: 'systemLabels.os_windows', color: 'blue' },
];

const SYSTEM_LABEL_VALUES = new Set<string>(SYSTEM_LABEL_OPTIONS.map((option) => option.value));

export function normalizeSystemLabels(labels?: readonly string[]): SystemLabel[] {
  const normalized = Array.from(
    new Set(
      (labels ?? [])
        .map((label) => label.trim().toLowerCase())
        .filter((label): label is SystemLabel => SYSTEM_LABEL_VALUES.has(label)),
    ),
  );
  if (normalized.length === 0) {
    return [SYSTEM_LABEL_OS_ANY];
  }
  if (normalized.includes(SYSTEM_LABEL_OS_ANY)) {
    return [SYSTEM_LABEL_OS_ANY];
  }
  return SYSTEM_LABEL_OPTIONS
    .map((option) => option.value)
    .filter((value) => normalized.includes(value));
}

export function normalizeTemplateSystemLabelSelection(labels?: readonly string[]): SystemLabel[] {
  const normalized = normalizeSelectionLabels(labels);
  if (normalized.length === 0) {
    return [SYSTEM_LABEL_OS_ANY];
  }
  const last = normalized[normalized.length - 1];
  if (last === SYSTEM_LABEL_OS_ANY) {
    return [SYSTEM_LABEL_OS_ANY];
  }
  return [last];
}

export function normalizeSizeSystemLabelSelection(labels?: readonly string[]): SystemLabel[] {
  const normalized = normalizeSelectionLabels(labels);
  if (normalized.length === 0) {
    return [SYSTEM_LABEL_OS_ANY];
  }
  if (normalized[normalized.length - 1] === SYSTEM_LABEL_OS_ANY) {
    return [SYSTEM_LABEL_OS_ANY];
  }
  const concreteLabels = normalized.filter((label) => label !== SYSTEM_LABEL_OS_ANY);
  return normalizeSystemLabels(concreteLabels);
}

export function systemLabelsForOSFamily(osFamily?: string): SystemLabel[] {
  switch ((osFamily ?? '').trim().toLowerCase()) {
    case 'linux':
      return [SYSTEM_LABEL_OS_LINUX];
    case 'windows':
      return [SYSTEM_LABEL_OS_WINDOWS];
    default:
      return [SYSTEM_LABEL_OS_ANY];
  }
}

export function systemLabelText(label: string, t: TFunction): string {
  const option = SYSTEM_LABEL_OPTIONS.find((item) => item.value === label);
  if (!option) {
    return label;
  }
  return t(option.labelKey, { defaultValue: fallbackSystemLabelText(option.value) });
}

export function systemLabelColor(label: string): string {
  return SYSTEM_LABEL_OPTIONS.find((item) => item.value === label)?.color ?? 'default';
}

export function templateInstanceSizeCompatible(
  templateLabels?: readonly string[],
  sizeLabels?: readonly string[],
): boolean {
  const template = normalizeSystemLabels(templateLabels);
  const size = normalizeSystemLabels(sizeLabels);
  if (template.includes(SYSTEM_LABEL_OS_ANY) || size.includes(SYSTEM_LABEL_OS_ANY)) {
    return true;
  }
  return template.some((label) => size.includes(label));
}

export function filterCompatibleInstanceSizes(
  sizes: InstanceSize[],
  template?: Pick<Template, 'system_labels'>,
): InstanceSize[] {
  if (!template) {
    return sizes;
  }
  return sizes.filter((size) => templateInstanceSizeCompatible(template.system_labels, size.system_labels));
}

function fallbackSystemLabelText(label: SystemLabel): string {
  switch (label) {
    case SYSTEM_LABEL_OS_LINUX:
      return 'Linux';
    case SYSTEM_LABEL_OS_WINDOWS:
      return 'Windows';
    default:
      return 'Generic OS';
  }
}

function normalizeSelectionLabels(labels?: readonly string[]): SystemLabel[] {
  const out: SystemLabel[] = [];
  for (const raw of labels ?? []) {
    const label = raw.trim().toLowerCase();
    if (!SYSTEM_LABEL_VALUES.has(label)) {
      continue;
    }
    const typed = label as SystemLabel;
    const existingIndex = out.indexOf(typed);
    if (existingIndex >= 0) {
      out.splice(existingIndex, 1);
    }
    out.push(typed);
  }
  return out;
}
