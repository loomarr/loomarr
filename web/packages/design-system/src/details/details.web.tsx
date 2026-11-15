/// <reference lib="dom" />
import { useTheme } from "@tamagui/core";
import { useState } from "react";

import { Text } from "../text";
import type { DetailsProps } from "./details.type";

// The web adapter: a native `<details>` with a `<summary>`, so the keyboard (Enter and Space on the
// summary), the `open` state, the toggle event, find-in-page and print all stay the browser's own.
// It draws the bare look the app has always had, a native marker and a summary line, and adds only
// the one amber focus outline for a keyboard reader.

const Details = ({
  children,
  defaultOpen = false,
  fitSummary = false,
  marker = true,
  onToggle,
  open,
  summary,
  textRole = "compact",
  tone,
}: DetailsProps) => {
  const theme = useTheme();
  const [state, setState] = useState(open ?? defaultOpen);
  const [given, setGiven] = useState(open);
  const [ringed, setRinged] = useState(false);

  // A page that changes `open` moves the Details with it; the reader's own toggles stand until then.
  if (open !== given) {
    setGiven(open);
    if (open !== undefined) setState(open);
  }

  return (
    <details
      onToggle={(event) => {
        const next = event.currentTarget.open;
        // The event also fires after the page's own change; only a difference is news.
        if (next === state) return;
        setState(next);
        onToggle?.(next);
      }}
      open={state}
    >
      <Text
        as="summary"
        cursor="pointer"
        onBlur={() => setRinged(false)}
        onFocus={(event: { currentTarget: HTMLElement }) =>
          setRinged(event.currentTarget.matches(":focus-visible"))
        }
        style={{
          ...(fitSummary ? { width: "fit-content" } : {}),
          ...(marker ? {} : { listStyle: "none" }),
          borderRadius: 2,
          outline: ringed ? `2px solid ${theme.actionFocus.val}` : "none",
          outlineOffset: 2,
        }}
        textRole={textRole}
        tone={tone}
      >
        {typeof summary === "function" ? summary(state) : summary}
      </Text>
      {children}
    </details>
  );
};

export { Details };
