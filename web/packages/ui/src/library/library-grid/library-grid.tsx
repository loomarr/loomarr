import { useEffect } from "react";
import { View } from "react-native";

import type { LibraryGridProps } from "./library-grid.type";

// Phone and TV: every row, in the screen's own scroll, and each loaded page asks for the next.
// No native screen shows the Library yet; windowing needs this list to be the screen's scroller,
// with the shelf above it as its header, which that screen will decide. Web resolves
// library-grid.web.tsx, which mounts only the rows near the screen.
const LibraryGrid = <Row,>({ rows, rowKey, renderRow, onEndReached, label }: LibraryGridProps<Row>) => {
  useEffect(() => {
    if (rows.length > 0) onEndReached?.();
  }, [rows.length, onEndReached]);
  return (
    <View aria-label={label} role="list">
      {rows.map((row, index) => (
        <View key={rowKey(row, index)} role="none">
          {renderRow(row)}
        </View>
      ))}
    </View>
  );
};

export { LibraryGrid };
