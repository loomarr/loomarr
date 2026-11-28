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
import { Copies, needsCopies, useCopyFocus } from "../copies";
import { Line, PAGE_GUTTER, Stack } from "../library-poster";
import { PickTitle } from "../pick-title";
import { SeasonEpisodes } from "../season-episodes";
import { TechnicalDetails } from "../technical-details";
import { CastAndCrew, Hero, OnChannels, playingLine, useTitleLayout } from "../title-parts";
import type { LibraryTitleDetail, LibraryTitleFix, LibraryTitleProps } from "./library-title.type";

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
// surface, so it reads over any backdrop. A home video has no catalogue to read again.
const FixMenu = ({
  homeVideo,
  fixes,
  triggerRef,
}: {
  homeVideo?: boolean;
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
          {homeVideo ? null : (
            <MenuAction density={density} icon={menuIcon(icons.refresh)} onSelect={fixes.update}>
              Update details
            </MenuAction>
          )}
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

const LibraryTitle = ({ title, onBack, fix, personLink, watchLink }: LibraryTitleProps) => {
  const { wide, density } = useTitleLayout();
  const fixes = useTitleFixes(fix);
  // Closing "Wrong title?" removes the button that was pressed; focus goes back to what opened it.
  const fixTrigger = useRef<ActionRef>(null);
  const copyFocus = useCopyFocus(title.files);
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
              {fix ? <FixMenu fixes={fixes} homeVideo={title.homeVideo} triggerRef={fixTrigger} /> : null}
            </>
          }
          headingRef={copyFocus.pageHeading}
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

      {title.overview ? (
        <Text textRole="body">{title.overview}</Text>
      ) : title.homeVideo ? (
        <Text textRole="body">
          {fix
            ? "A home video you kept as it is. Loomarr won't look it up, and it keeps the file's own name."
            : "A home video from your household's library."}
        </Text>
      ) : null}

      <OnChannels collectionsLink={fix?.collectionsLink} title={title} watchLink={watchLink} />

      <CastAndCrew personLink={personLink} title={title} />

      {/* The store leaves out a broken copy that another copy plays in place of, so what is left
          is broken with nothing in its place: a part, an edition, or the only copy. */}
      {fix && title.unplayable > 0 ? (
        <Unplayable count={title.unplayable} kind={title.kind} onCheck={fix.onCheckFiles} />
      ) : null}

      {/* A series carries no files of its own. */}
      {fix && needsCopies(title.files) ? (
        <Copies files={title.files} focus={copyFocus} onRemove={fix.onRemoveExtra} />
      ) : null}

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
