import { FlatList } from "react-native";

import type { LibraryGridProps } from "./library-grid.type";

// Phone and TV: FlatList windows the rows and asks for more near the end. Web resolves
// library-grid.web.tsx.
const LibraryGrid = <Row,>({ rows, rowKey, renderRow, onEndReached, label }: LibraryGridProps<Row>) => (
  <FlatList
    accessibilityLabel={label}
    data={rows}
    keyExtractor={rowKey}
    onEndReached={onEndReached}
    onEndReachedThreshold={1}
    renderItem={({ item }) => renderRow(item)}
    role="list"
  />
);

export { LibraryGrid };
