import { formatAgo } from "@loomarr/core/format";
import { Action, Icon, icons, StatusDot, Surface, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import { folderName, joinNames } from "../../library-words";
import { AUTHORITY_NAMES } from "../authority-names";
import type { LibraryFolder } from "../library-settings.type";
import { MOUNT_HINT } from "../mount-hint";

const foundThrough = (folder: LibraryFolder) => {
  if (folder.addedByHand) return "Added by you";
  const names = [...new Set(folder.foundIn.map((f) => AUTHORITY_NAMES[f.authority]))];
  return names.length > 0 ? `Found through ${joinNames(names)}` : "No app names this folder any more";
};

const LibraryFolderCard = ({
  folder,
  now,
  checking,
  onCheckAgain,
  onUseFolder,
}: {
  folder: LibraryFolder;
  now?: number;
  checking: boolean;
  onCheckAgain: () => void;
  onUseFolder: (path: string, inUse: boolean) => void;
}) => (
  <Surface
    aria-label={folderName(folder.path)}
    borderColor={folder.readable && !folder.empty ? "borderDecorative" : "stateWarning"}
    borderRadius="control"
    gap="control"
    padding="section"
    role="group"
  >
    <Line alignItems="flex-start" flexWrap="nowrap" gap="control">
      <Stack flexBasis={0} flexGrow={1} flexShrink={1} gap="inline">
        <Text as="h3" textRole="headline">
          {folderName(folder.path)}
        </Text>
        <Text textRole="data" tone="muted">
          {folder.path}
        </Text>
      </Stack>
      {folder.inUse && folder.readable ? (
        <Text textRole="label" tone="success">
          In use
        </Text>
      ) : null}
    </Line>
    <Line flexWrap="nowrap" gap="inline">
      {folder.readable && folder.empty ? (
        <>
          <StatusDot label="Empty" tone="warn" />
          <Text textRole="body" tone="warning">
            This folder is empty — is the drive connected?
          </Text>
        </>
      ) : folder.readable ? (
        <>
          <Icon decorative glyph={icons.success} size="compact" tone="success" />
          <Text textRole="body" tone="success">
            Loomarr can open this folder
          </Text>
        </>
      ) : (
        <>
          <StatusDot label="Can't open" tone="warn" />
          <Text textRole="body" tone="warning">
            Loomarr can't open this folder right now
          </Text>
        </>
      )}
    </Line>
    <Text textRole="body" tone="secondary">
      {foundThrough(folder)}
      {" · "}
      {folder.checkedAt ? `Checked ${formatAgo(folder.checkedAt, now)}` : "Not checked yet"}
    </Text>
    {folder.readable ? null : (
      <Text textRole="body" tone="secondary">
        {MOUNT_HINT}
      </Text>
    )}
    <Line gap="control">
      {/* Once the drive is back, Check again clears the warning without waiting for the next pass. */}
      {folder.readable && !folder.empty ? null : (
        <Action disabled={checking} onPress={onCheckAgain} variant="outline">
          Check again
        </Action>
      )}
      {folder.inUse ? (
        <Action onPress={() => onUseFolder(folder.path, false)} variant="ghost">
          Stop using
        </Action>
      ) : folder.readable ? (
        <Action onPress={() => onUseFolder(folder.path, true)} variant="outline">
          Use this folder
        </Action>
      ) : null}
    </Line>
  </Surface>
);

export { LibraryFolderCard };
