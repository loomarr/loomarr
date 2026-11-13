import { Disclosure, Text } from "@loomarr/design-system";
import { useState } from "react";

import { Stack } from "../../library-poster";
import { fileName } from "../file-name";
import type { LibraryOrderFile } from "../library-needs.type";
import { plural } from "../plural";

const FILES_SHOWN = 3;
const pad = (n: number) => String(n).padStart(2, "0");

const OrderFiles = ({ files }: { files: readonly LibraryOrderFile[] }) => {
  const [open, setOpen] = useState(false);
  if (files.length === 0) return null;
  return (
    <Disclosure expanded={open} label={plural(files.length, "episode")} onExpandedChange={setOpen}>
      <Stack gap={4} padding="control">
        {files.slice(0, FILES_SHOWN).map((f) => (
          <Text key={f.path} textRole="compact" tone="secondary">
            {`S${pad(f.season)}E${pad(f.episode)} — ${fileName(f.path)}`}
          </Text>
        ))}
        {files.length > FILES_SHOWN ? (
          <Text textRole="hint" tone="muted">{`and ${files.length - FILES_SHOWN} more`}</Text>
        ) : null}
      </Stack>
    </Disclosure>
  );
};

export { OrderFiles };
