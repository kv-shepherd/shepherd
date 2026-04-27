"use client";

import type { ReactNode } from "react";
import { Button, Flex, Input, Typography } from "antd";

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
    <div className="page-search-toolbar">
      <Flex vertical gap={10} style={{ width: "100%" }}>
      <Flex
        className="page-search-toolbar__main"
        align="flex-start"
        justify="space-between"
        gap={12}
        wrap
        style={{ width: "100%" }}
      >
        <Flex
          className="page-search-toolbar__search"
          vertical
          gap={8}
          style={{ flex: "1 1 420px", minWidth: 280, maxWidth: 760 }}
        >
          <Input.Search
            className="page-search-toolbar__search-input"
            key={isDraftControlled ? undefined : searchValue}
            enterButton
            allowClear
            size="large"
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
            <Typography.Text
              type="secondary"
              className="page-search-toolbar__help"
            >
              {searchHelp}
            </Typography.Text>
          ) : null}
        </Flex>
        <Flex
          className="page-search-toolbar__controls"
          align="center"
          gap={8}
          wrap
          justify="flex-end"
        >
          {secondaryActions}
          {advancedSearch ? (
            <Button
              className="app-shell-action-button"
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
              className="app-shell-action-button"
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
        <div
          className="page-search-toolbar__advanced"
        >
          <Flex vertical gap={12}>
            <Typography.Text strong className="page-search-toolbar__advanced-title">
              {advancedSearch.title ?? advancedSearch.openLabel}
            </Typography.Text>
            {advancedSearch.content}
          </Flex>
        </div>
      ) : null}
      </Flex>
    </div>
  );
}
