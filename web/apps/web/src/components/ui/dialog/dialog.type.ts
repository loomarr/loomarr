import type { Dialog as DialogPrimitive } from "@base-ui/react/dialog";
import type * as React from "react";

// DialogContent collapses three Base UI parts (Portal → Backdrop + Popup) into the one component
// the app writes. Radix's `Content` did the same job, so call sites are unchanged.
type DialogContentProps = DialogPrimitive.Popup.Props & {
  /** Optional portal target for contained renderers such as the deterministic story gallery. */
  portalContainer?: HTMLElement | ShadowRoot | React.RefObject<HTMLElement | ShadowRoot | null> | null;
};

// Header/Footer are plain layout rows — no primitive behind them.
type DialogSectionProps = React.HTMLAttributes<HTMLDivElement>;

type DialogTitleProps = DialogPrimitive.Title.Props;
type DialogDescriptionProps = DialogPrimitive.Description.Props;

export type { DialogContentProps, DialogDescriptionProps, DialogSectionProps, DialogTitleProps };
