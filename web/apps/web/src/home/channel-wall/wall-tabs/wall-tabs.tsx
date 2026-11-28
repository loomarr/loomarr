import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import type { ChannelListEvent } from "@loomarr/core/channel-list";
import { Stack } from "@loomarr/design-system";
import { ChannelGroupTabs } from "@loomarr/ui";
import {
  type ComponentProps,
  type ComponentType,
  type FocusEvent,
  type KeyboardEvent,
  useRef,
  useState,
} from "react";

type Handle = { focus: () => void };

// The wall passes the tabs no pointer handler: the keyboard ring is theirs to draw.
const GroupTabs = ChannelGroupTabs as ComponentType<
  Omit<ComponentProps<typeof ChannelGroupTabs>, "onMouseDown">
>;

interface WallTabsProps {
  /** The four groups in the order the tabs show them. */
  kinds: readonly LineupGroupKind[];
  group: LineupGroupKind;
  onChoose: (group: LineupGroupKind) => void;
}

// The channel list's own tabs, with the keys the list gives them: one tab stop, Left and Right move
// between tabs and stop at the ends, and Enter or a press chooses.
const WallTabs = ({ kinds, group, onChoose }: WallTabsProps) => {
  const [focused, setFocused] = useState<LineupGroupKind | undefined>(undefined);
  const tabs = useRef(new Map<LineupGroupKind, Handle>());

  const onEvent = (event: ChannelListEvent) => {
    if (event.type === "focusTab") setFocused(event.group);
    if (event.type === "chooseTab") onChoose(event.group);
  };
  const onKeyDown = (event: KeyboardEvent) => {
    const by = event.key === "ArrowLeft" ? -1 : event.key === "ArrowRight" ? 1 : 0;
    if (!by) return;
    event.preventDefault();
    const next = kinds[kinds.indexOf(focused ?? group) + by];
    if (next) tabs.current.get(next)?.focus();
  };
  // Leaving the tabs puts the tab stop back on the chosen group.
  const onBlur = (event: FocusEvent) => {
    if (!event.currentTarget.contains(event.relatedTarget)) setFocused(undefined);
  };

  return (
    // The tabs scroll sideways where they do not fit, and never wrap.
    <Stack overflow="auto" {...({ onBlur, onKeyDown } as object)}>
      <GroupTabs
        cursor={focused ? { group: focused, kind: "tab" } : { kind: "none" }}
        group={group}
        onEvent={onEvent}
        query={null}
        // The wall has no find of its own.
        television
        tabRef={
          ((kind: LineupGroupKind) => (node: Handle | null) => {
            if (node) tabs.current.set(kind, node);
            else tabs.current.delete(kind);
          }) as ComponentProps<typeof ChannelGroupTabs>["tabRef"]
        }
      />
    </Stack>
  );
};

export type { WallTabsProps };
export { WallTabs };
