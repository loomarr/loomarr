import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import type { ChannelListCursor, ChannelListEvent } from "@loomarr/core/channel-list";
import { Icon, icons, RingedPressable, Surface, Text } from "@loomarr/design-system";

import { SearchField } from "../../search-field";
import type { FocusHandle } from "../channel-list.type";

const TAB_NAMES: Record<LineupGroupKind, string> = {
  all: "All",
  decade: "By decade",
  favourites: "Favourites",
  recent: "Recent",
};
const TABS: readonly LineupGroupKind[] = ["favourites", "all", "recent", "decade"];
// Room enough that a tab or find's controls are a finger's height.
const TARGET = 44;
const FIND_LABEL = "Find a channel or a programme";

interface ListHeadProps {
  cursor: ChannelListCursor;
  group: LineupGroupKind;
  query: string | null;
  television: boolean;
  onEvent: (event: ChannelListEvent) => void;
  tabRef: (group: LineupGroupKind) => (node: FocusHandle | null) => void;
}

/** The group tabs and find's control, or the open query in their place. */
// A pointer never takes focus in the head, as in the list, so the amber ring is the keys' alone.
const ListHead = ({ cursor, group, query, television, onEvent, tabRef }: ListHeadProps) => {
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
          <RingedPressable
            accessibilityLabel="Close find"
            pointerTakesNoFocus
            onPress={() => onEvent({ type: "clearFind" })}
            size={TARGET}
          >
            <Icon decorative glyph={icons.close} size="compact" tone="muted" />
          </RingedPressable>
        </Surface>
      </Surface>
    );
  }
  return (
    <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" alignItems="center" gap={4}>
      {television ? null : (
        <Surface backgroundColor="transparent" borderWidth={0}>
          <RingedPressable
            accessibilityLabel="Find a channel"
            pointerTakesNoFocus
            onPress={() => onEvent({ query: "", type: "find" })}
            size={TARGET}
          >
            <Icon decorative glyph={icons.search} size="compact" tone="muted" />
          </RingedPressable>
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
              <RingedPressable
                accessibilityLabel={TAB_NAMES[kind]}
                onFocus={() => onEvent({ group: kind, type: "focusTab" })}
                pointerTakesNoFocus
                onPress={() => onEvent({ group: kind, type: "chooseTab" })}
                ref={tabRef(kind) as never}
                role="tab"
                selected={group === kind}
                size={TARGET}
                tabIndex={tabStop ? 0 : -1}
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
              </RingedPressable>
            </Surface>
          );
        })}
      </Surface>
    </Surface>
  );
};

export { ListHead };
