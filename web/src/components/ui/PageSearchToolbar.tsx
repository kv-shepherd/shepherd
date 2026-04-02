"use client";

import type { ReactNode } from "react";
import { Button, Card, Flex, Input, Typography } from "antd";

export const filterOptionByLabel = (
  input: string,
  option?: { label?: unknown },
) => {
  const label = typeof option?.label === "string" ? option.label : "";
  return label.toLowerCase().includes(input.trim().toLowerCase());
};

interface AdvancedSearchConfig {
  open: boolean;
  onToggle: () => void;
  openLabel: string;
  closeLabel: string;
  content: ReactNode;
  title?: ReactNode;
  toggleTestId?: string;
}

interface PageSearchToolbarProps {
  searchValue: string;
  searchDraftValue?: string;
  onSearchDraftChange?: (value: string) => void;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  searchTestId?: string;
  searchHelp?: ReactNode;
  primaryActions?: ReactNode;
  secondaryActions?: ReactNode;
  advancedSearch?: AdvancedSearchConfig;
  hasActiveFilters?: boolean;
  onClear?: () => void;
  clearLabel?: string;
  clearTestId?: string;
}

export function PageSearchToolbar({
  searchValue,
  searchDraftValue,
  onSearchDraftChange,
  onSearchChange,
  searchPlaceholder,
  searchTestId,
  searchHelp,
  primaryActions,
  secondaryActions,
  advancedSearch,
  hasActiveFilters = false,
  onClear,
  clearLabel,
  clearTestId,
}: PageSearchToolbarProps) {
  const isDraftControlled =
    searchDraftValue !== undefined && onSearchDraftChange !== undefined;

  return (
    <Flex vertical gap={10} style={{ width: "100%" }}>
      <Flex
        align="flex-start"
        justify="space-between"
        gap={12}
        wrap
        style={{ width: "100%" }}
      >
        <Flex
          vertical
          gap={8}
          style={{ flex: "1 1 420px", minWidth: 280, maxWidth: 760 }}
        >
          <Input.Search
            key={isDraftControlled ? undefined : searchValue}
            enterButton
            allowClear
            {...(isDraftControlled
              ? {
                  value: searchDraftValue,
                }
              : {
                  defaultValue: searchValue,
                })}
            placeholder={searchPlaceholder}
            onChange={(event) => {
              if (isDraftControlled) {
                onSearchDraftChange(event.target.value);
                return;
              }
            }}
            onSearch={(value) => onSearchChange(value)}
            data-testid={searchTestId}
          />
          {searchHelp ? (
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              {searchHelp}
            </Typography.Text>
          ) : null}
        </Flex>
        <Flex align="center" gap={8} wrap justify="flex-end">
          {secondaryActions}
          {advancedSearch ? (
            <Button
              onClick={advancedSearch.onToggle}
              data-testid={advancedSearch.toggleTestId}
            >
              {advancedSearch.open
                ? advancedSearch.closeLabel
                : advancedSearch.openLabel}
            </Button>
          ) : null}
          {hasActiveFilters && onClear ? (
            <Button
              onClick={() => {
                if (isDraftControlled) {
                  onSearchDraftChange("");
                }
                onClear();
              }}
              data-testid={clearTestId}
            >
              {clearLabel}
            </Button>
          ) : null}
          {primaryActions}
        </Flex>
      </Flex>
      {advancedSearch?.open ? (
        <Card
          size="small"
          style={{
            background: "#fafafa",
            borderColor: "#f0f0f0",
          }}
        >
          <Flex vertical gap={12}>
            <Typography.Text strong>
              {advancedSearch.title ?? advancedSearch.openLabel}
            </Typography.Text>
            {advancedSearch.content}
          </Flex>
        </Card>
      ) : null}
    </Flex>
  );
}
