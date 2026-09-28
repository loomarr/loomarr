import { FlatList } from "react-native";

import type { GuideRowsProps } from "./guide-rows.type";

// Phone and TV: FlatList, with the ruler as its sticky header. Web resolves guide-rows.web.tsx,
// which adds the keyboard (focusIndex, onKey).
const GuideRows = ({ channels, header, headerHeight, renderRow, rowHeight }: GuideRowsProps) => (
  <FlatList
    data={channels}
    getItemLayout={(_, index) => ({ index, length: rowHeight, offset: headerHeight + rowHeight * index })}
    keyExtractor={(channel) => channel.source.channelId}
    ListHeaderComponent={header}
    renderItem={({ item }) => renderRow(item)}
    stickyHeaderIndices={[0]}
    style={{ minWidth: 900 }}
  />
);

export { GuideRows };
