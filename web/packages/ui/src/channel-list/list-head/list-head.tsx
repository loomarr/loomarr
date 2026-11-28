import type { ChannelListCursor, ChannelListEvent } from "@loomarr/core/channel-list";
import type { LineupGroupKind } from "@loomarr/core/channel-lineup";
import { Icon, icons, Surface, Text } from "@loomarr/design-system";
import { View } from "react-native";

import { SearchField } from "../../search-field";
import { ListTarget, type TargetHandle } from "../list-target";

const TAB_NAMES: Record<LineupGroupKind, string> = {
  all: "All",
  decade: "By decade",
  favourites: "Favourites",
  recent: "Recent",
};
const TABS: readonly LineupGroupKind[] = ["favourites", "all", "recent", "decade"];
const TAB_HEIGHT = 44;
const FIND_LABEL = "Find a channel or a programme";

interface ListHeadProps {
  cursor: ChannelListCursor;
  group: LineupGroupKind;
  query: string | null;
  television: boolean;
  /** Whether the cursor draws its ring. */
  ring: boolean;
  onEvent: (event: ChannelListEvent) => void;
  tabRef: (group: LineupGroupKind) => (node: TargetHandle | null) => void;
}

/** The group tabs and find's control, or the open query in their place. */
const ListHead = ({ cursor, group, query, television, ring, onEvent, tabRef }: ListHeadProps) => {
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
        <ListTarget
          label="Close find"
          minHeight={TAB_HEIGHT}
          onPress={() => onEvent({ type: "clearFind" })}
          ringed={false}
          role="button"
          tabStop
        >
          <Icon decorative glyph={icons.close} size="compact" tone="muted" />
        </ListTarget>
      </Surface>
    );
  }
  return (
    <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" alignItems="center" gap={4}>
      {television ? null : (
        <ListTarget
          label="Find a channel"
          minHeight={TAB_HEIGHT}
          onPress={() => onEvent({ query: "", type: "find" })}
          ringed={false}
          role="button"
          tabStop
        >
          <Icon decorative glyph={icons.search} size="compact" tone="muted" />
        </ListTarget>
      )}
      <View aria-label="Channel groups" role="tablist" style={{ flexDirection: "row", gap: 4 }}>
        {TABS.map((kind) => (
          <ListTarget
            key={kind}
            label={TAB_NAMES[kind]}
            minHeight={TAB_HEIGHT}
            onKeyFocus={() => onEvent({ group: kind, type: "focusTab" })}
            onPress={() => onEvent({ group: kind, type: "chooseTab" })}
            ringed={ring && cursor.kind === "tab" && cursor.group === kind}
            role="tab"
            selected={group === kind}
            tabStop={cursor.kind === "tab" ? cursor.group === kind : group === kind}
            targetRef={tabRef(kind)}
          >
            <Surface
              backgroundColor="transparent"
              borderWidth={0}
              borderBottomColor={group === kind ? "contentPrimary" : "transparent"}
              borderBottomWidth={1}
            >
              <Text textRole="cardLabel" tone={group === kind ? "primary" : "secondary"}>
                {TAB_NAMES[kind]}
              </Text>
            </Surface>
          </ListTarget>
        ))}
      </View>
    </Surface>
  );
};

export { ListHead };
