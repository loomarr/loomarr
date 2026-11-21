import { Text } from "../text";
import type { FileNameProps } from "./file-name.type";
import { visibleFileName } from "./split-file-name";

// The native adapter: a file name in the data face on one line. React Native shortens a line in its
// middle itself, which keeps the end of the name and its extension, and a screen reader reads the
// whole text. A control character is drawn as its visible picture, so the name stays on one line.
// `layout` is a web concern (a native Text already takes its parent's width). The web adapter
// (file-name.web.tsx) draws the same rule with two parts.
const FileName = ({ density = "pointer", name, strong = false, tone = "primary" }: FileNameProps) => (
  <Text
    density={density}
    ellipsizeMode="middle"
    numberOfLines={1}
    strong={strong}
    textRole="data"
    tone={tone}
  >
    {visibleFileName(name)}
  </Text>
);

export { FileName };
