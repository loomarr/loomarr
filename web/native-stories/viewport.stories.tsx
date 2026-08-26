import { LoomarrProvider, Screen, Surface, Text, type ViewportInsets } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";

const NativeViewport = ({ density, insets }: { density: "touch" | "tv"; insets?: ViewportInsets }) => (
  <LoomarrProvider insets={insets}>
    <Screen density={density}>
      <Surface flex={1} gap="$control" justifyContent="center" level="focus" padding="$section">
        <Text density={density} textAlign="center" textRole="title">
          Safe content frame
        </Text>
        <Text density={density} textAlign="center" textRole="body">
          Content stays inside the platform inset and Loomarr gutter.
        </Text>
      </Surface>
    </Screen>
  </LoomarrProvider>
);

const meta = {
  title: "Loomarr Foundations/Viewport and Safe Areas",
  component: NativeViewport,
  args: { density: "touch" },
} satisfies Meta<typeof NativeViewport>;

type Story = StoryObj<typeof meta>;
const PhoneNotch: Story = {
  args: { insets: { bottom: 34, left: 0, right: 0, top: 47 } },
};
const TvOverscan: Story = { args: { density: "tv" } };
const LightPhoneNotch: Story = {
  args: { insets: { bottom: 34, left: 0, right: 0, top: 47 } },
  globals: { theme: "light" },
};

export default meta;
export { LightPhoneNotch, PhoneNotch, TvOverscan };
