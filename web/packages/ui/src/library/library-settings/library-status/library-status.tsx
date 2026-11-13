import { Action, Surface } from "@loomarr/design-system";

import {
  detailsWaiting,
  folderName,
  heldNames,
  heldTitle,
  unreadableNames,
  unreadableTitle,
} from "../../library-words";
import { Callout } from "../callout";
import { CheckAgain } from "../check-again";
import { Headline } from "../headline";
import { KeepingUp } from "../keeping-up";
import type { LibrarySettingsProps } from "../library-settings.type";
import { ScanActions } from "../scan-actions";

const LibraryStatus = ({
  health,
  folders,
  scan,
  readySoFar,
  held,
  unreadable = [],
  hooks,
  details,
  checking,
  onCheckAgain,
  onRescan,
  onRefreshEverything,
  onConfirmRemoval,
  timeZone,
}: Pick<
  LibrarySettingsProps,
  | "health"
  | "folders"
  | "scan"
  | "readySoFar"
  | "held"
  | "unreadable"
  | "hooks"
  | "details"
  | "checking"
  | "onCheckAgain"
  | "onRescan"
  | "onRefreshEverything"
  | "onConfirmRemoval"
  | "timeZone"
>) => {
  const inUse = folders.filter((f) => f.inUse);
  const offline = inUse.filter((f) => !f.readable);
  // What needs the maintainer leads. An import still running for the other folders keeps its bar,
  // step, file and time left below it; "All caught up" is never said beside something that isn't.
  // An empty folder is said on its own card; here it only keeps "All caught up" from being said.
  const alerted = (health !== "ok" && health !== "folder_empty") || held.length > 0 || unreadable.length > 0;
  const importing = health !== "needs_folders" && scan.step !== "idle";
  // Titles stuck waiting for details are said under Keeping up to date, and keep "All caught up"
  // from being said too.
  const waiting = detailsWaiting(details, scan, timeZone);
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
          {[heldNames(removal), "We haven't removed anything — if a drive is unplugged, plug it back in."]
            .filter(Boolean)
            .join(" ")}
        </Callout>
      ))}
      {unreadable.length > 0 ? (
        <Callout
          actions={
            <Action onPress={onRescan} tone="primary">
              Check again
            </Action>
          }
          title={unreadableTitle(unreadable)}
        >
          {unreadableNames(unreadable)}
        </Callout>
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
      {alerted && importing ? <Headline readySoFar={readySoFar} scan={scan} /> : null}
      {health === "needs_folders" ? null : <KeepingUp hooks={hooks} offline={offline} waiting={waiting} />}
      {health === "needs_folders" ? null : (
        <ScanActions onRefreshEverything={onRefreshEverything} onRescan={onRescan} scan={scan} />
      )}
    </Surface>
  );
};

export { LibraryStatus };
