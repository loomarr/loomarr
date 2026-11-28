import { formatBytes } from "@loomarr/core/format";
import {
  Action,
  Icon,
  icons,
  Menu,
  MenuAction,
  MenuContent,
  MenuTrigger,
  StatusDot,
  Surface,
  Text,
} from "@loomarr/design-system";
import { type ComponentProps, type ReactNode, type Ref, useRef, useState } from "react";
import { fileName } from "../file-facts";
import { Line, PAGE_GUTTER, Stack } from "../library-poster";
import { PickTitle } from "../pick-title";
import { SeasonEpisodes } from "../season-episodes";
import { TechnicalDetails } from "../technical-details";
import { CastAndCrew, Hero, OnChannels, playingLine, useTitleLayout } from "../title-parts";
import type {
  LibraryTitleDetail,
  LibraryTitleFile,
  LibraryTitleFix,
  LibraryTitleProps,
} from "./library-title.type";

// The title and what an admin can do about it.
type TitleFixes = LibraryTitleFix & Pick<LibraryTitleProps, "title">;

// A TITLE'S PAGE (#4476), a TV-app page: a backdrop with the title over it, the facts, the story,
// when the channels show it, the people, then the seasons and the files. Presentational: the app
// owns the request, draws the artwork and words the times.

// "Wrong title?" (decision 26) follows what decides: the app's own page, a note when the file
// itself says, else a search, and Forget my answer when the household answered.
const WrongTitle = ({
  title,
  onOpenApp,
  onSearch,
  onLink,
  onForget,
}: Pick<TitleFixes, "title" | "onOpenApp" | "onSearch" | "onLink" | "onForget">) => {
  const [done, setDone] = useState<string>();
  const [failed, setFailed] = useState(false);
  const { density } = useTitleLayout();
  const { by, app } = title.match;
  if (by === "app" && app) {
    return (
      <Surface gap="inline" padding="control">
        <Text textRole="body">{`${app} decides what this is. Change it in ${app}.`}</Text>
        {onOpenApp ? (
          <Line>
            <Action density={density} onPress={onOpenApp} variant="outline">
              {`Open ${app}`}
            </Action>
          </Line>
        ) : null}
      </Surface>
    );
  }
  if (by === "file") {
    return (
      <Surface gap="inline" padding="control">
        <Text textRole="body">
          This file's name says what it is. Rename it, or change its NFO, to fix it.
        </Text>
      </Surface>
    );
  }
  if (done) {
    return (
      <Line>
        <StatusDot label="Done" tone="ok" />
        <Text textRole="body">{`Done. ${done} is sorted.`}</Text>
      </Line>
    );
  }
  const answer = (save: Promise<void>, name?: string) => {
    setFailed(false);
    save.then(
      () => name && setDone(name),
      () => setFailed(true),
    );
  };
  return (
    <Stack gap="inline">
      <PickTitle
        kind={title.kind}
        onPick={(c) => answer(onLink(c.externalId), c.name)}
        onSearch={onSearch}
        others={[]}
      />
      {failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't save your answer. Try again in a moment.
        </Text>
      ) : null}
      {by === "you" ? (
        <Line>
          <Action density={density} onPress={() => answer(onForget())} variant="ghost">
            Forget my answer
          </Action>
        </Line>
      ) : null}
    </Stack>
  );
};

type Busy = "update" | "check";

// The instance the Action draws, which takes focus.
type ActionRef = Extract<ComponentProps<typeof Action>["ref"], { current: unknown }>["current"] & {
  focus: () => void;
};

// The Fix menu's actions and what they said back. A press waits for its answer: while one is out
// another is dropped, a check that went through says so, and one that failed says to try again.
const useTitleFixes = (fix: LibraryTitleFix | undefined) => {
  const [busy, setBusy] = useState<Busy>();
  const [checked, setChecked] = useState(false);
  const [failed, setFailed] = useState<Busy>();
  const [wrong, setWrong] = useState(false);
  const run = (which: Busy, act: () => Promise<void>) => {
    if (busy) return;
    setBusy(which);
    setChecked(false);
    setFailed(undefined);
    act().then(
      () => {
        setBusy(undefined);
        if (which === "check") setChecked(true);
      },
      () => {
        setBusy(undefined);
        setFailed(which);
      },
    );
  };
  const said = busy
    ? busy === "update"
      ? "Updating…"
      : "Checking…"
    : failed
      ? failed === "update"
        ? "Couldn't update the details just now. Try again in a moment."
        : "Couldn't check the files just now. Try again in a moment."
      : checked
        ? "Checked just now"
        : "";
  return {
    said,
    wrong,
    update: () => fix && run("update", fix.onUpdateDetails),
    check: () => fix && run("check", fix.onCheckFiles),
    toggleWrong: () => setWrong(!wrong),
    closeWrong: () => setWrong(false),
  };
};

