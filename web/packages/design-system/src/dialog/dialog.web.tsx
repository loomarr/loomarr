/// <reference lib="dom" />
import { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import { type CSSProperties, createContext, useContext, useState } from "react";

import { icons } from "../icons";
import { useWindowWidth } from "../layout";
import { iconography, semanticColors, semanticRadius } from "../tokens";
import type {
  DialogActionProps,
  DialogContentProps,
  DialogFooterProps,
  DialogProps,
  DialogSectionProps,
} from "./dialog.type";

// The web adapter, on Base UI Dialog: focus moves in on open, Tab is trapped, Escape and an outside
// press close it, the popup is named and described by its Title and Description, the page behind
// stops scrolling, and focus returns to the trigger or to whatever had it before. The look is the
// app's dialog as the Tailwind version drew it, value for value: a raised card with a decorative
// hairline, a 60% black scrim, and a muted corner close that brightens on hover and takes the amber
// ring from the keyboard. Type sizes are the Web type scale's (`--text-lg` 20, `--text-sm` 13) at
// Tailwind's own line heights for those steps.

const WIDE = 640;
const maxWidths: Record<NonNullable<DialogContentProps["size"]>, number> = {
  default: 448,
  form: 672,
  reading: 768,
};
const shadow = "0 10px 15px -3px rgb(0 0 0 / 0.1), 0 4px 6px -4px rgb(0 0 0 / 0.1)";
const rule = `1px solid ${semanticColors.surface.elevated}`;

const LayoutContext = createContext<NonNullable<DialogContentProps["layout"]>>("padded");

// Whether the corner close is drawn: a dialog that must be answered has none.
const DismissibleContext = createContext(true);

const Dialog = ({ children, defaultOpen, dismissible = true, onOpenChange, open }: DialogProps) => (
  <DialogPrimitive.Root
    defaultOpen={defaultOpen}
    onOpenChange={(next, details) => {
      // Escape and a press outside are asks to close; a dialog that must be answered refuses them. Its
      // own DialogClose, which is a close press, still closes it.
      if (!(next || dismissible) && (details.reason === "escape-key" || details.reason === "outside-press")) {
        details.cancel();
        return;
      }
      onOpenChange?.(next);
    }}
    open={open}
  >
    <DismissibleContext.Provider value={dismissible}>{children}</DismissibleContext.Provider>
  </DialogPrimitive.Root>
);

const DialogTrigger = ({ children }: DialogActionProps) => <DialogPrimitive.Trigger render={children} />;

const DialogClose = ({ children }: DialogActionProps) => <DialogPrimitive.Close render={children} />;

const CornerClose = () => {
  const [hovered, setHovered] = useState(false);
  const [ringed, setRinged] = useState(false);
  return (
    <DialogPrimitive.Close
      aria-label="Close"
      onBlur={() => setRinged(false)}
      onFocus={(event) => setRinged(event.currentTarget.matches(":focus-visible"))}
      onPointerEnter={() => setHovered(true)}
      onPointerLeave={() => setHovered(false)}
      style={{
        position: "absolute",
        top: 16,
        right: 16,
        padding: 0,
        border: 0,
        borderRadius: 4,
        background: "transparent",
        color: hovered ? semanticColors.content.primary : semanticColors.content.secondary,
        outline: "none",
        transition: "color 150ms cubic-bezier(0.4, 0, 0.2, 1)",
        ...(ringed ? { boxShadow: `0 0 0 2px ${semanticColors.action.focus}` } : {}),
      }}
    >
      <icons.close
        aria-hidden
        color="currentColor"
        size={iconography.size.compact}
        strokeWidth={iconography.strokeWidth}
      />
    </DialogPrimitive.Close>
  );
};

// The side panel the Tailwind sheet drew: docked right at full height, 576 wide at most, with a
// hairline on its open edge and the body scrolling under the header band.
const sidePanel: CSSProperties = {
  position: "fixed",
  top: 0,
  right: 0,
  bottom: 0,
  zIndex: 50,
  boxSizing: "border-box",
  width: "100%",
  maxWidth: 576,
  overflowY: "auto",
  borderLeft: `1px solid ${semanticColors.border.decorative}`,
  background: semanticColors.surface.raised,
  color: semanticColors.content.primary,
  boxShadow: shadow,
  outline: "none",
};

// A command surface held near the top of the screen, so the results grow downward from a field
// that stays put. The content brings its own surface; this only places it.
const commandSurface: CSSProperties = {
  position: "fixed",
  top: 96,
  left: "50%",
  zIndex: 50,
  boxSizing: "border-box",
  width: "calc(100% - 2rem)",
  maxWidth: 576,
  transform: "translateX(-50%)",
  outline: "none",
};

const fullScreen: CSSProperties = { position: "fixed", inset: 0, zIndex: 50, outline: "none" };

// A sheet docked to the bottom edge, the phone's "More": the content brings its own surface.
const bottomDock: CSSProperties = {
  position: "fixed",
  right: 0,
  bottom: 0,
  left: 0,
  zIndex: 50,
  outline: "none",
};

const DialogContent = ({
  accessibilityLabel,
  children,
  finalFocus,
  layout,
  placement = "center",
  portalContainer,
  size = "default",
}: DialogContentProps) => {
  const wide = useWindowWidth() >= WIDE;
  const side = placement === "side";
  const top = placement === "top";
  const fill = placement === "fill";
  const bottom = placement === "bottom";
  const dismissible = useContext(DismissibleContext);
  const resolvedLayout = layout ?? (side ? "sectioned" : "padded");
  const popup: CSSProperties = side
    ? sidePanel
    : fill
      ? fullScreen
      : bottom
        ? bottomDock
        : top
          ? commandSurface
          : {
              position: "fixed",
              top: "50%",
              left: "50%",
              zIndex: 50,
              display: "flex",
              flexDirection: "column",
              gap: 16,
              boxSizing: "border-box",
              width: wide ? "calc(100% - 2rem)" : "100%",
              maxWidth: maxWidths[size],
              maxHeight: "calc(100dvh - 2rem)",
              overflow: resolvedLayout === "sectioned" ? "hidden" : "auto",
              padding: resolvedLayout === "sectioned" ? 0 : 24,
              transform: "translate(-50%, -50%)",
              border: `1px solid ${semanticColors.border.decorative}`,
              borderRadius: semanticRadius.cardCompact,
              background: semanticColors.surface.raised,
              color: semanticColors.content.primary,
              boxShadow: shadow,
              outline: "none",
            };
  return (
    <DialogPrimitive.Portal container={portalContainer as HTMLElement | ShadowRoot | null | undefined}>
      <DialogPrimitive.Backdrop
        style={{ position: "fixed", inset: 0, zIndex: 50, background: "rgb(0 0 0 / 0.6)" }}
      />
      <DialogPrimitive.Popup
        aria-label={accessibilityLabel}
        finalFocus={finalFocus as { readonly current: HTMLElement | null } | undefined}
        style={popup}
      >
        <LayoutContext.Provider value={resolvedLayout}>{children}</LayoutContext.Provider>
        {top || fill || bottom || !dismissible ? null : <CornerClose />}
      </DialogPrimitive.Popup>
    </DialogPrimitive.Portal>
  );
};

const DialogHeader = ({ children }: DialogSectionProps) => {
  const sectioned = useContext(LayoutContext) === "sectioned";
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 6,
        ...(sectioned ? { padding: "24px 48px 16px 24px", borderBottom: rule } : {}),
      }}
    >
      {children}
    </div>
  );
};

