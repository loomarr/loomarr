import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import type { ChannelListEvent, ChannelListNotice } from "@loomarr/core/channel-list";
import {
  cursorProgramme,
  type GuideViewEvent,
  guideViewLook,
  guideViewRows,
  guideViewSpan,
} from "@loomarr/core/guide-view";
import { Action, decadeTint, Layer, RingedPressable, Stack, Surface, Text } from "@loomarr/design-system";
import { type ReactNode, useEffect, useImperativeHandle, useRef, useState } from "react";
import { ScrollView } from "react-native";

import { ChannelGroupTabs } from "../channel-list";
import { FilmStrip, groupSeriesRuns, type StripAiring } from "../film-strip";
import { ProgrammeFacts, StripMessage } from "../programme-facts";
import { StatePanel } from "../state-panel";
import type { GuideViewEntry, GuideViewKey, GuideViewProps, GuideViewReady } from "./guide-view.type";
import { Head } from "./head";
import { Row, type RowHandle } from "./row";
import { Ruler } from "./ruler";

// The row's channel column, the compact rows' height and the screen's gutter, per layout.
const SIZES = {
  desktop: { gutter: 24, label: 220, row: 44 },
  phone: { gutter: 16, label: 0, row: 44 },
  television: { gutter: 28, label: 200, row: 32 },
} as const;

/** The channel list's words for a group with nothing of its own. */
const NOTICES: Partial<Record<ChannelListNotice, string>> = {
  "empty-decade": "No channels have a decade yet.",
  "empty-favourites": "No favourites yet.",
  "empty-recent": "Channels you watch collect here.",
};

const defaultArtwork = (airing: StripAiring) => airing.thumbUrl || airing.backdropImage?.src || undefined;

/** What the part needs of its own container on the web. */
interface Root {
  contains: (other: unknown) => boolean;
  focus: () => void;
  querySelector: (selector: string) => { focus: () => void } | null;
}

interface KeyEvent {
  altKey: boolean;
  ctrlKey: boolean;
  key: string;
  metaKey: boolean;
  preventDefault: () => void;
  stopPropagation?: () => void;
}

const KEYS: Record<string, GuideViewEvent> = {
  ArrowDown: { by: 1, type: "move" },
  ArrowLeft: { by: -1, type: "across" },
  ArrowRight: { by: 1, type: "across" },
  ArrowUp: { by: -1, type: "move" },
  Backspace: { type: "back" },
  Enter: { type: "choose" },
  Escape: { type: "back" },
};

/** A control with Enter and Space of its own: they act on it and never reach the Guide's cursor. */
const OwnKeys = ({ children }: { children: ReactNode }) => (
  <Layer
    inFlow
    onKeyDown={(event) => {
      const own = event as KeyEvent;
      if (own.key === "Enter" || own.key === " ") own.stopPropagation?.();
    }}
  >
    {children}
  </Layer>
);

