"use client";

import { useState } from "react";

import { Typography } from "antd";
import { useTranslation } from "react-i18next";

const { Text } = Typography;

export default function LocalTimezoneBadge() {
  const { t } = useTranslation("common");
  const [timeZone] = useState(() =>
    formatUtcOffsetLabel(new Date().getTimezoneOffset()),
  );

  if (!timeZone) {
    return null;
  }

  return (
    <Text
      type="secondary"
      style={{ fontSize: 12, lineHeight: 1, whiteSpace: "nowrap" }}
      suppressHydrationWarning
      title={timeZone}
    >
      {t("status.local_timezone", { timeZone })}
    </Text>
  );
}

function formatUtcOffsetLabel(offsetMinutesWestOfUtc: number): string {
  const totalMinutes = -offsetMinutesWestOfUtc;
  const sign = totalMinutes >= 0 ? "+" : "-";
  const absoluteMinutes = Math.abs(totalMinutes);
  const hours = Math.floor(absoluteMinutes / 60);
  const minutes = absoluteMinutes % 60;

  if (minutes === 0) {
    return `UTC${sign}${hours}`;
  }

  return `UTC${sign}${hours}:${String(minutes).padStart(2, "0")}`;
}
