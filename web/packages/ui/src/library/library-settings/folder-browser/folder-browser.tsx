import { Action, Icon, icons, PressableRow, StatusLabel, Surface, Text } from "@loomarr/design-system";

import { Line, Stack } from "../../library-poster";
import { folderName, folderSummary } from "../../library-words";
import type {
  LibraryBrowseFolder,
  LibraryBrowseListing,
  LibraryFolderBrowser,
} from "../library-settings.type";

// THE FOLDER BROWSER (#2151): Add a folder by opening folders, the apps' folders first, then this
// machine's drives and shares, instead of typing a path. Each row opens its folder; "Use this
// folder" adds the folder open now. What a folder holds arrives after it is listed (a
// library_folder frame), so a slow share never holds the list up. A folder inside one in use can't
// be added; one holding folders in use is added in their place.

const rowNote = (folder: LibraryBrowseFolder) => {
  if (folder.inUse) return "In use";
  if (folder.within) return `Part of ${folderName(folder.within)}`;
  if (folder.holds.length > 0)
    return `Holds ${folder.holds.length === 1 ? "a folder" : `${folder.holds.length} folders`} in use`;
  return undefined;
};

const FolderRow = ({
  folder,
  divided,
  onOpen,
}: {
  folder: LibraryBrowseFolder;
  divided: boolean;
  onOpen: (path: string) => void;
}) => {
  const note = rowNote(folder);
  return (
    <PressableRow
      accessibilityHint={`Opens ${folder.name}`}
      divided={divided}
      onPress={() => onOpen(folder.path)}
    >
      <Icon decorative glyph={icons.folder} size="compact" tone="muted" />
      <Stack alignItems="flex-start" flexBasis={0} flexGrow={1} flexShrink={1} gap="inline">
        <Text textRole="label">{folder.name}</Text>
        <Text textRole="hint" tone="muted">
          {folder.named ? `From your apps · ${folderSummary(folder.summary)}` : folderSummary(folder.summary)}
        </Text>
      </Stack>
      {note ? <StatusLabel tone={folder.inUse ? "ok" : "off"}>{note}</StatusLabel> : null}
      <Icon decorative glyph={icons.disclosure} size="compact" tone="muted" />
    </PressableRow>
  );
};

// What "Use this folder" does for the folder open now, or why it can't.
const choiceFor = (listing: LibraryBrowseListing | undefined) => {
  if (!listing || listing.path === "")
    return { disabled: true, label: "Use this folder", hint: "Open a folder to use it." };
  if (listing.inUse) return { disabled: true, label: "In use", hint: "Loomarr already reads this folder." };
  if (listing.within) {
    return {
      disabled: true,
      label: "Use this folder",
      hint: `Loomarr already reads this as part of ${folderName(listing.within)}.`,
    };
  }
  if (listing.holds.length > 0) {
    const n = listing.holds.length;
    return {
      disabled: false,
      replace: true,
      label: "Use this folder instead",
      hint: `Loomarr reads ${n === 1 ? "a folder" : `${n} folders`} inside it now. Using this one reads everything in it as one folder.`,
    };
  }
  return { disabled: false, label: "Use this folder", hint: undefined };
};

const FolderBrowser = ({
  browser,
  saving,
  refusal,
  onUse,
  onTypePath,
  onCancel,
}: {
  browser: LibraryFolderBrowser;
  saving: boolean;
  /** Why the server refused the folder last chosen. */
  refusal?: string;
  onUse: (path: string, replace: boolean) => void;
  onTypePath: () => void;
  onCancel: () => void;
}) => {
  const { listing, opening, error, onOpen } = browser;
  // While a folder opens, the header names it and the last folder's rows wait under "Opening…";
  // Up and "Use this folder" wait for it to be listed. One that can't be opened leaves Up to go
  // back to the folder it was picked from.
  const waiting = opening !== undefined;
  const upTo = waiting && error ? (listing?.path ?? "") : (listing?.parent ?? "");
  const path = opening ?? listing?.path ?? "";
  const choice = waiting ? { disabled: true, label: "Use this folder", hint: undefined } : choiceFor(listing);
  const folders = listing?.folders ?? [];
  return (
    <Stack gap="control">
      <Line alignItems="center" flexWrap="nowrap" gap="control">
        {path !== "" ? (
          <Action disabled={waiting && !error} icon="goBack" onPress={() => onOpen(upTo)} variant="ghost">
            Up
          </Action>
        ) : null}
        {path === "" ? (
          <Text textRole="body" tone="secondary">
            Your apps' folders and this machine's drives and shares
          </Text>
        ) : (
          <Text flexShrink={1} textRole="data" tone="muted">
            {path}
          </Text>
        )}
      </Line>
      <Surface
        backgroundColor="transparent"
        borderColor="$borderDecorative"
        borderRadius={12}
        borderWidth={1}
      >
        {error ? (
          <Text padding="section" textRole="body" tone="warning">
            {error}
          </Text>
        ) : waiting || !listing ? (
          <Text padding="section" textRole="body" tone="secondary">
            Opening…
          </Text>
        ) : folders.length === 0 ? (
          <Text padding="section" textRole="body" tone="secondary">
            {path === ""
              ? "Loomarr can't see any drives or shares yet. Type the folder's path instead."
              : "No folders in here."}
          </Text>
        ) : (
          folders.map((folder, index) => (
            <FolderRow
              divided={index < folders.length - 1}
              folder={folder}
              key={folder.path}
              onOpen={onOpen}
            />
          ))
        )}
      </Surface>
      {refusal ? (
        <Text textRole="body" tone="warning">
          {refusal}
        </Text>
      ) : choice.hint ? (
        <Text textRole="body" tone="secondary">
          {choice.hint}
        </Text>
      ) : null}
      <Line gap="control">
        <Action
          disabled={choice.disabled || saving}
          onPress={() => onUse(path, choice.replace === true)}
          tone="primary"
        >
          {saving ? "Adding…" : choice.label}
        </Action>
        <Action onPress={onTypePath} variant="ghost">
          Type a path instead
        </Action>
        <Action onPress={onCancel} variant="ghost">
          Cancel
        </Action>
      </Line>
    </Stack>
  );
};

export { FolderBrowser };
