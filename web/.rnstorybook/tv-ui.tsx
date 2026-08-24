import { brandChroma, semanticThemes } from "@loomarr/design-system";
import type { LiteUI } from "@storybook/react-native-ui-lite";
import { useState } from "react";
import { Pressable, ScrollView, StyleSheet, Text, View } from "react-native";

const TVStorybookUI: typeof LiteUI = ({ children, setStory, story, storyHash }) => {
  const stories = Object.values(storyHash).flatMap((entry) =>
    entry.type === "story" ? [{ id: entry.id, name: entry.name, title: entry.title }] : [],
  );
  const [focusedStoryId, setFocusedStoryId] = useState(story?.id ?? stories[0]?.id);

  return (
    <View style={styles.screen}>
      <View style={styles.rail}>
        <View accessibilityLabel="Loomarr design system" style={styles.brand}>
          <View style={styles.chroma}>
            {brandChroma.map((color) => (
              <View key={color} style={[styles.chromaSegment, { backgroundColor: color }]} />
            ))}
          </View>
          <Text style={styles.wordmark}>LOOMARR</Text>
          <Text style={styles.eyebrow}>NATIVE STORYBOOK</Text>
        </View>

        <ScrollView contentContainerStyle={styles.storyList}>
          {stories.map((entry) => {
            const focused = entry.id === focusedStoryId;
            const selected = entry.id === story?.id;
            return (
              <Pressable
                accessibilityLabel={`${entry.title}, ${entry.name}`}
                accessibilityRole="button"
                hasTVPreferredFocus={selected}
                key={entry.id}
                onFocus={() => setFocusedStoryId(entry.id)}
                onPress={() => setStory(entry.id)}
                style={[
                  styles.storyButton,
                  selected && styles.storyButtonSelected,
                  focused && styles.storyButtonFocused,
                ]}
              >
                <Text numberOfLines={1} style={styles.storyTitle}>
                  {entry.title.replace(/^Loomarr\//, "")}
                </Text>
                <Text numberOfLines={1} style={styles.storyName}>
                  {entry.name}
                </Text>
              </Pressable>
            );
          })}
        </ScrollView>

        <Text style={styles.hint}>D-pad to browse · Select to preview</Text>
      </View>

      <View accessibilityLabel="Story preview" style={styles.canvas}>
        {children}
      </View>
    </View>
  );
};

const colors = semanticThemes.dark;

const styles = StyleSheet.create({
  screen: {
    backgroundColor: colors.surface.canvas,
    flex: 1,
    flexDirection: "row",
  },
  rail: {
    backgroundColor: colors.surface.raised,
    borderRightColor: colors.border.decorative,
    borderRightWidth: 1,
    paddingBottom: 24,
    paddingHorizontal: 24,
    paddingTop: 28,
    width: 300,
  },
  brand: {
    marginBottom: 28,
  },
  chroma: {
    borderRadius: 3,
    flexDirection: "row",
    height: 8,
    marginBottom: 12,
    overflow: "hidden",
    width: 176,
  },
  chromaSegment: {
    flex: 1,
  },
  wordmark: {
    color: colors.content.primary,
    fontSize: 27,
    fontWeight: "700",
    letterSpacing: 3.2,
  },
  eyebrow: {
    color: colors.content.muted,
    fontFamily: "monospace",
    fontSize: 11,
    fontWeight: "600",
    letterSpacing: 1.5,
    marginTop: 5,
  },
  storyList: {
    gap: 10,
    paddingBottom: 12,
  },
  storyButton: {
    backgroundColor: colors.surface.elevated,
    borderColor: "transparent",
    borderRadius: 12,
    borderWidth: 3,
    minHeight: 68,
    paddingHorizontal: 16,
    paddingVertical: 10,
  },
  storyButtonSelected: {
    backgroundColor: colors.surface.focus,
  },
  storyButtonFocused: {
    borderColor: colors.action.focus,
  },
  storyTitle: {
    color: colors.content.secondary,
    fontSize: 14,
    fontWeight: "600",
  },
  storyName: {
    color: colors.content.primary,
    fontSize: 18,
    fontWeight: "600",
    marginTop: 2,
  },
  hint: {
    color: colors.content.muted,
    fontFamily: "monospace",
    fontSize: 11,
    marginTop: "auto",
  },
  canvas: {
    backgroundColor: colors.surface.canvas,
    flex: 1,
    overflow: "hidden",
  },
});

export { TVStorybookUI };
