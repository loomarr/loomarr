import { Stack } from "../library-poster";
import { AddFolder } from "./add-folder";
import { foldersDescription } from "./folders-description";
import { LibraryFolderCard } from "./library-folder-card";
import type { LibrarySettingsProps } from "./library-settings.type";
import { LibraryStatus } from "./library-status";
import { PathMap } from "./path-map";
import { Section } from "./section";

// SETTINGS → LIBRARY (#2151 decision 29, design/library): the status block and the media folders.
// Presentational: the app owns the requests. Every word follows the mock's plain-words sheet: a
// folder, never a root; "Loomarr can open this folder", never "readable".

const LibrarySettings = ({
  health,
  folders,
  pathMap,
  scan,
  readySoFar,
  held,
  unreadable,
  hooks,
  details,
  checking,
  onCheckAgain,
  onRescan,
  onRefreshEverything,
  onConfirmRemoval,
  onUseFolder,
  onAddFolder,
  onMapFolder,
  now,
  timeZone,
}: LibrarySettingsProps) => (
  <Stack alignSelf="center" gap="screen" maxWidth={880} width="100%">
    <Section title="Status">
      <LibraryStatus
        checking={checking}
        folders={folders}
        health={health}
        details={details}
        held={held}
        unreadable={unreadable}
        hooks={hooks}
        onCheckAgain={onCheckAgain}
        onConfirmRemoval={onConfirmRemoval}
        onRefreshEverything={onRefreshEverything}
        onRescan={onRescan}
        readySoFar={readySoFar}
        scan={scan}
        timeZone={timeZone}
      />
    </Section>
    <Section description={foldersDescription(pathMap)} title="Media folders">
      <Stack gap="control">
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
      </Stack>
      <PathMap onMapFolder={onMapFolder} pathMap={pathMap} />
    </Section>
  </Stack>
);

export { LibrarySettings };
