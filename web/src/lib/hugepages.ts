export const HUGEPAGES_PAGE_SIZE_PATH =
  "spec.template.spec.domain.memory.hugepages.pageSize";
export const HUGEPAGES_PRESET_OPTIONS = ["2Mi", "1Gi"] as const;

export function normalizeHugepagesPageSizeValue(
  value: unknown,
): string | undefined {
  if (typeof value !== "string") return undefined;
  const trimmed = value.trim();
  if (!trimmed) return undefined;

  const compact = trimmed.replace(/\s+/g, "");
  const mbOnly = compact.match(/^([1-9]\d*)$/);
  if (mbOnly) {
    return `${mbOnly[1]}Mi`;
  }
  const mi = compact.match(/^([1-9]\d*)mi$/i);
  if (mi) {
    return `${mi[1]}Mi`;
  }
  const gi = compact.match(/^([1-9]\d*)gi$/i);
  if (gi) {
    return `${gi[1]}Gi`;
  }

  return compact;
}

export function isValidHugepagesPageSizeValue(value: unknown): boolean {
  if (value === undefined || value === null || value === "") return true;
  if (typeof value !== "string") return false;
  if (
    HUGEPAGES_PRESET_OPTIONS.includes(
      value as (typeof HUGEPAGES_PRESET_OPTIONS)[number],
    )
  ) {
    return true;
  }
  return /^[1-9]\d*Mi$/.test(value);
}

export function normalizeHugepagesPageSizeList(values: unknown): string[] {
  if (!Array.isArray(values)) {
    return [];
  }

  const normalized: string[] = [];
  const seen = new Set<string>();
  for (const raw of values) {
    const value = normalizeHugepagesPageSizeValue(raw);
    if (!value || !isValidHugepagesPageSizeValue(value) || seen.has(value)) {
      continue;
    }
    seen.add(value);
    normalized.push(value);
  }

  return normalized;
}
