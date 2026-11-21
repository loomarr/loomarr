/// <reference lib="dom" />
import { type CSSProperties, type KeyboardEvent, useId, useState } from "react";

import { icons } from "../icons";
import { iconography, playerChrome, semanticThemes } from "../tokens";
import type { DockedPanelProps } from "./docked-panel.type";

// The web adapter: a plain section docked inside its container, which must be `position: relative`
// (the player's frame). It is deliberately not a modal. It has no scrim, no portal, no focus trap
// and no scroll lock, so the picture beside it keeps playing and the page keeps its keys. It owns
// Escape, and it is `role="dialog"` so the player's up and down arrows leave its list alone.
//
// It wears the player's chrome, dark in both themes, because it always sits over a picture. Its
// values are the Watch page's channel drawer, value for value: a 95% canvas, a decorative hairline
// on the inner edge, an elevated hairline under the header, a 13px/600 title.

const DEFAULT_WIDTH = 372;
const Z_INDEX = 40;
const dark = semanticThemes.dark;

const CloseButton = ({ label, onClick }: { label: string; onClick: () => void }) => {
  const [hovered, setHovered] = useState(false);
  const [ringed, setRinged] = useState(false);
  return (
    <button
      type="button"
      aria-label={label}
      onBlur={() => setRinged(false)}
      onClick={onClick}
      onFocus={(event) => setRinged(event.currentTarget.matches(":focus-visible"))}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        width: 24,
        height: 24,
        marginLeft: "auto",
        padding: 0,
        border: 0,
        borderRadius: 4,
        background: "transparent",
        color: hovered ? playerChrome.text : playerChrome.textMuted,
        outline: "none",
        ...(ringed ? { boxShadow: `0 0 0 2px ${dark.action.focus}` } : {}),
      }}
    >
      <icons.close
        aria-hidden
        color="currentColor"
        size={iconography.size.compact}
        strokeWidth={iconography.strokeWidth}
      />
    </button>
  );
};

const DockedPanel = ({
  children,
  closeLabel,
  edge = "right",
  meta,
  onClose,
  title,
  width = DEFAULT_WIDTH,
}: DockedPanelProps) => {
  const titleId = useId();
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    // Escape closes this panel and nothing behind it (the player's own Escape handling).
    event.stopPropagation();
    onClose();
  };
  const section: CSSProperties = {
    position: "absolute",
    top: 0,
    bottom: 0,
    [edge]: 0,
    zIndex: Z_INDEX,
    boxSizing: "border-box",
    display: "flex",
    flexDirection: "column",
    width,
    maxWidth: "100%",
    [edge === "right" ? "borderLeft" : "borderRight"]: `1px solid ${dark.border.decorative}`,
    background: playerChrome.panel,
    color: playerChrome.text,
  };
  return (
    <section aria-labelledby={titleId} role="dialog" onKeyDown={onKeyDown} style={section}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 10,
          padding: "16px 18px 12px",
          borderBottom: `1px solid ${dark.surface.elevated}`,
        }}
      >
        <h2 id={titleId} style={{ margin: 0, fontSize: 13, fontWeight: 600, lineHeight: "20px" }}>
          {title}
        </h2>
        {meta === undefined ? null : <span style={{ color: playerChrome.textMuted }}>{meta}</span>}
        <CloseButton label={closeLabel} onClick={onClose} />
      </div>
      <div style={{ display: "flex", flex: 1, flexDirection: "column", minHeight: 0 }}>{children}</div>
    </section>
  );
};

export { DockedPanel };
