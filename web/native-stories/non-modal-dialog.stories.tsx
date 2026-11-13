import { NonModalDialog, playerChrome, Screen, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { View } from "react-native";

// A non-modal dialog sits over a picture that keeps playing: no scrim, no focus trap. The caller
// draws its surface; here it is the player's menu, from the chrome tokens.
const NativeNonModalDialogWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => (
  <Screen density={density}>
    <View style={{ backgroundColor: "#202833", flex: 1, justifyContent: "center", padding: 24 }}>
      <Text density={density} textRole="title">
        Playback keeps running
      </Text>
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
          width: density === "tv" ? 360 : 320,
        }}
      >
        <Text density={density} strong textRole="cardMeta" tone="secondary">
          Subtitles
        </Text>
        <Text density={density} textRole="body" tone="primary">
          Off
        </Text>
        <Text density={density} textRole="body" tone="primary">
          English
        </Text>
      </NonModalDialog>
    </View>
  </Screen>
);

const meta = {
  title: "Loomarr Components/Non-modal Dialog",
  component: NativeNonModalDialogWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeNonModalDialogWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