const menuIcon = (glyph: (typeof icons)[keyof typeof icons]) => (
  <Icon decorative glyph={glyph} size="compact" tone="muted" />
);

// Update details · Check files again · Wrong title?, in one menu. Its trigger sits on a solid
// surface, so it reads over any backdrop.
const FixMenu = ({
  fixes,
  triggerRef,
}: {
  fixes: ReturnType<typeof useTitleFixes>;
  triggerRef: Ref<ActionRef>;
}) => {
  const { density } = useTitleLayout();
  return (
    <Surface backgroundColor="surfaceElevated" borderRadius="control" borderWidth={0}>
      <Menu>
        <MenuTrigger>
          <Action density={density} icon="settingsAdjust" ref={triggerRef} variant="outline">
            Fix
          </Action>
        </MenuTrigger>
        <MenuContent align="end" width={240}>
          <MenuAction density={density} icon={menuIcon(icons.refresh)} onSelect={fixes.update}>
            Update details
          </MenuAction>
          <MenuAction density={density} icon={menuIcon(icons.folder)} onSelect={fixes.check}>
            Check files again
          </MenuAction>
          <MenuAction
            density={density}
            icon={menuIcon(fixes.wrong ? icons.selected : icons.swap)}
            onSelect={fixes.toggleWrong}
          >
            {fixes.wrong ? "Wrong title? (open)" : "Wrong title?"}
          </MenuAction>
        </MenuContent>
      </Menu>
    </Surface>
  );
};

// An extra copy (decision 37) is a calm warning with one primary action. The card stays until the
// page refetches without the file; a press while one is out is dropped.
const ExtraCopy = ({
  file,
  name,
  onRemove,
}: {
  file: LibraryTitleFile;
  name: string;
  onRemove: (path: string) => Promise<void>;
}) => {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const { density } = useTitleLayout();
  const remove = () => {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    onRemove(file.path).then(
      () => setBusy(false),
      () => {
        setBusy(false);
        setFailed(true);
      },
    );
  };
  return (
    <Surface borderColor="stateWarning" gap="control" padding="section">
      <Text as="h2" textRole="title">{`An extra copy is taking ${formatBytes(file.size)}`}</Text>
      <Text textRole="body" tone="secondary">
        Loomarr plays the better copy, so this one never plays. Removing it moves it to Loomarr's trash. You
        can undo right after. After that, the file is in the .loomarr-trash folder inside its media folder for
        30 days.
      </Text>
      {failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't remove it. Loomarr may not be allowed to change this folder.
        </Text>
      ) : null}
      <Line>
        <Action density={density} onPress={remove} variant="outline">
          {`Remove ${name}`}
        </Action>
      </Line>
    </Surface>
  );
};

// A file that can't be played (decision 24, the word sheet's "1 episode can't be played — the file
// may be damaged" + "Check again"): a calm warning whose one action reads the files again.
const Unplayable = ({
  kind,
  count,
  onCheck,
}: {
  kind: LibraryTitleDetail["kind"];
  count: number;
  onCheck: () => Promise<void>;
}) => {
  const [busy, setBusy] = useState(false);
  const [checked, setChecked] = useState(false);
  const [failed, setFailed] = useState(false);
  const { density } = useTitleLayout();
  const check = () => {
    if (busy) return;
    setBusy(true);
    setFailed(false);
    onCheck().then(
      () => {
        setBusy(false);
        setChecked(true);
      },
      () => {
        setBusy(false);
        setFailed(true);
      },
    );
  };
  const what =
    kind === "movie"
      ? "This movie can't be played. The file may be damaged"
      : count === 1
        ? "1 episode can't be played. The file may be damaged"
        : `${count} episodes can't be played. The files may be damaged`;
  return (
    <Surface borderColor="stateWarning" gap="control" padding="section">
      <Text as="h2" textRole="title">
        {what}
      </Text>
      {failed ? (
        <Text textRole="body" tone="secondary">
          Couldn't check the files just now. Try again in a moment.
        </Text>
      ) : null}
      <Line>
        <Action density={density} onPress={check} tone="primary">
          {checked ? "Checked just now" : "Check files again"}
        </Action>
      </Line>
    </Surface>
  );
};

