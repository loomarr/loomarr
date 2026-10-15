import type { ComponentPropsWithoutRef, ElementType } from "react";

interface CaptionProps extends ComponentPropsWithoutRef<"span"> {
  // SHOUT renders uppercase with tracking — the section-label voice ("POD · 1:10", "FILLING
  // IN"). Default is plain mono metadata that sits beside content without announcing itself.
  // `danger` is the onair-300 red for a value a person must act on. The colour is inline style, so
  // a `text-*` class can't recolour a caption: a tone is the only way to change it (#1956).
  tone?: "muted" | "strong" | "danger";
  shout?: boolean;
  // The element to render. A caption is often a <div> in a stack or a <dt> in a list; forcing
  // <span> would push callers back to hand-rolled markup, which is what this replaces.
  as?: ElementType;
}

export type { CaptionProps };
