import {
  Action,
  ArtworkFrame,
  BottomSheet,
  LoomarrProvider,
  semanticThemes,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { View } from "react-native";

// The two ways a phone holds a selected programme (#1659 native mock): iPhone's sheet with a
// grabber (5d) and Android's docked strip (5f). The content is the story's; the sheet is the part.
const SheetGallery = ({ theme = "dark" }: { theme?: "dark" | "light" }) => {
  const colors = semanticThemes[theme];
  return (
    <LoomarrProvider theme={theme}>
      <main
        style={{
          background: colors.surface.canvas,
          boxSizing: "border-box",
          display: "grid",
          gap: 32,
          gridTemplateColumns: "repeat(auto-fit, 390px)",
          minHeight: "100vh",
          padding: 32,
        }}
      >
        <section aria-label="iPhone sheet" style={{ alignSelf: "end" }}>
          <BottomSheet accessibilityLabel="Selected programme" onDismiss={() => undefined}>
            <View style={{ flexDirection: "row", gap: 12 }}>
              <ArtworkFrame borderRadius={8} density="touch" state="missing" width={112} />
              <View style={{ flex: 1, gap: 3 }}>
                <Text density="touch" textRole="caption" tone="secondary">
                  21 · Nature Documentaries
                </Text>
                <Text density="touch" textRole="label" tone="primary">
                  Deep Water
                </Text>
                <Text density="touch" textRole="metadata">
                  8:50–9:39 PM · in 24m · TV-G
                </Text>
              </View>
            </View>
            <Action density="touch">Watch 21 now</Action>
          </BottomSheet>
        </section>
        <section aria-label="Android docked strip" style={{ alignSelf: "end" }}>
          <BottomSheet accessibilityLabel="Selected programme" handle={false}>
            <View style={{ alignItems: "center", flexDirection: "row", gap: 12 }}>
              <ArtworkFrame borderRadius={4} density="touch" state="missing" width={96} />
              <View style={{ flex: 1, gap: 2 }}>
                <Text density="touch" textRole="caption" tone="secondary">
                  21 · 8:50–9:39 PM
                </Text>
                <Text density="touch" numberOfLines={1} textRole="label" tone="primary">
                  Deep Water
                </Text>
              </View>
              <Action density="touch">Watch</Action>
            </View>
          </BottomSheet>
        </section>
      </main>
    </LoomarrProvider>
  );
};

const meta = {
  title: "Loomarr Foundations/Bottom Sheet",
  component: SheetGallery,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof SheetGallery>;

type Story = StoryObj<typeof meta>;
const Dark: Story = {};
const Light: Story = { args: { theme: "light" } };

export default meta;
export { Dark, Light };