const LibraryTitle = ({ title, onBack, fix }: LibraryTitleProps) => {
  const { wide, density } = useTitleLayout();
  const fixes = useTitleFixes(fix);
  // Closing "Wrong title?" removes the button that was pressed; focus goes back to what opened it.
  const fixTrigger = useRef<ActionRef>(null);
  const closeWrong = () => {
    fixes.closeWrong();
    fixTrigger.current?.focus();
  };
  const technical = !fix && title.playing ? playingLine(title.playing) : "";
  const solid = (action: ReactNode) => (
    <Surface backgroundColor="surfaceElevated" borderRadius="control" borderWidth={0}>
      {action}
    </Surface>
  );
  return (
    <Stack
      alignSelf="center"
      gap="screen"
      maxWidth={1152 + 2 * PAGE_GUTTER.wide}
      padding={wide ? PAGE_GUTTER.wide : PAGE_GUTTER.phone}
      width="100%"
    >
      <Stack gap="control">
        <Hero
          actions={
            <>
              {solid(
                <Action density={density} icon="back" onPress={onBack} variant="ghost">
                  Library
                </Action>,
              )}
              {fix ? <FixMenu fixes={fixes} triggerRef={fixTrigger} /> : null}
            </>
          }
          title={title}
        />
        {technical ? (
          <Text textRole="compact" tone="muted">
            {technical}
          </Text>
        ) : null}
        {/* Mounted before it has anything to say: a live region added with its words is often missed. */}
        {fix ? (
          <Text aria-live="polite" textRole="body" tone="secondary">
            {fixes.said}
          </Text>
        ) : null}
        {fix && fixes.wrong ? (
          <Stack gap="inline">
            <WrongTitle title={title} {...fix} />
            <Line>
              <Action density={density} icon="close" onPress={closeWrong} variant="ghost">
                Close
              </Action>
            </Line>
          </Stack>
        ) : null}
      </Stack>

      {title.overview ? <Text textRole="body">{title.overview}</Text> : null}

      <OnChannels title={title} />

      <CastAndCrew title={title} />

      {fix && title.unplayable > 0 ? (
        <Unplayable count={title.unplayable} kind={title.kind} onCheck={fix.onCheckFiles} />
      ) : null}

      {fix
        ? title.files
            .filter((file) => file.extra)
            .map((file) => (
              <ExtraCopy
                file={file}
                key={file.path}
                name={fileName(
                  file.path,
                  title.files.map((f) => f.path),
                )}
                onRemove={fix.onRemoveExtra}
              />
            ))
        : null}

      {fix?.onChooseOrder && title.seasons.some((s) => s.episodes.some((e) => e.fromFile)) ? (
        <Surface
          backgroundColor="stateWarningSurface"
          borderWidth={0}
          paddingHorizontal={14}
          paddingVertical={10}
        >
          <Line columnGap="control" flexWrap="wrap" rowGap="inline">
            <Line flexBasis={240} flexGrow={1} flexWrap="nowrap" minWidth={0}>
              <StatusDot label="Needs you" tone="warn" />
              <Text textRole="compact">We're not sure which order these episodes are in.</Text>
            </Line>
            <Action density={density} onPress={fix.onChooseOrder} variant="outline">
              Choose
            </Action>
          </Line>
        </Surface>
      ) : null}

      {title.seasons.length > 0 ? (
        <SeasonEpisodes
          onEpisodeFiles={fix?.onEpisodeFiles}
          readAt={fix?.episodeFilesAt}
          seasons={title.seasons}
        />
      ) : null}

      {/* A member's title carries no files or match to tell. */}
      {fix ? <TechnicalDetails density={density} title={title} /> : null}
    </Stack>
  );
};

export { LibraryTitle };
