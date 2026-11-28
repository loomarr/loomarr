import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import type { ChannelListEvent } from "@loomarr/core/channel-list";
import { Stack } from "@loomarr/design-system";
import { ChannelGroupTabs } from "@loomarr/ui/channels";
import { type FocusEvent, type KeyboardEvent, useRef, useState } from "react";

type Handle = { focus: () => void };

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
    <Stack data-testid="wall-tabs" overflow="auto" onBlur={onBlur} onKeyDown={onKeyDown}>
      <ChannelGroupTabs
        cursor={focused ? { group: focused, kind: "tab" } : { kind: "none" }}
        group={group}
        onEvent={onEvent}
        query={null}
        // The wall has no find. `television` is the tabs' one way to leave its control out.
        television
        tabRef={(kind) => (node) => {
          if (node) tabs.current.set(kind, node);
          else tabs.current.delete(kind);
        }}
      />
    </Stack>
  );
};

export type { WallTabsProps };
export { WallTabs };