const DialogFooter = ({ children, spread = false, stacked = false }: DialogFooterProps) => {
  const sectioned = useContext(LayoutContext) === "sectioned";
  return (
    <div
      style={{
        display: "flex",
        gap: 8,
        ...(stacked
          ? { flexDirection: "column", alignItems: "stretch" }
          : {
              justifyContent: spread ? "space-between" : "flex-end",
              ...(spread ? { flexWrap: "wrap" } : {}),
            }),
        ...(sectioned ? { padding: "16px 24px", borderTop: rule } : {}),
      }}
    >
      {children}
    </div>
  );
};

// A padded title keeps clear of the corner close; a sectioned header already does.
const DialogTitle = ({ children }: DialogSectionProps) => {
  const dismissible = useContext(DismissibleContext);
  const padded = useContext(LayoutContext) === "padded" && dismissible;
  return (
    <DialogPrimitive.Title
      style={{
        margin: 0,
        fontSize: 20,
        fontWeight: 600,
        lineHeight: "calc(1.75 / 1.125)",
        ...(padded ? { paddingRight: 32 } : {}),
      }}
    >
      {children}
    </DialogPrimitive.Title>
  );
};

const DialogDescription = ({ children }: DialogSectionProps) => (
  <DialogPrimitive.Description
    style={{
      margin: 0,
      fontSize: 13,
      lineHeight: "calc(1.25 / 0.875)",
      color: semanticColors.content.secondary,
    }}
  >
    {children}
  </DialogPrimitive.Description>
);

export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
};
