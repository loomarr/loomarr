import type { ComponentPropsWithoutRef, ElementType } from "react";

interface CaptionProps extends ComponentPropsWithoutRef<"span"> {
  // SHOUT renders uppercase with tracking — the section-label voice ("POD · 1:10", "FILLING
  // IN"). Default is plain mono metadata that sits beside content without announcing itself.
  tone?: "muted" | "strong";
  shout?: boolean;
  // The element to render. A caption is often a <div> in a stack or a <dt> in a list; forcing
  // <span> would push callers back to hand-rolled markup, which is what this replaces.
  as?: ElementType;
}

export type { CaptionProps };
