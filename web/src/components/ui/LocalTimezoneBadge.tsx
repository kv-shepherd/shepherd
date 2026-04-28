"use client";

import React from "react";

import { Select } from "antd";
import { useTranslation } from "react-i18next";
import { useDisplayTimeZone } from "@/components/providers/DisplayTimeZoneProvider";
import {
  formatTimeZoneOptionLabel,
  formatTimeZoneShortLabel,
  getTimeZoneLocationFallback,
  getSupportedTimeZones,
} from "@/lib/timeZone";

const FOLLOW_BROWSER_VALUE = "__browser__";
const TIME_ZONE_OPTIONS = getSupportedTimeZones();

function timeZoneCityKey(timeZone: string): string {
  return `status.timezone.city.${timeZone.replaceAll("/", "_")}`;
}

export default function LocalTimezoneBadge() {
  const { t } = useTranslation("common");
  const { browserTimeZone, preferenceTimeZone, isSaving, setTimeZone } =
    useDisplayTimeZone();
  const formatLocalizedTimeZoneShortLabel = React.useCallback((timeZone: string) => (
    formatTimeZoneShortLabel(
      timeZone,
      t(timeZoneCityKey(timeZone), {
        defaultValue: getTimeZoneLocationFallback(timeZone),
      }),
    )
  ), [t]);
  const formatLocalizedTimeZoneOptionLabel = React.useCallback((timeZone: string) => (
    formatTimeZoneOptionLabel(
      timeZone,
      t(timeZoneCityKey(timeZone), {
        defaultValue: getTimeZoneLocationFallback(timeZone),
      }),
    )
  ), [t]);
  const options = React.useMemo(
    () => [
      {
        value: FOLLOW_BROWSER_VALUE,
        label: t("status.local_timezone_follow_browser", {
          timeZone: browserTimeZone
            ? formatLocalizedTimeZoneOptionLabel(browserTimeZone)
            : t("status.local_timezone_auto"),
        }),
        shortLabel: browserTimeZone
          ? formatLocalizedTimeZoneShortLabel(browserTimeZone)
          : t("status.local_timezone_auto"),
      },
      ...TIME_ZONE_OPTIONS.map((timeZone) => ({
        value: timeZone,
        label: formatLocalizedTimeZoneOptionLabel(timeZone),
        shortLabel: formatLocalizedTimeZoneShortLabel(timeZone),
      })),
    ],
    [
      browserTimeZone,
      formatLocalizedTimeZoneOptionLabel,
      formatLocalizedTimeZoneShortLabel,
      t,
    ],
  );

  return (
    <Select
      aria-label={t("status.display_timezone")}
      className="app-shell-timezone-select"
      loading={isSaving}
      optionFilterProp="label"
      optionLabelProp="shortLabel"
      options={options}
      popupMatchSelectWidth={340}
      showSearch
      title={t("status.display_timezone")}
      value={preferenceTimeZone ?? FOLLOW_BROWSER_VALUE}
      onChange={(value) => {
        void setTimeZone(
          value === FOLLOW_BROWSER_VALUE ? null : String(value),
        );
      }}
      filterOption={(input, option) =>
        String(option?.label ?? "")
          .toLowerCase()
          .includes(input.toLowerCase())
      }
    />
  );
}
