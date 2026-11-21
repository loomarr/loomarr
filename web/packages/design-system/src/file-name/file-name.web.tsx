/// <reference lib="dom" />
import { useLayoutEffect, useRef, useState } from "react";

import { Text } from "../text";
import type { FileNameProps } from "./file-name.type";
import { splitFileName, visibleFileName } from "./split-file-name";

const box = {
  // Inline in a sentence or a link: one line, clipped to its own box, never breaking the line.
  inline: { display: "inline-block", maxWidth: "100%", verticalAlign: "bottom" },
  // A block that fills what its parent gives and contributes no width of its own, so a table
  // column is sized by the table, not held open by the name.
  fill: { display: "block", minWidth: "100%", width: 0 },
} as const;

// A shortened part: a clipped inline box that ends in an ellipsis.
const clipped = {
  display: "inline-block",
  overflow: "hidden",
  textOverflow: "ellipsis",
  verticalAlign: "top",
  whiteSpace: "pre",
} as const;

/**
 * Whether the name overflows its box. The name is drawn whole as plain inline text until it does
 * not fit, then shortened; it returns to whole when its box grows back to the name's full width.
 * Plain inline text is what find-in-page searches across, so the name stays whole whenever it can.
 */
const useShortened = (name: string) => {
  const ref = useRef<HTMLSpanElement>(null);
  const [shortened, setShortened] = useState(false);
  const fullWidth = useRef(0);
  // biome-ignore lint/correctness/useExhaustiveDependencies: a new name is measured afresh.
  useLayoutEffect(() => {
    const element = ref.current;
    if (!element || typeof ResizeObserver === "undefined") return;
    setShortened(false);
    fullWidth.current = 0;
    const measure = () => {
      setShortened((now) => {
        if (!now) {
          fullWidth.current = element.scrollWidth;
          return element.scrollWidth > element.clientWidth + 0.5;
        }
        return element.clientWidth < fullWidth.current - 0.5;
      });
    };
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    return () => observer.disconnect();
  }, [name]);
  return { ref, shortened };
};

/**
 * The web adapter: a file name in the data face on one line. While it fits it is plain inline text.
 * When it does not, CSS can shorten only a line's end, so the name is drawn in two parts that
 * together are still the name once (copy and assistive tech read it whole): the head gives way first
 * with an ellipsis, leaving the tail's length (in `ch`, the data face being monospaced) for the
 * tail; when even the tail does not fit, it gives way with its own ellipsis. Find-in-page then
 * matches within either part, not across the cut. The row follows the name's own direction, so a
 * right-to-left name keeps its ellipsis beside its tail. The whole name shows on hover. A control
 * character is drawn as its visible picture, so the name stays on one line.
 *
 * It shortens when its parent bounds its width: a block, or a flex or grid item that may shrink
 * (`min-width: 0`). In a table cell use `layout="fill"`: an inline name holds the column open at
 * its full length.
 */
const FileName = ({
  density = "pointer",
  layout = "inline",
  name,
  strong = false,
  tone = "primary",
}: FileNameProps) => {
  const shown = visibleFileName(name);
  const { head, tail, tailLength } = splitFileName(shown);
  const { ref, shortened } = useShortened(shown);
  return (
    <span
      data-shortened={shortened || undefined}
      dir="auto"
      ref={ref}
      style={{ ...box[layout], overflow: "hidden", whiteSpace: "pre" }}
      title={shown}
    >
      <Text
        as="span"
        density={density}
        strong={strong}
        style={{ display: "contents" }}
        textRole="data"
        tone={tone}
      >
        {head ? (
          <span
            data-part="head"
            // At least a pixel wide: a part with no width drops out of a copied selection.
            style={
              shortened ? { ...clipped, maxWidth: `calc(100% - ${tailLength}ch)`, minWidth: 1 } : undefined
            }
          >
            {head}
          </span>
        ) : null}
        <span
          data-part="tail"
          style={shortened ? { ...clipped, maxWidth: head ? "calc(100% - 1px)" : "100%" } : undefined}
        >
          {tail}
        </span>
      </Text>
    </span>
  );
};

export { FileName };
