/// <reference lib="dom" />
import { Text } from "../text";
import type { FileNameProps } from "./file-name.type";
import { splitFileName } from "./split-file-name";

const visuallyHidden = {
  border: 0,
  clip: "rect(0 0 0 0)",
  height: 1,
  margin: -1,
  overflow: "hidden",
  padding: 0,
  position: "absolute",
  whiteSpace: "nowrap",
  width: 1,
} as const;

// The web adapter: a file name in the data face on one line. CSS shortens only a line's end, so the
// name is drawn in two parts: the head gives way with an ellipsis and the tail (the end of the name
// and its extension) never shrinks. The drawn parts are hidden from assistive tech, which reads the
// whole name once from a visually hidden copy; a pointer reads it from the title on hover.
const FileName = ({ density = "pointer", name, strong = false, tone = "primary" }: FileNameProps) => {
  const { head, tail } = splitFileName(name);
  return (
    <span style={{ display: "flex", maxWidth: "100%", minWidth: 0 }} title={name}>
      <Text
        as="span"
        density={density}
        strong={strong}
        style={{ display: "flex", minWidth: 0, position: "relative" }}
        textRole="data"
        tone={tone}
      >
        <span style={visuallyHidden}>{name}</span>
        {head ? (
          <span
            aria-hidden
            data-part="head"
            style={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "pre" }}
          >
            {head}
          </span>
        ) : null}
        <span aria-hidden data-part="tail" style={{ flexShrink: 0, whiteSpace: "pre" }}>
          {tail}
        </span>
      </Text>
    </span>
  );
};

export { FileName };
