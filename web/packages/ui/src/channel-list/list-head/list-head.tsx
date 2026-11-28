import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import type { ChannelListCursor, ChannelListEvent } from "@loomarr/core/channel-list";
import { Icon, icons, PressableRow, Surface, Text } from "@loomarr/design-system";
import { SearchField } from "@loomarr/design-system/fields";

import type { FocusHandle } from "../channel-list.type";

const TAB_NAMES: Record<LineupGroupKind, string> = {
  all: "All",
  decade: "By decade",
  favourites: "Favourites",
  recent: "Recent",
};
const TABS: readonly LineupGroupKind[] = ["favourites", "all", "recent", "decade"];
// Room enough that a tab or find's controls are a finger's height.
const INSET = { x: 8, y: 12 };
const FIND_LABEL = "Find a channel or a programme";

interface ListHeadProps {
  cursor: ChannelListCursor;
  group: LineupGroupKind;
  query: string | null;
  television: boolean;
  onEvent: (event: ChannelListEvent) => void;
  onMouseDown: (event: { preventDefault: () => void }) => void;
  tabRef: (group: LineupGroupKind) => (node: FocusHandle | null) => void;
}

/** The group tabs and find's control, or the open query in their place. */
const ListHead = ({ cursor, group, query, television, onEvent, onMouseDown, tabRef }: ListHeadProps) => {
  if (query !== null) {
    return (
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap={8} alignItems="center">
        <Surface backgroundColor="transparent" borderWidth={0} flex={1}>
          <SearchField
            autoFocus
            label={FIND_LABEL}
            labelHidden
            onChangeText={(text) => onEvent({ query: text, type: "find" })}
            placeholder={`${FIND_LABEL}…`}
            value={query}
          />
        </Surface>
        <Surface backgroundColor="transparent" borderWidth={0}>
          <PressableRow
            accessibilityLabel="Close find"
            inset={INSET}
            onPress={() => onEvent({ type: "clearFind" })}
          >
            <Icon decorative glyph={icons.close} size="compact" tone="muted" />
          </PressableRow>
        </Surface>
      </Surface>
    );
  }
  return (
    <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" alignItems="center" gap={4}>
      {television ? null : (
        <Surface backgroundColor="transparent" borderWidth={0}>
          <PressableRow
            accessibilityLabel="Find a channel"
            inset={INSET}
            onPress={() => onEvent({ query: "", type: "find" })}
          >
            <Icon decorative glyph={icons.search} size="compact" tone="muted" />
          </PressableRow>
        </Surface>
      )}
      <Surface
        aria-label="Channel groups"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap={4}
        role="tablist"
      >
        {TABS.map((kind) => {
          const tabStop = cursor.kind === "tab" ? cursor.group === kind : group === kind;
          return (
            <Surface backgroundColor="transparent" borderWidth={0} key={kind}>
              <PressableRow
                accessibilityLabel={TAB_NAMES[kind]}
                inset={INSET}
                onFocus={() => onEvent({ group: kind, type: "focusTab" })}
                onMouseDown={onMouseDown}
                onPress={() => onEvent({ group: kind, type: "chooseTab" })}
                ref={tabRef(kind) as never}
                {...({ "aria-selected": group === kind, role: "tab", tabIndex: tabStop ? 0 : -1 } as object)}
              >
                <Surface
                  backgroundColor="transparent"
                  borderBottomColor={group === kind ? "contentPrimary" : "transparent"}
                  borderBottomWidth={1}
                  borderRadius={0}
                  borderWidth={0}
                >
                  <Text textRole="cardLabel" tone={group === kind ? "primary" : "secondary"}>
                    {TAB_NAMES[kind]}
                  </Text>
                </Surface>
              </PressableRow>
            </Surface>
          );
        })}
      </Surface>
    </Surface>
  );
};

export { ListHead };
