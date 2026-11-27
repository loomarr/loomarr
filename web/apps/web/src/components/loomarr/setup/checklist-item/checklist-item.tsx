import { parseDocHref } from "@loomarr/core/anchor";
import { ActivityIndicator, Card, Icon, icons, Stack, Text, TextLink } from "@loomarr/design-system";
import { Link } from "@tanstack/react-router";
import type { ReactNode } from "react";
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
  <Card
    alignItems="flex-start"
    borderColor={status === "pass" ? "stateSuccess" : undefined}
    className={className}
    flexDirection="row"
    gap={12}
    paddingHorizontal={12}
    paddingVertical={10}
    tone={status === "fail" ? "caution" : undefined}
  >
    <Stack aria-label={ICON[status].label} direction="row" marginTop={2} render="span" role="img">
      {ICON[status].node}
    </Stack>
    <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>
      <Text as="p" textRole="cardLabel">
        {name}
      </Text>
      {status === "fail" && hint && (
        <Stack marginTop={2}>
          <Text as="p" textRole="compact" tone="secondary">
            {hint}
          </Text>
        </Stack>
      )}
    </Stack>
    {status === "fail" && docHref && (
      // Route into the Help center — the docHref ("troubleshooting#tunarr") is parsed
      // into { page, section }, not used as a raw href (which resolves relative to the
      // current path and 404s). This is what makes §13's "every red check deep-links to
      // its section" actually navigate.
      <Stack flexShrink={0} render="span">
        <Text as="span" textRole="hint">
          <TextLink icon="goTo" render={<Link search={parseDocHref(docHref)} to="/help" />}>
            Fix
          </TextLink>
        </Text>
      </Stack>
    )}
  </Card>
);

export { ChecklistItem };
