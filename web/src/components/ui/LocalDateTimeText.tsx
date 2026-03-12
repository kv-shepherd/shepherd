"use client";

interface LocalDateTimeTextProps {
  value?: string | null;
  emptyFallback?: string;
}

export function LocalDateTimeText({
  value,
  emptyFallback = "—",
}: LocalDateTimeTextProps) {
  const formatted = formatLocalDateTime(value);

  return (
    <span suppressHydrationWarning title={value ?? undefined}>
      {formatted ?? emptyFallback}
    </span>
  );
}

function formatLocalDateTime(value?: string | null): string | null {
  if (!value || value.trim() === "") {
    return null;
  }

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }

  const formatter = new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hourCycle: "h23",
  });
  const parts = formatter.formatToParts(parsed);
  const pick = (type: Intl.DateTimeFormatPartTypes) =>
    parts.find((part) => part.type === type)?.value ?? "";
  return `${pick("year")}-${pick("month")}-${pick("day")} ${pick("hour")}:${pick("minute")}`;
}
