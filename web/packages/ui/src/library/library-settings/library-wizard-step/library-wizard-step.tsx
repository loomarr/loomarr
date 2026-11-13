import { Action, Surface, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import { joinNames } from "../../library-words";
import { AddFolder } from "../add-folder";
import { AUTHORITY_NAMES } from "../authority-names";
import { Headline } from "../headline";
import { LibraryFolderCard } from "../library-folder-card";
import type { LibraryWizardStepProps } from "../library-settings.type";
import { MOUNT_HINT } from "../mount-hint";

// THE WIZARD'S LIBRARY STEP (#2151 decision 30, design/library variant D): the folders Loomarr
// found, each with Use this folder, and Check again. Choosing one starts adding the media; the
// step shows its progress but never waits for it. The wizard frame owns the heading and Continue.
const LibraryWizardStep = ({
  folders,
  pathMap,
  scan,
  readySoFar,
  checking,
  checkedJustNow,
  onCheckAgain,
  onUseFolder,
  onAddFolder,
  now,
}: LibraryWizardStepProps) => {
  const names = [...new Set(pathMap.map((m) => AUTHORITY_NAMES[m.authority]))];
  const usable = folders.some((f) => f.inUse && f.readable);
  return (
    <Stack gap="section">
      <Text textRole="body" tone="secondary">
        {names.length > 0
          ? `We found these through ${joinNames(names)} — pick the ones that hold your library.`
          : folders.length > 0
            ? "Pick the folders that hold your library."
            : "Loomarr didn't find any folders yet. Add the folder that holds your shows and movies."}
      </Text>
      <Surface gap="control" padding="section">
        {folders.map((folder) => (
          <LibraryFolderCard
            checking={checking}
            folder={folder}
            key={folder.path}
            now={now}
            onCheckAgain={onCheckAgain}
            onUseFolder={onUseFolder}
          />
        ))}
        <AddFolder onAddFolder={onAddFolder} />
        <Line gap="control">
          <Action disabled={checking} onPress={onCheckAgain} variant="outline">
            {checking ? "Checking…" : "Check again"}
          </Action>
          {checkedJustNow ? (
            <Text textRole="metadata" tone="secondary">
              Checked just now
            </Text>
          ) : null}
        </Line>
        {folders.some((f) => f.inUse) ? (
          <>
            <Headline readySoFar={readySoFar} scan={scan} />
            <Text textRole="body" tone="secondary">
              You can carry on — titles appear as they're ready.
            </Text>
          </>
        ) : null}
      </Surface>
      {usable ? null : (
        <Text textRole="body" tone="secondary">
          {MOUNT_HINT}
        </Text>
      )}
    </Stack>
  );
};

export { LibraryWizardStep };
