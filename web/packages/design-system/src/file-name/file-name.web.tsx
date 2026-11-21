/// <reference lib="dom" />
import type { ClipboardEvent } from "react";

import { Text } from "../text";
import type { FileNameProps } from "./file-name.type";
import { nameDirection, splitFileName, visibleFileName } from "./split-file-name";

const box = {
  // Inline in a sentence or a link: one line, clipped to its own box, never breaking the line.
  inline: { display: "inline-flex", maxWidth: "100%", minWidth: 0, verticalAlign: "bottom" },
  // For a table cell only: it takes the width the cell gives and asks for none, so the table sizes
  // the column. In a parent sized to its content it has no width at all.
  fill: { display: "flex", minWidth: "100%", width: 0 },
} as const;

const part = { minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "pre" } as const;

/**
 * The web adapter: a file name in the data face on one line, in CSS alone. The name is two parts in
 * a row: the head gives way first with an ellipsis; the tail (the end of the name and its
 * extension) is as wide as its own text, so a wide glyph never pushes the extension out, and gives
 * way last with its own ellipsis at its start, so the extension stays to the end. The box clips, follows the name's own direction (a right-to-left
 * name keeps its ellipsis beside its tail), and the whole name shows on hover. Copying a selection
 * inside the name copies the name exactly. A control character or line separator is drawn as a
 * visible picture, so the name stays on one line.
 *
 * What it does not do: find-in-page does not match text that spans the two parts, and copying a
 * larger selection that merely includes the name puts a line break at the cut.
 *
 * It shortens when its parent bounds its width: a block, or a flex or grid item that may shrink
 * (`min-width: 0`). In a table cell use `layout="fill"`: an inline name holds the column open.
 */
const FileName = ({
  density = "pointer",
  layout = "inline",
  name,
  strong = false,
  tone = "primary",
}: FileNameProps) => {
  const shown = visibleFileName(name);
  const { head, tail } = splitFileName(shown);
  const direction = nameDirection(shown);
  const copyWhole = (event: ClipboardEvent<HTMLSpanElement>) => {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) return;
    if (!event.currentTarget.contains(selection.getRangeAt(0).commonAncestorContainer)) return;
    event.clipboardData.setData("text/plain", name);
    event.preventDefault();
  };
  return (
    <span dir="auto" onCopy={copyWhole} style={{ ...box[layout], overflow: "hidden" }} title={shown}>
      <Text
        as="span"
        density={density}
        strong={strong}
        style={{ display: "contents" }}
        textRole="data"
        tone={tone}
      >
        {head ? (
          <span data-part="head" style={{ ...part, flexShrink: 1 }}>
            {head}
          </span>
        ) : null}
        {/* The tail's box runs against the name, so its ellipsis falls at the tail's start; the
            text inside keeps the name's own direction. */}
        <span
          data-part="tail"
          style={{ ...part, direction: direction === "rtl" ? "ltr" : "rtl", flexShrink: 0, maxWidth: "100%" }}
        >
          <span dir={direction}>{tail}</span>
        </span>
      </Text>
    </span>
  );
};

export { FileName };
