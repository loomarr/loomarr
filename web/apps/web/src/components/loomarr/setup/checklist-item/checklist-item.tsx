import { parseDocHref } from "@loomarr/core/anchor";
import { ActivityIndicator, Icon, icons, Stack, Text, TextLink } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { ChecklistItemProps, CheckStatus } from "./checklist-item.type";

// ChecklistItem — the wizard / Settings-troubleshooting check row (§3, §6). A pass
// "locks in" green (§2.4); a fail never blames — plain cause + the exact fix + a
// deep link into the embedded docs (§13). Backed by GET /v1/setup/status.
const ICON: Record<CheckStatus, { node: ReactNode; label: string }> = {
  pending: { node: <Icon decorative glyph={icons.pending} size="compact" tone="muted" />, label: "Pending" },
  running: { node: <ActivityIndicator decorative size="compact" tone="info" />, label: "Checking" },
  pass: { node: <Icon decorative glyph={icons.passed} size="compact" tone="success" />, label: "Passed" },
  fail: { node: <Icon decorative glyph={icons.failed} size="compact" tone="danger" />, label: "Failed" },
};

const ChecklistItem = ({ name, status, hint, docHref, className }: ChecklistItemProps) => (
  <Stack
    align="start"
    className={cn(
      "border-border bg-card rounded-md border px-3 py-2.5 transition-colors",
      status === "pass" && "border-lock-tint-30",
      status === "fail" && "border-onair-tint-30",
      className,
    )}
    direction="row"
    gap="control"
  >
    <span className="mt-0.5" role="img" aria-label={ICON[status].label}>
      {ICON[status].node}
    </span>
    <div className="min-w-0 flex-1">
      <Text as="p" textRole="cardLabel">
        {name}
      </Text>
      {status === "fail" && hint && (
        <Text as="p" className="mt-0.5" textRole="compact" tone="secondary">
          {hint}
        </Text>
      )}
    </div>
    {status === "fail" && docHref && (
      // Route into the Help center — the docHref ("troubleshooting#tunarr") is parsed
      // into { page, section }, not used as a raw href (which resolves relative to the
      // current path and 404s). This is what makes §13's "every red check deep-links to
      // its section" actually navigate.
      <Text as="span" className="shrink-0" textRole="hint">
        <TextLink icon="goTo" render={<Link search={parseDocHref(docHref)} to="/help" />}>
          Fix
        </TextLink>
      </Text>
    )}
  </Stack>
);

export { ChecklistItem };
