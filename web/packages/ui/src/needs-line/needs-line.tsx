import { StatusDot, Surface, Text } from "@loomarr/design-system";
import { cloneElement, useState } from "react";

import type { NeedsLineItem, NeedsLineProps } from "./needs-line.type";

const Item = ({ compact, item }: { compact: boolean; item: NeedsLineItem }) => {
  const [hovered, setHovered] = useState(false);
  const own = item.link.props as {
    onMouseEnter?: (event: unknown) => void;
    onMouseLeave?: (event: unknown) => void;
    style?: object;
  };
  return cloneElement(
    item.link,
    {
      ...own,
      onMouseEnter: (event: unknown) => {
        own.onMouseEnter?.(event);
        setHovered(true);
      },
      onMouseLeave: (event: unknown) => {
        own.onMouseLeave?.(event);
        setHovered(false);
      },
      style: {
        alignItems: "center",
        backgroundColor: hovered ? "var(--surfaceElevated)" : "transparent",
        borderRadius: 8,
        color: "var(--contentPrimary)",
        display: "flex",
        gap: 6,
        padding: "2px 6px",
        textDecoration: "none",
        transition: "background-color 120ms",
        ...own.style,
      },
    },
    <>
      {item.warn && <StatusDot label="Running short" size="compact" tone="warn" />}
      <Text as="span" strong style={{ fontVariantNumeric: "tabular-nums" }} textRole="body">
        {item.value}
      </Text>
      {/* Read as "3 requests to approve": the flex gap draws the space, this says it. */}{" "}
      <Text as="span" textRole="body">
        {compact ? item.short : item.label}
      </Text>
    </>,
  );
};

// NeedsLine — the admin's "Needs you" counts on Home as one quiet line above the channel wall
// (#2785, Direction mock layout B). Each item is a number and the page that deals with it. Filler
// space running short is a warning rather than a to-do, so it carries a dot. The caller passes only
// the items above zero, and the line is gone when there are none.
const NeedsLine = ({ compact = false, items }: NeedsLineProps) => {
  if (items.length === 0) return null;
  return (
    <Surface
      alignItems="center"
      aria-label="Needs you"
      backgroundColor="transparent"
      borderRadius="cardCompact"
      columnGap={4}
      flexDirection="row"
      flexWrap={compact ? "wrap" : "nowrap"}
      paddingHorizontal={12}
      paddingVertical={8}
      role="navigation"
      rowGap={4}
    >
      <Text as="span" marginRight={8} style={{ fontWeight: 500 }} textRole="body" tone="secondary">
        Needs you
      </Text>
      {items.map((item, index) => (
        <Surface
          alignItems="center"
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          gap={4}
          key={item.id}
        >
          {index > 0 && (
            <Text aria-hidden as="span" textRole="body">
              ·
            </Text>
          )}
          <Item compact={compact} item={item} />
        </Surface>
      ))}
    </Surface>
  );
};

export { NeedsLine };
