import { Text, useWindowWidth } from "@loomarr/design-system";
import { cloneElement, type ReactElement, useState } from "react";

import type { HighlightRowProps } from "./list-row.type";

// The phone breakpoint the legacy row's `sm:` grid used.
const PHONE_MAX = 640;

// HighlightRow — one of Tonight's highlights (#1659 web mock): time, channel number, the show and
// why it is worth watching, on a fixed 80 / 36 / fill / auto grid so the columns line up down the
// list. At phone width the reason drops under the show (#1785): on one line it took the show's
// whole column. The time column stays fixed there too, or each row's time sets its own width.
//
// The whole row is ONE control (the channel's Watch) with nothing interactive inside it. Given a
// router `<Link>` it renders as that link, so middle-click and "open in new tab" work; otherwise it
// is a button. Lives inside a `ListGroup`, which draws the hairlines.
const HighlightRow = ({ channelNumber, detail, onClick, reason, render, time, title }: HighlightRowProps) => {
  const phone = useWindowWidth() < PHONE_MAX;
  const [hovered, setHovered] = useState(false);
  const host: ReactElement<Record<string, unknown>> = render ?? <button type="button" />;
  const own = host.props as { onClick?: (event: unknown) => void; style?: object };

  return cloneElement(
    host,
    {
      ...own,
      onClick: (event: unknown) => {
        own.onClick?.(event);
        onClick?.();
      },
      onMouseEnter: () => setHovered(true),
      onMouseLeave: () => setHovered(false),
      style: {
        alignItems: "center",
        backgroundColor: hovered ? "var(--surfaceElevated)" : "transparent",
        border: "none",
        boxSizing: "border-box",
        color: "var(--contentPrimary)",
        columnGap: 12,
        cursor: "pointer",
        display: "grid",
        font: "inherit",
        gridTemplateColumns: phone ? "80px 36px minmax(0, 1fr)" : "80px 36px minmax(0, 1fr) auto",
        padding: "12px 16px",
        rowGap: phone ? 2 : 0,
        textAlign: "left",
        textDecoration: "none",
        width: "100%",
        ...own.style,
      },
    },
    <>
      <Text as="span" textRole="data">
        {time}
      </Text>
      <Text as="span" strong textRole="data">
        {channelNumber}
      </Text>
      <Text
        as="span"
        strong
        style={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}
        textRole="body"
      >
        {title}
        {detail != null && (
          <Text as="span" textRole="body" tone="secondary">
            {detail}
          </Text>
        )}
      </Text>
      {reason != null && (
        <Text
          as="span"
          style={phone ? { gridColumnStart: 3 } : { whiteSpace: "nowrap" }}
          textRole="caption"
          tone="secondary"
        >
          {reason}
        </Text>
      )}
    </>,
  );
};

export { HighlightRow };
