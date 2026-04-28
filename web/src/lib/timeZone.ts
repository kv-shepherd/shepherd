const FALLBACK_TIME_ZONES = [
  "UTC",
  "Asia/Hong_Kong",
  "Asia/Shanghai",
  "Asia/Singapore",
  "Asia/Tokyo",
  "Europe/London",
  "Europe/Berlin",
  "America/New_York",
  "America/Los_Angeles",
];

const PRIORITY_TIME_ZONES = FALLBACK_TIME_ZONES;

type IntlWithSupportedValues = typeof Intl & {
  supportedValuesOf?: (key: "timeZone") => string[];
};

export function getBrowserTimeZone(): string | null {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone ?? null;
  } catch {
    return null;
  }
}

export function getSupportedTimeZones(): string[] {
  const supported = (Intl as IntlWithSupportedValues).supportedValuesOf?.("timeZone");
  if (Array.isArray(supported) && supported.length > 0) {
    return Array.from(new Set([...PRIORITY_TIME_ZONES, ...supported]));
  }
  return FALLBACK_TIME_ZONES;
}


function formatTimeZoneOffsetLabel(
  timeZone: string,
  date: Date = new Date(),
): string | null {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone,
      timeZoneName: "shortOffset",
    }).formatToParts(date);
    const rawOffset = parts.find((part) => part.type === "timeZoneName")?.value;

    if (!rawOffset) {
      return null;
    }
    if (rawOffset === "GMT") {
      return "UTC";
    }

    return rawOffset.replace(/^GMT/, "UTC");
  } catch {
    return null;
  }
}

export function getTimeZoneLocationFallback(timeZone: string): string {
  if (timeZone === "UTC") {
    return "UTC";
  }
  if (timeZone === "Asia/Shanghai") {
    return "Beijing";
  }
  return timeZone.split("/").at(-1)?.replaceAll("_", " ") ?? timeZone;
}

export function formatTimeZoneShortLabel(
  timeZone: string,
  locationLabel = getTimeZoneLocationFallback(timeZone),
): string {
  const offsetLabel = formatTimeZoneOffsetLabel(timeZone);
  return offsetLabel ? `${offsetLabel} · ${locationLabel}` : locationLabel;
}

export function formatTimeZoneOptionLabel(
  timeZone: string,
  locationLabel = getTimeZoneLocationFallback(timeZone),
): string {
  const offsetLabel = formatTimeZoneOffsetLabel(timeZone);
  return offsetLabel
    ? `${locationLabel} · ${timeZone} (${offsetLabel})`
    : `${locationLabel} · ${timeZone}`;
}
