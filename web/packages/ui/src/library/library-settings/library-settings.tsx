import { formatAgo } from "@loomarr/core/format";
import { Action, Disclosure, Field, Icon, icons, StatusDot, Surface, Text } from "@loomarr/design-system";
import { type ComponentProps, type ReactNode, useState } from "react";

import type {
  LibraryAuthority,
  LibraryFolder,
  LibraryPathMapping,
  LibrarySettingsProps,
} from "./library-settings.type";

// SETTINGS → LIBRARY (#2151 decision 29, design/library): the status block and the media folders.
// Presentational: the app owns the requests. Every word follows the mock's plain-words sheet: a
// folder, never a root; "Loomarr can open this folder", never "readable".

// Transparent layout boxes: Surface without its card look.
const Stack = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderWidth={0} {...props} />
);
const Line = (props: ComponentProps<typeof Surface>) => (
  <Stack alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline" {...props} />
);

const AUTHORITY_NAMES: Record<LibraryAuthority, string> = {
  emby: "Emby",
  jellyfin: "Jellyfin",
  radarr: "Radarr",
  sonarr: "Sonarr",
};

const joinNames = (names: readonly string[]) =>
  names.length <= 1 ? (names[0] ?? "") : `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;

// "/media/tv" → "TV", "/media/movies" → "Movies", anything else → its last folder name.
const folderName = (path: string) => {
  const last = path.split("/").filter(Boolean).pop() ?? path;
  return last.length <= 2 ? last.toUpperCase() : last.charAt(0).toUpperCase() + last.slice(1);
};

const foundThrough = (folder: LibraryFolder) => {
  if (folder.addedByHand) return "Added by you";
  const names = [...new Set(folder.foundIn.map((f) => AUTHORITY_NAMES[f.authority]))];
  return names.length > 0 ? `Found through ${joinNames(names)}` : "No app names this folder any more";
};

const MOUNT_HINT =
  "Loomarr needs to open your media folders itself. Make them available to Loomarr (read-only is fine), then Check again.";

// A calm warning with one way out.
const Callout = ({
  title,
  children,
  actions,
}: {
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
}) => (
  <Surface borderColor="stateWarning" borderRadius="control" gap="control" padding="section" role="alert">
    <Text textRole="headline" tone="warning">
      {title}
    </Text>
    {children ? (
      <Text textRole="body" tone="secondary">
        {children}
      </Text>
    ) : null}
    {actions ? <Line gap="control">{actions}</Line> : null}
  </Surface>
);

const Section = ({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) => (
  <Stack aria-label={title} gap="control" role="group">
    <Stack gap="inline">
      <Text as="h2" textRole="headline">
        {title}
      </Text>
      {description ? (
        <Text textRole="body" tone="secondary">
          {description}
        </Text>
      ) : null}
    </Stack>
    {children}
  </Stack>
);

const CheckAgain = ({ checking, onPress }: { checking: boolean; onPress: () => void }) => (
  <Action disabled={checking} onPress={onPress} tone="primary">
    {checking ? "Checking…" : "Check again"}
  </Action>
);

const LibraryStatus = ({
  health,
  folders,
  checking,
  onCheckAgain,
}: Pick<LibrarySettingsProps, "health" | "folders" | "checking" | "onCheckAgain">) => {
  const inUse = folders.filter((f) => f.inUse);
  const offline = inUse.filter((f) => !f.readable);
  return (
    <Surface aria-label="Library status" gap="section" padding="section" role="region">
      {health === "ok" ? (
        <Line gap="control">
          <StatusDot label="Up to date" tone="ok" />
          <Text textRole="headline">
            {inUse.length === 1 ? "Using 1 media folder" : `Using ${inUse.length} media folders`}
          </Text>
        </Line>
      ) : null}
      {health === "folder_offline"
        ? offline.map((folder) => (
            <Callout
              actions={<CheckAgain checking={checking} onPress={onCheckAgain} />}
              key={folder.path}
              title={`Can't reach your ${folderName(folder.path)} folder`}
            >
              Nothing has been removed.
            </Callout>
          ))
        : null}
      {health === "needs_folders" ? (
        <Callout title="Tell Loomarr where your media is">
          {folders.length > 0
            ? "Loomarr found these folders. Choose the ones that hold your library and it will start adding your media."
            : "Loomarr didn't find any folders yet. Add the folder that holds your shows and movies."}
        </Callout>
      ) : null}
    </Surface>
  );
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
    borderColor={folder.readable ? "borderDecorative" : "stateWarning"}
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
      {folder.readable ? (
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
      {folder.readable ? null : (
        <Action disabled={checking} onPress={onCheckAgain} variant="secondary">
          Check again
        </Action>
      )}
      {folder.inUse ? (
        <Action onPress={() => onUseFolder(folder.path, false)} variant="ghost">
          Stop using
        </Action>
      ) : folder.readable ? (
        <Action onPress={() => onUseFolder(folder.path, true)} variant="secondary">
          Use this folder
        </Action>
      ) : null}
    </Line>
  </Surface>
);

const AddFolder = ({ onAddFolder }: Pick<LibrarySettingsProps, "onAddFolder">) => {
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string>();
  const [saving, setSaving] = useState(false);
  if (!adding) {
    return (
      <Line>
        <Action icon="plus" onPress={() => setAdding(true)} variant="outline">
          Add a folder
        </Action>
      </Line>
    );
  }
  const submit = async () => {
    setSaving(true);
    const refused = await onAddFolder(draft.trim());
    setSaving(false);
    setError(refused);
    if (!refused) {
      setDraft("");
      setAdding(false);
    }
  };
  return (
    <Line alignItems="flex-end" gap="control">
      <Stack flexBasis={240} flexGrow={1}>
        <Field
          error={error}
          label="Where is the folder?"
          onChangeText={(text) => {
            setDraft(text);
            setError(undefined);
          }}
          onSubmitEditing={() => void submit()}
          placeholder="/media/…"
          value={draft}
        />
      </Stack>
      <Action disabled={draft.trim() === "" || saving} onPress={() => void submit()} tone="primary">
        Add folder
      </Action>
      <Action
        onPress={() => {
          setAdding(false);
          setDraft("");
          setError(undefined);
        }}
        variant="ghost"
      >
        Cancel
      </Action>
    </Line>
  );
};

// One row of the map. The app's own name is fixed; the folder here is edited and saved on leaving
// the field. Clearing it hands the row back to Loomarr.
const PathMapRow = ({
  mapping,
  onMapFolder,
}: {
  mapping: LibraryPathMapping;
  onMapFolder: LibrarySettingsProps["onMapFolder"];
}) => {
  const [value, setValue] = useState(mapping.localPath);
  const name = AUTHORITY_NAMES[mapping.authority];
  const commit = () => {
    const next = value.trim();
    if (next !== mapping.localPath) {
      onMapFolder({ authority: mapping.authority, authorityPath: mapping.authorityPath, localPath: next });
    }
  };
  return (
    <Line alignItems="flex-end" gap="control">
      <Stack paddingBottom="inline" width={72}>
        <Text textRole="label">{name}</Text>
      </Stack>
      <Stack flexBasis={160} flexGrow={1} paddingBottom="inline">
        <Text textRole="data" tone="secondary">
          {mapping.authorityPath}
        </Text>
      </Stack>
      <Text textRole="body" tone="muted">
        →
      </Text>
      <Stack flexBasis={160} flexGrow={1}>
        <Field
          accessibilityLabel={`${name} ${mapping.authorityPath} maps to`}
          onBlur={commit}
          onChangeText={setValue}
          onSubmitEditing={commit}
          placeholder="Not found here"
          value={value}
        />
      </Stack>
    </Line>
  );
};

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

const foldersDescription = (pathMap: readonly LibraryPathMapping[]) => {
  const names = [...new Set(pathMap.map((m) => AUTHORITY_NAMES[m.authority]))];
  return names.length > 0
    ? `Loomarr looks for your shows and movies in these folders. It found them through ${joinNames(names)}.`
    : "Loomarr looks for your shows and movies in these folders.";
};

const LibrarySettings = ({
  health,
  folders,
  pathMap,
  checking,
  onCheckAgain,
  onUseFolder,
  onAddFolder,
  onMapFolder,
  now,
}: LibrarySettingsProps) => (
  <Stack alignSelf="center" gap="screen" maxWidth={880} width="100%">
    <Section title="Status">
      <LibraryStatus checking={checking} folders={folders} health={health} onCheckAgain={onCheckAgain} />
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
