import { Text } from "../text";
import type { FileNameProps } from "./file-name.type";

// The native adapter: a file name in the data face on one line. React Native shortens a line in its
// middle itself, which keeps the end of the name and its extension, and a screen reader reads the
// whole text. The web adapter (file-name.web.tsx) draws the same rule with two parts.
const FileName = ({ density = "pointer", name, strong = false, tone = "primary" }: FileNameProps) => (
  <Text
    density={density}
    ellipsizeMode="middle"
    numberOfLines={1}
    strong={strong}
    textRole="data"
    tone={tone}
  >
    {name}
  </Text>
);

export { FileName };
