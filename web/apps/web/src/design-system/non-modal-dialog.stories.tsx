import { LoomarrProvider, NonModalDialog, playerChrome, semanticThemes, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { View } from "react-native";

// A non-modal dialog sits in the page, over a picture that keeps playing: no scrim, no focus trap.
// The caller draws its surface; here it is the player's cluster menu, from the chrome tokens.
const NonModalDialogGallery = ({ theme = "dark" }: { theme?: "dark" | "light" }) => (
  <LoomarrProvider theme={theme}>
    <main
      style={{
        background: semanticThemes[theme].surface.canvas,
        boxSizing: "border-box",
        minHeight: "100vh",
        padding: 32,
      }}
    >
      <div
        style={{
          background: "#202833",
          color: "#F4F6FA",
          height: 320,
          overflow: "hidden",
          position: "relative",
        }}
      >
        <p style={{ margin: 24 }}>Playback keeps running</p>
        <NonModalDialog
          label="Subtitles"
          style={{
            backgroundColor: playerChrome.band,
            borderColor: playerChrome.hairline,
            borderRadius: 12,
            borderWidth: 1,
            bottom: 24,
            gap: 2,
            padding: 6,
            position: "absolute",
            right: 24,
            width: 280,
          }}
        >
          <View style={{ height: 32, justifyContent: "center", paddingHorizontal: 10 }}>
            <Text strong textRole="cardMeta" tone="secondary">
              Subtitles
            </Text>
          </View>
          <View style={{ minHeight: 44, justifyContent: "center", paddingHorizontal: 10 }}>
            <Text textRole="label" tone="primary">
              Off
            </Text>
          </View>
          <View style={{ minHeight: 44, justifyContent: "center", paddingHorizontal: 10 }}>
            <Text textRole="label" tone="primary">
              English
            </Text>
          </View>
        </NonModalDialog>
      </div>
    </main>
  </LoomarrProvider>
);

const meta = {
  title: "Loomarr Foundations/Non-modal Dialog",
  component: NonModalDialogGallery,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof NonModalDialogGallery>;

type Story = StoryObj<typeof meta>;
const Dark: Story = {};
const Light: Story = { args: { theme: "light" } };

export default meta;
export { Dark, Light };
