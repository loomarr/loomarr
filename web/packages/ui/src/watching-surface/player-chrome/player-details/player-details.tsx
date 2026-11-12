import { type Density, Icon, icons, playerChrome, Text, useFocusLook } from "@loomarr/design-system";
import { useEffect, useRef, useState } from "react";
import { Pressable, View } from "react-native";

import { nativeFocus, raised, webOnly } from "../flat";

// Playback details (§9.5): the one place technical terms are allowed. Mono values for what the player
// really knows, one row each, and Copy details for a bug report. A row the player has no figure for
// is not drawn: nothing here is invented. On a pointer it is a panel at the upper left; on a phone
// the same body sits in a sheet.

interface PlayerDetailRow {
  label: string;
  value: string;
}

interface PlayerDetailsProps {
  density: Density;
  /** Copies the rows as text (the web writes the clipboard); omitted, there is no Copy key. */
  onCopy?: (text: string) => void;
  rows: readonly PlayerDetailRow[];
}

const COPIED_MS = 1_500;

const CopyKey = ({ density, onPress }: { density: Density; onPress: () => void }) => {
  const [copied, setCopied] = useState(false);
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  return (
    <Pressable
      accessibilityLabel="Copy details"
      accessibilityRole="button"
      {...focusHandlers}
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      onPress={() => {
        onPress();
        setCopied(true);
        clearTimeout(timer.current);
        timer.current = setTimeout(() => setCopied(false), COPIED_MS);
      }}
      style={{
        alignItems: "center",
        alignSelf: "flex-start",
        borderRadius: 6,
        flexDirection: "row",
        gap: 8,
        height: 40,
        paddingHorizontal: 16,
        ...raised(),
        ...(hovered ? { backgroundColor: playerChrome.surfaceHover } : null),
        ...webOnly({
          outlineColor: playerChrome.signal,
          outlineOffset: 2,
          outlineStyle: focused ? "solid" : "none",
          outlineWidth: 2,
          transition: "background-color 120ms ease",
        }),
        ...nativeFocus(focused),
      }}
    >
      <Icon color={playerChrome.text} decorative glyph={copied ? icons.success : icons.copy} size="compact" />
      <Text density={density} strong textRole="label" tone="primary">
        {copied ? "Copied" : "Copy details"}
      </Text>
    </Pressable>
  );
};

const PlayerDetails = ({ density, onCopy, rows }: PlayerDetailsProps) => (
  <View style={{ gap: 16 }}>
    <View role="list" style={{ gap: 8 }}>
      {rows.map((row) => (
        <View
          key={row.label}
          role="listitem"
          style={{ flexDirection: "row", gap: 24, justifyContent: "space-between" }}
        >
          <Text density={density} textRole="compact" tone="muted">
            {row.label}
          </Text>
          <Text
            density={density}
            style={{ flexShrink: 1, textAlign: "right" }}
            textRole="mono"
            tone="primary"
          >
            {row.value}
          </Text>
        </View>
      ))}
    </View>
    {onCopy ? (
      <CopyKey
        density={density}
        onPress={() => onCopy(rows.map((r) => `${r.label}: ${r.value}`).join("\n"))}
      />
    ) : null}
  </View>
);

/** The pointer's form: a panel at the upper left, over the picture, beside nothing else. */
const PlayerDetailsPanel = ({ onClose, ...body }: PlayerDetailsProps & { onClose: () => void }) => {
  const [hovered, setHovered] = useState(false);
  const { focused, ...focusHandlers } = useFocusLook();
  return (
    <View
      aria-label="Playback details"
      role="dialog"
      style={{
        backgroundColor: playerChrome.surface,
        borderColor: playerChrome.hairline,
        borderRadius: 6,
        borderWidth: 1,
        left: 48,
        padding: 24,
        position: "absolute",
        top: 112,
        width: 424,
        zIndex: 35,
      }}
    >
      <View style={{ alignItems: "center", flexDirection: "row", gap: 8, marginBottom: 16 }}>
        <View style={{ backgroundColor: playerChrome.text, borderRadius: 4, height: 8, width: 8 }} />
        <Text density={body.density} strong style={{ flex: 1 }} textRole="label" tone="primary">
          Playback details
        </Text>
        <Pressable
          accessibilityLabel="Close Playback details"
          accessibilityRole="button"
          {...focusHandlers}
          onHoverIn={() => setHovered(true)}
          onHoverOut={() => setHovered(false)}
          onPress={onClose}
          style={{
            alignItems: "center",
            backgroundColor: hovered ? playerChrome.surfaceHover : "transparent",
            borderRadius: 6,
            height: 32,
            justifyContent: "center",
            width: 32,
            ...webOnly({
              outlineColor: playerChrome.signal,
              outlineStyle: focused ? "solid" : "none",
              outlineWidth: 2,
              transition: "background-color 120ms ease",
            }),
            ...nativeFocus(focused),
          }}
        >
          <Icon color={playerChrome.text} decorative glyph={icons.close} size="compact" />
        </Pressable>
      </View>
      <PlayerDetails {...body} />
    </View>
  );
};

export type { PlayerDetailRow, PlayerDetailsProps };
export { PlayerDetails, PlayerDetailsPanel };
