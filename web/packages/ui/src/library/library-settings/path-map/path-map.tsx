import { Disclosure, Text } from "@loomarr/design-system";
import { useState } from "react";

import { Stack } from "../../library-poster";
import { joinNames } from "../../library-words";
import { AUTHORITY_NAMES } from "../authority-names";
import type { LibrarySettingsProps } from "../library-settings.type";
import { PathMapRow } from "../path-map-row";

const PathMap = ({ pathMap, onMapFolder }: Pick<LibrarySettingsProps, "pathMap" | "onMapFolder">) => {
  const [open, setOpen] = useState(false);
  if (pathMap.length === 0) return null;
  return (
    <Disclosure expanded={open} label="Folders have different names in other apps" onExpandedChange={setOpen}>
      <Stack gap="control" paddingTop="inline">
        <Text textRole="body" tone="secondary">
          {`${joinNames([...new Set(pathMap.map((m) => AUTHORITY_NAMES[m.authority]))])} each call these folders something different. This matches them up. Only change it if something looks wrong.`}
        </Text>
        {pathMap.map((mapping) => (
          <PathMapRow
            // Re-seed the row's draft when the saved value changes.
            key={`${mapping.authority}|${mapping.authorityPath}|${mapping.localPath}`}
            mapping={mapping}
            onMapFolder={onMapFolder}
          />
        ))}
      </Stack>
    </Disclosure>
  );
};

export { PathMap };