const Ready = <C extends GuideViewEntry>({
  artworkUri = defaultArtwork,
  autoFocus = false,
  context,
  dayName,
  handle,
  layout,
  lowerThird,
  message,
  onEvent,
  picture,
  state,
}: GuideViewReady<C>) => {
  const { formatTime, lineup, nowMs } = context;
  const size = SIZES[layout];
  const phone = layout === "phone";
  const view = guideViewRows(state, context);
  const span = guideViewSpan(state, context);
  const look = guideViewLook(state, context);
  const watched = lineup.channels.find((channel) => channel.id === state.watchedId);
  const cursor = state.cursor;
  const programme = cursorProgramme(state, context);
  const cursorKey = cursor.kind === "tab" ? `tab:${cursor.group}` : `${cursor.channelId}/${programme?.id}`;

  // Focus follows the cursor only while the keys are in use, so a pointer never causes a scroll.
  const [keys, setKeys] = useState(autoFocus);
  const handles = useRef(new Map<string, RowHandle>());
  const hold = (key: string) => (node: RowHandle | null) => {
    if (node) handles.current.set(key, node);
    else handles.current.delete(key);
  };
  useEffect(() => {
    if (keys) handles.current.get(cursorKey)?.focus();
  }, [cursorKey, keys]);

  // A control that leaves the screen while focused drops focus to the body, where no key reaches the
  // Guide. It goes back to the cursor's block after a key, and to the Guide itself after a pointer.
  const root = useRef<Root | null>(null);
  const holding = useRef(false);
  const shown = `${view.notice ?? ""}|${view.rows.map((channel) => channel.id).join(",")}`;
  useEffect(() => {
    // A native client has no document, and its focus engine does this by itself.
    const page = (globalThis as { document?: { activeElement: unknown; body: unknown } }).document;
    if (!shown || !holding.current || !page) return;
    if (page.activeElement && page.activeElement !== page.body) return;
    // A compact block has a handle; the strip's cursor block is its one Tab stop.
    const block = keys
      ? (handles.current.get(cursorKey) ??
        root.current?.querySelector('[data-testid^="film-strip-"][tabindex="0"]'))
      : undefined;
    (block ?? root.current)?.focus();
  }, [shown, keys, cursorKey]);

  const onKeyDown = (event: GuideViewKey) => {
    const next = KEYS[event.key];
    if (!next || event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return;
    event.preventDefault();
    pointer.current = false;
    setKeys(true);
    onEvent(next);
  };
  useImperativeHandle(handle, () => ({ onKeyDown }));

  // OK and Enter arrive as a control's own press, so a press no pointer started is the keys'.
  const pointer = useRef(false);
  const fromKeys = () => {
    const keyed = !pointer.current;
    pointer.current = false;
    return keyed;
  };
  const press = (event: GuideViewEvent) => {
    // A control that is not the cursor's keeps its focus, unless it leaves the screen.
    setKeys(event.type === "showAll" && fromKeys());
    pointer.current = false;
    onEvent(event);
  };
  const pressBlock = (channelId: string, programmeId: string) => {
    const keyed = fromKeys();
    setKeys(keyed);
    onEvent({ channelId, keyed, programmeId, type: "press" });
  };

  const onTab = (event: ChannelListEvent) => {
    if (event.type === "focusTab" || event.type === "chooseTab") onEvent(event);
  };

  const tabs = (
    <OwnKeys>
      <ChannelGroupTabs
        cursor={cursor.kind === "tab" ? cursor : { channelId: cursor.channelId, kind: "row" }}
        group={state.group}
        onEvent={onTab}
        onMouseDown={(event) => event.preventDefault()}
        query={null}
        // The Guide has no find of its own.
        television
        tabRef={(group: LineupGroupKind) => hold(`tab:${group}`) as never}
      />
    </OwnKeys>
  );

  const pager =
    layout === "television" ? null : (
      <OwnKeys>
        <Stack direction="row" gap="1" justify="end">
          <Action
            disabled={!span.canEarlier}
            icon="earlier"
            onPress={() => press({ by: -1, type: "page" })}
            variant="ghost"
          >
            Earlier
          </Action>
          <Action
            disabled={!span.canLater}
            icon="later"
            iconPlacement="end"
            onPress={() => press({ by: 1, type: "page" })}
            variant="ghost"
          >
            Later
          </Action>
        </Stack>
      </OwnKeys>
    );

  const strip = (channel: GuideViewEntry) => {
    // A programme beyond each edge bounds a run that ends inside the span, so it keeps its count;
    // a run cut by an edge has nothing beyond it and says none.
    const within = channel.airings.filter((a) => a.stopMs > span.startMs && a.startMs < span.stopMs);
    const beside = (airing: StripAiring | undefined, from: StripAiring | undefined) =>
      airing && from && from.startMs >= span.startMs && from.stopMs <= span.stopMs ? [airing] : [];
    const first = within[0];
    const last = within.at(-1);
    const airings = [
      ...beside(channel.airings.filter((a) => first && a.stopMs <= first.startMs).at(-1), first),
      ...within,
      ...beside(
        channel.airings.find((a) => last && a.startMs >= last.stopMs),
        last,
      ),
    ];
    const items = groupSeriesRuns(airings).filter((item) => item.type === "programme");
    const mine = cursor.kind === "programme" && cursor.channelId === channel.id;
    // The strip names its blocks from the airings it holds, and a run it holds only part of is
    // shorter than the row's, so the two are matched by the time they share.
    const stripId =
      mine && programme
        ? items.find((item) => item.startMs < programme.stopMs && item.stopMs > programme.startMs)?.id
        : undefined;
    const blockOf = (startMs: number, stopMs: number) =>
      channel.programmes.find((p) => p.startMs < stopMs && startMs < p.stopMs);
    return (
      <FilmStrip
        airings={airings}
        artworkUri={artworkUri}
        blockContext={`channel ${channel.number}, ${channel.name}`}
        cursor={{ id: stripId, takeFocus: keys && mine }}
        formatTime={formatTime}
        layout="row"
        nowMs={nowMs}
        onSelect={(selected) => {
          const block = blockOf(selected.startMs, selected.stopMs);
          if (block) pressBlock(channel.id, block.id);
        }}
        window={{ startMs: span.startMs, stopMs: span.stopMs }}
      />
    );
  };

  const label = (channel: GuideViewEntry) => {
    const tuned = channel.id === state.watchedId;
    return (
      <OwnKeys>
        <RingedPressable
          // The name leads with the words on screen (WCAG 2.5.3).
          accessibilityLabel={`${channel.number} ${channel.name}, watch channel`}
          onPress={() => press({ channelId: channel.id, type: "pressChannel" })}
          // A row with nothing drawn has no block for the cursor, so its channel holds the focus.
          ref={hold(`${channel.id}/undefined`)}
          size={size.row}
          tabIndex={-1}
        >
          <Stack align="center" direction="row" gap="3" width={phone ? undefined : size.label - 24}>
            <Text
              textAlign="right"
              textRole={tuned && !phone ? "title" : "cardLabel"}
              tone={tuned ? decadeTint(channel.decade) : "secondary"}
              width={36}
            >
              {/* The space is content: without it the two read as one word to axe (WCAG 2.5.3). */}
              {`${channel.number} `}
            </Text>
            <Text flexShrink={1} minWidth={0} numberOfLines={1} textRole="cardLabel">
              {channel.name}
            </Text>
          </Stack>
        </RingedPressable>
      </OwnKeys>
    );
  };

  const factsAiring =
    phone && programme && programme.startMs > nowMs
      ? look.channel?.programmes.find((p) => p.id === programme.id)?.airings[0]
      : undefined;

  return (
    <Layer
      flex={1}
      inFlow
      onKeyDown={(event) => onKeyDown(event as GuideViewKey)}
      {...({
        onBlur: (event: { relatedTarget: unknown }) => {
          if (event.relatedTarget && !root.current?.contains(event.relatedTarget)) holding.current = false;
        },
        onFocus: () => {
          holding.current = true;
        },
        // Only watches: a block keeps OK and Enter to itself, and any key ends a pointer's press.
        onKeyDownCapture: () => {
          pointer.current = false;
        },
        onPointerDown: () => {
          pointer.current = true;
        },
        ref: root,
        // Focusable so that keys still reach the Guide after a pointer; it is no Tab stop.
        tabIndex: -1,
      } as object)}
    >
      <Stack
        aria-label="Guide"
        flex={1}
        gap={layout === "television" ? "1" : "2"}
        minHeight={0}
        paddingHorizontal={size.gutter}
        paddingVertical={layout === "television" ? 8 : size.gutter}
        role="region"
      >
        <Head
          formatTime={formatTime}
          layout={layout}
          look={look}
          lowerThird={lowerThird}
          nowMs={nowMs}
          picture={picture}
          tabs={layout === "television" ? tabs : undefined}
          watched={watched}
        />
        {layout === "television" ? null : (
          <Stack align={phone ? "stretch" : "center"} direction={phone ? "column" : "row"} justify="between">
            {tabs}
            {pager}
          </Stack>
        )}
        <Stack direction="row">
          {size.label ? <Surface backgroundColor="transparent" borderWidth={0} width={size.label} /> : null}
          <Stack flex={1}>
            <Ruler
              dayName={dayName}
              formatTime={formatTime}
              hourStartMs={context.hourStartMs}
              nowMs={nowMs}
              startMs={span.startMs}
              stopMs={span.stopMs}
            />
          </Stack>
        </Stack>
        <ScrollView showsVerticalScrollIndicator={false}>
          <Stack gap="1" paddingVertical={2}>
            {view.notice ? (
              <Stack align="start" gap="2">
                <Text textRole="body" tone="secondary">
                  {NOTICES[view.notice]}
                </Text>
                <OwnKeys>
                  <Action onPress={() => press({ type: "showAll" })} variant="outline">
                    Show all channels
                  </Action>
                </OwnKeys>
              </Stack>
            ) : null}
            {view.rows.map((channel) => {
              const decade = view.headings.get(channel.id);
              const mine = cursor.kind === "programme" && cursor.channelId === channel.id;
              return (
                <Stack gap="1" key={channel.id}>
                  {decade === undefined ? null : (
                    <Stack role="heading" {...({ "aria-level": 2 } as object)}>
                      <Text textRole="label" tone={decadeTint(decade)}>{`${decade}s`}</Text>
                    </Stack>
                  )}
                  <Stack
                    aria-label={`${channel.number} ${channel.name}`}
                    align={phone ? "stretch" : "center"}
                    direction={phone ? "column" : "row"}
                    role="group"
                  >
                    <Stack flexShrink={0} width={size.label || undefined}>
                      {label(channel)}
                    </Stack>
                    <Stack flex={phone ? 0 : 1} minWidth={0}>
                      {channel.id === state.watchedId &&
                      channel.programmes.some((p) => p.stopMs > span.startMs && p.startMs < span.stopMs) ? (
                        strip(channel)
                      ) : (
                        <Row
                          artworkUri={artworkUri}
                          blockRef={(id) => hold(`${channel.id}/${id}`)}
                          channel={channel}
                          cursorId={mine ? programme?.id : undefined}
                          formatTime={formatTime}
                          height={size.row}
                          nowMs={nowMs}
                          onPress={(programmeId) => pressBlock(channel.id, programmeId)}
                          startMs={span.startMs}
                          stopMs={span.stopMs}
                        />
                      )}
                    </Stack>
                  </Stack>
                </Stack>
              );
            })}
            {span.endsAtMs === undefined ? null : (
              <Text textRole="label" tone="secondary">
                {`The schedule ends at ${formatTime(span.endsAtMs)}`}
              </Text>
            )}
          </Stack>
        </ScrollView>
        {factsAiring ? (
          <ProgrammeFacts
            airing={factsAiring}
            formatTime={formatTime}
            nowMs={nowMs}
            startMs={programme?.startMs ?? factsAiring.startMs}
            stopMs={programme?.stopMs ?? factsAiring.stopMs}
          />
        ) : null}
        <StripMessage message={message} />
      </Stack>
    </Layer>
  );
};

/**
 * The Guide as a view of the watching screen (frontend-design §9.5, The Cross): every channel's day
 * on one clock, with the picture still playing. `guideViewReduce` decides what its keys and presses
 * do.
 */
const GuideView = <C extends GuideViewEntry>(props: GuideViewProps<C>) =>
  props.loading ? (
    <Stack gap="4" padding={SIZES[props.layout].gutter}>
      <Head
        formatTime={String}
        layout={props.layout}
        look={undefined}
        lowerThird={null}
        nowMs={0}
        picture={props.picture}
        watched={undefined}
      />
      <StatePanel
        description="Reading the latest channel schedule."
        kind="loading"
        title="Loading channels"
      />
    </Stack>
  ) : (
    <Ready {...props} />
  );

export { GuideView };
