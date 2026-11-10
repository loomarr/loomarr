import { Platform } from "react-native";

const row = { alignItems: "center", flexDirection: "row", gap: 8 } as const;
const thumb = {
  alignItems: "center",
  borderRadius: 4,
  height: 44,
  justifyContent: "center",
  overflow: "hidden",
  // The mock's #2a3140 → #14171d scene backdrop; the bugs are drawn to be read on a dark frame.
  ...(Platform.OS === "web"
    ? ({ backgroundImage: "linear-gradient(135deg, #2a3140, #14171d)" } as object)
    : { backgroundColor: "#1c212a" }),
} as const;
// A radio group laid out as a wrapping row; Surface carries the role (Text drops it on web).
const group = { borderWidth: 0, flexDirection: "row", flexWrap: "wrap", gap: 8 } as const;

export { group, row, thumb };
