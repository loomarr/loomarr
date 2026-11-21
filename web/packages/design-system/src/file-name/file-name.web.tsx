/// <reference lib="dom" />
import { Text } from "../text";
import type { FileNameProps } from "./file-name.type";
import { splitFileName } from "./split-file-name";

const part = { minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "pre" } as const;

/**
 * The web adapter: a file name in the data face on one line. CSS shortens only a line's end, so the
 * name is drawn in two parts that together are the name once (for copy, find-in-page and assistive
 * tech alike): the head gives way first with an ellipsis, then the tail (the end of the name and its
 * extension) with its own when even it does not fit. The row follows the name's own direction, so a
 * right-to-left name keeps its ellipsis beside its tail. The whole name shows on hover.
 *
 * It sits inline in a sentence or a link, and never draws outside its own box. It shortens only when
 * its parent bounds its width: a block, a flex or grid item that may shrink (`min-width: 0`), or a
 * table cell with a set width. An auto-layout table cell lets it shorten only as far as the table is
 * squeezed, down to the tail's width.
 */
const FileName = ({ density = "pointer", name, strong = false, tone = "primary" }: FileNameProps) => {
  const { head, tail } = splitFileName(name);
  // The outer span is the box (inline, clipped, in the name's direction); the Text inside it sets
  // the face and colour and lays out as its contents, so the two parts are the box's flex items.
  return (
    <span
      dir="auto"
      style={{ display: "inline-flex", maxWidth: "100%", minWidth: 0, overflow: "hidden" }}
      title={name}
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
          <span data-part="head" style={{ ...part, flexShrink: 1 }}>
            {head}
          </span>
        ) : null}
        <span data-part="tail" style={{ ...part, flexShrink: 0, maxWidth: "100%" }}>
          {tail}
        </span>
      </Text>
    </span>
  );
};

export { FileName };
