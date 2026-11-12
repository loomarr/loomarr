import { formatAgo } from "@loomarr/core/format";
import {
  Action,
  Disclosure,
  Field,
  Icon,
  icons,
  ProgressTrack,
  StatusDot,
  Surface,
  Text,
} from "@loomarr/design-system";
import { useStore } from "@tanstack/react-form";
import { type ComponentProps, type ReactNode, useRef, useState } from "react";

import { useUiForm } from "../../form-kit";
import { ModalOverlay } from "../../overlay";
import { useCommittedDraft } from "../../use-committed-draft";
import { LibraryPoster, Twirl } from "../library-poster";
import { detailsWaiting, folderName, heldTitle, joinNames, scanHeadline } from "../library-words";
import type {
  LibraryAuthority,
  LibraryFolder,
  LibraryHook,
  LibraryPathMapping,
  LibraryReadyTitle,
  LibraryScan,
  LibrarySettingsProps,
  LibraryWizardStepProps,
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

const foundThrough = (folder: LibraryFolder) => {
  if (folder.addedByHand) return "Added by you";
  const names = [...new Set(folder.foundIn.map((f) => AUTHORITY_NAMES[f.authority]))];
  return names.length > 0 ? `Found through ${joinNames(names)}` : "No app names this folder any more";
};

const MOUNT_HINT =
  "Loomarr needs to open your media folders itself. Make them available to Loomarr (read-only is fine), then Check again.";

// A calm warning with one way out. It stands for as long as the state lasts, so it is a polite
// status, not an alert that interrupts the reader each time the page draws it.
const Callout = ({
  title,
  children,
  actions,
}: {
  title: string;
  children?: ReactNode;
  actions?: ReactNode;
}) => (
  <Surface borderColor="stateWarning" borderRadius="control" gap="control" padding="section" role="status">
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

// The latest few titles that became ready while the import runs (decision 29): a plain 2:3 tile
// each, too narrow to set the name in, so it is the one-line caption under it.
const READY_SO_FAR = 5;
const ReadySoFar = ({ titles }: { titles: readonly LibraryReadyTitle[] }) => (
  <Stack gap="inline">
    <Text textRole="label" tone="secondary">
      Ready so far
    </Text>
    <Line gap="control">
      {titles.slice(0, READY_SO_FAR).map((t) => (
        <Stack key={t.id} width={64}>
          <LibraryPoster art={t.poster} name="" />
          {/* A clipped, min-width-0 box is what lets the design system truncate. */}
          <Stack marginTop={6} minWidth={0} overflow="hidden" width={64}>
            <Text numberOfLines={1} textRole="label" tone="secondary">
              {t.name}
            </Text>
          </Stack>
        </Stack>
      ))}
    </Line>
  </Stack>
);

// The one line at the top of the status while nothing needs the maintainer.
const Headline = ({
  scan,
  readySoFar = [],
  notAllThere = false,
}: {
  scan: LibraryScan;
  readySoFar?: readonly LibraryReadyTitle[];
  /** A folder in use held nothing, so idle says "Up to date", never "All caught up". */
  notAllThere?: boolean;
}) => {
  const words = scanHeadline(scan, notAllThere);
  if (words.quiet) {
    // An hourly pass with nothing to add: one small line, not the block.
    return (
      <Line gap="control">
        <Twirl />
        <Text textRole="body" tone="secondary">
          {words.text}
        </Text>
      </Line>
    );
  }
  const reading = scan.step === "reading";
  return (
    <>
      <Line gap="control">
        {/* A step moving twirls; paused, done or off keeps the still dot. */}
        {words.moving ? <Twirl /> : <StatusDot label={words.text} tone={words.tone} />}
        <Text textRole="headline">{words.text}</Text>
      </Line>
      {words.running ? (
        <Stack gap="control">
          {/* Adding media is preparation (`tune`); paused drops to the neutral fill. */}
          <ProgressTrack
            height={6}
            label={`Adding your media, ${words.percent} percent`}
            percent={words.percent}
            tone={scan.paused ? "elapsed" : "tune"}
          />
          <Text textRole="body" tone="secondary">
            {words.stepLine}
          </Text>
          {scan.paused ? (
            <Text textRole="body" tone="secondary">
              Picks up again when everyone's done watching.
            </Text>
          ) : (
            <>
              <Text textRole="body" tone="secondary">
                {words.timeLine}
              </Text>
              {words.current ? (
                // A clipped, min-width-0 box is what lets the design system truncate.
                <Stack minWidth={0} overflow="hidden">
                  <Text numberOfLines={1} textRole="mono" tone="secondary">
                    {words.current}
                  </Text>
                </Stack>
              ) : null}
              {reading ? (
                <Text textRole="body" tone="secondary">
                  Titles appear as they're ready.
                </Text>
              ) : null}
              {reading && readySoFar.length > 0 ? <ReadySoFar titles={readySoFar} /> : null}
            </>
          )}
        </Stack>
      ) : null}
    </>
  );
};

const HOOK_NAMES: Record<LibraryHook["app"], string> = { sonarr: "Sonarr", radarr: "Radarr" };

// What keeps the library current: the folder watch, and each connected arr's webhook.
const KeepingUp = ({
  offline,
  hooks,
  waiting,
}: {
  offline: readonly LibraryFolder[];
  hooks: readonly LibraryHook[];
  /** Titles stuck waiting for their details, and why; undefined when none are. */
  waiting?: ReturnType<typeof detailsWaiting>;
}) => {
  const signals = [
    {
      key: "watch",
      label: "Loomarr notices new and deleted files on its own",
      waitingFor: offline.length > 0 ? joinNames(offline.map((f) => folderName(f.path))) : "",
    },
    ...hooks
      .filter((hook) => hook.state !== "not_configured")
      .map((hook) => ({
        key: hook.app,
        label: `${HOOK_NAMES[hook.app]} tells Loomarr about new downloads`,
        waitingFor:
          hook.state === "registered"
            ? ""
            : hook.state === "no_address"
              ? "Loomarr's address"
              : HOOK_NAMES[hook.app],
      })),
  ];
  return (
    <Stack gap="control">
      <Text textRole="label" tone="secondary">
        Keeping up to date
      </Text>
      <Stack gap="inline">
        {signals.map((signal) => (
          <Line alignItems="flex-start" flexWrap="nowrap" gap="inline" key={signal.key}>
            {signal.waitingFor ? (
              <StatusDot label="Waiting" size="compact" tone="warn" />
            ) : (
              <Icon decorative glyph={icons.success} size="compact" tone="success" />
            )}
            <Text textRole="body">
              {signal.waitingFor ? `${signal.label} — waiting for ${signal.waitingFor}` : signal.label}
            </Text>
          </Line>
        ))}
        {waiting ? (
          <Line alignItems="flex-start" flexWrap="nowrap" gap="inline">
            <StatusDot label="Waiting" size="compact" tone="warn" />
            <Stack gap="inline">
              <Text textRole="body">{waiting.title}</Text>
              <Text textRole="body" tone="secondary">
                {waiting.reason}
              </Text>
            </Stack>
          </Line>
        ) : null}
      </Stack>
    </Stack>
  );
};

// Scan now and Refresh everything (Emby and Jellyfin's Scan library and Replace all metadata).
// Scan now is a pass now; Refresh everything is Update details for every title, so it asks first.
const ScanActions = ({
  scan,
  onRescan,
  onRefreshEverything,
}: Pick<LibrarySettingsProps, "scan" | "onRescan" | "onRefreshEverything">) => {
  const [asking, setAsking] = useState(false);
  // Looking up details runs on its own clock and never holds up a scan.
  const scanning = scan.step === "walking" || scan.step === "reading";
  return (
    <Line gap="control">
      <ModalOverlay
        actions={[
          { label: "Not now", onPress: () => setAsking(false), preferredFocus: true, tone: "secondary" },
          {
            label: "Refresh everything",
            onPress: () => {
              setAsking(false);
              onRefreshEverything();
            },
            tone: "primary",
          },
        ]}
        description="Loomarr reads every title's details again, from your files and your apps, and looks each one up again. Everything keeps playing. A big library takes a while."
        onDismiss={() => setAsking(false)}
        title="Refresh every title?"
        visible={asking}
      />
      <Action disabled={!scan.enabled || scanning} onPress={onRescan} variant="outline">
        {scanning ? "Scanning…" : "Scan now"}
      </Action>
      <Action disabled={!scan.enabled} onPress={() => setAsking(true)} variant="ghost">
        Refresh everything
      </Action>
    </Line>
  );
};

const LibraryStatus = ({
  health,
  folders,
  scan,
  readySoFar,
  held,
  hooks,
  details,
  checking,
  onCheckAgain,
  onRescan,
  onRefreshEverything,
  onConfirmRemoval,
}: Pick<
  LibrarySettingsProps,
  | "health"
  | "folders"
  | "scan"
  | "readySoFar"
  | "held"
  | "hooks"
  | "details"
  | "checking"
  | "onCheckAgain"
  | "onRescan"
  | "onRefreshEverything"
  | "onConfirmRemoval"
>) => {
  const inUse = folders.filter((f) => f.inUse);
  const offline = inUse.filter((f) => !f.readable);
  // What needs the maintainer leads. An import still running for the other folders keeps its bar,
  // step, file and time left below it; "All caught up" is never said beside something that isn't.
  // An empty folder is said on its own card; here it only keeps "All caught up" from being said.
  const alerted = (health !== "ok" && health !== "folder_empty") || held.length > 0;
  const importing = health !== "needs_folders" && scan.step !== "idle";
  // Titles stuck waiting for details are said under Keeping up to date, and keep "All caught up"
  // from being said too.
  const waiting = detailsWaiting(details, scan);
  return (
    <Surface aria-label="Library status" gap="section" padding="section" role="region">
      {alerted ? null : (
        <Headline
          notAllThere={health === "folder_empty" || waiting !== undefined}
          readySoFar={readySoFar}
          scan={scan}
        />
      )}
      {held.map((removal) => (
        <Callout
          actions={
            <>
              <Action onPress={onRescan} tone="primary">
                Check again
              </Action>
              <Action onPress={() => onConfirmRemoval(removal.path)} tone="danger">
                They're gone, remove them
              </Action>
            </>
          }
          key={removal.path}
          title={heldTitle(removal)}
        >
          We haven't removed anything — if a drive is unplugged, plug it back in.
        </Callout>
      ))}
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
      {alerted && importing ? <Headline readySoFar={readySoFar} scan={scan} /> : null}
      {health === "needs_folders" ? null : <KeepingUp hooks={hooks} offline={offline} waiting={waiting} />}
      {health === "needs_folders" ? null : (
        <ScanActions onRefreshEverything={onRefreshEverything} onRescan={onRescan} scan={scan} />
      )}
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

const AddFolder = ({ onAddFolder }: Pick<LibrarySettingsProps, "onAddFolder">) => {
  const [adding, setAdding] = useState(false);
  // The server's refusal (a path it cannot open) is not the form's own validation: it stays under the
  // field until the path is edited.
  const [error, setError] = useState<string>();
  const form = useUiForm({
    defaultValues: { path: "" },
    onSubmit: async ({ formApi, value }) => {
      const refused = await onAddFolder(value.path.trim());
      setError(refused);
      if (!refused) {
        formApi.reset();
        setAdding(false);
      }
    },
  });
  const path = useStore(form.store, (current) => current.values.path);
  const saving = useStore(form.store, (current) => current.isSubmitting);
  if (!adding) {
    return (
      <Line>
        <Action icon="plus" onPress={() => setAdding(true)} variant="outline">
          Add a folder
        </Action>
      </Line>
    );
  }
  const submit = () => void form.handleSubmit();
  return (
    <Line alignItems="flex-end" gap="control">
      <Stack flexBasis={240} flexGrow={1}>
        <form.AppField listeners={{ onChange: () => setError(undefined) }} name="path">
          {(field) => (
            <field.TextField
              error={error}
              label="Where is the folder?"
              onSubmitEditing={submit}
              placeholder="/media/…"
            />
          )}
        </form.AppField>
      </Stack>
      <Action disabled={path.trim() === "" || saving} onPress={submit} tone="primary">
        Add folder
      </Action>
      <Action
        onPress={() => {
          setAdding(false);
          form.reset();
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
          onChangeText={type}
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
  scan,
  readySoFar,
  held,
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
}: LibrarySettingsProps) => (
  <Stack alignSelf="center" gap="screen" maxWidth={880} width="100%">
    <Section title="Status">
      <LibraryStatus
        checking={checking}
        folders={folders}
        health={health}
        details={details}
        held={held}
        hooks={hooks}
        onCheckAgain={onCheckAgain}
        onConfirmRemoval={onConfirmRemoval}
        onRefreshEverything={onRefreshEverything}
        onRescan={onRescan}
        readySoFar={readySoFar}
        scan={scan}
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

export { LibrarySettings, LibraryWizardStep };
