import { Field, Text } from "@loomarr/design-system";
import { useRef } from "react";

import { useCommittedDraft } from "../../../use-committed-draft";
import { Line, Stack } from "../../library-poster";
import { AUTHORITY_NAMES } from "../authority-names";
import type { LibraryPathMapping, LibrarySettingsProps } from "../library-settings.type";

// One row of the map. The app's own name is fixed; the folder here is edited and saved on leaving
// the field. Clearing it hands the row back to Loomarr.
const PathMapRow = ({
  mapping,
  onMapFolder,
}: {
  mapping: LibraryPathMapping;
  onMapFolder: LibrarySettingsProps["onMapFolder"];
}) => {
  const { draft: value, editDraft: setValue } = useCommittedDraft(mapping.localPath);
  // What this edit already sent. Enter then leaving the field would otherwise save twice: the
  // second commit runs before the server's answer updates mapping.localPath. Typing clears it, so
  // a save that failed can be sent again.
  const sent = useRef<string | null>(null);
  const name = AUTHORITY_NAMES[mapping.authority];
  const commit = () => {
    const next = value.trim();
    if (next !== mapping.localPath && next !== sent.current) {
      sent.current = next;
      onMapFolder({ authority: mapping.authority, authorityPath: mapping.authorityPath, localPath: next });
    }
  };
  const type = (text: string) => {
    sent.current = null;
    setValue(text);
  };
  return (
    <Line alignItems="flex-end" gap="control">
      <Stack paddingBottom="inline" width={72}>
        <Text textRole="label">{name}</Text>
      </Stack>
      <Stack flexBasis={160} flexGrow={1} flexShrink={1} paddingBottom="inline">
        <Text textRole="data" tone="secondary">
          {mapping.authorityPath}
        </Text>
      </Stack>
      <Text textRole="body" tone="muted">
        →
      </Text>
      <Stack flexBasis={160} flexGrow={1} flexShrink={1}>
        <Field
          accessibilityLabel={`${name} ${mapping.authorityPath} maps to`}
          onBlur={commit}
          onChangeText={type}
          onSubmitEditing={commit}
          placeholder="Not found here"
          value={value}
        />
      </Stack>
    </Line>
  );
};

export { PathMapRow };
