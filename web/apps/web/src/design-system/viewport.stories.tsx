import { LoomarrProvider, Screen, Surface, Text, type ViewportInsets } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

const ViewportContract = ({
  density,
  insets,
}: {
  density: "pointer" | "touch" | "tv";
  insets?: ViewportInsets;
}) => (
  <LoomarrProvider insets={insets}>
    <Screen density={density}>
      <Surface flex={1} gap="$control" justifyContent="center" level="focus" padding="$section">
        <Text density={density} textAlign="center" textRole="title">
          Safe content frame
        </Text>
        <Text density={density} textAlign="center" textRole="body">
          The canvas reaches every edge. Content stays inside the platform inset and Loomarr gutter.
        </Text>
      </Surface>
    </Screen>
  </LoomarrProvider>
);

const meta = {
  title: "Loomarr Foundations/Viewport and Safe Areas",
  component: ViewportContract,
  args: { density: "pointer" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ViewportContract>;

type Story = StoryObj<typeof meta>;
const Desktop: Story = {};
const PhoneNotch: Story = {
  args: { density: "touch", insets: { bottom: 34, left: 0, right: 0, top: 47 } },
};
const TvOverscan1080p: Story = { args: { density: "tv" } };
const TvOverscan4k: Story = { args: { density: "tv" } };

export default meta;
export { Desktop, PhoneNotch, TvOverscan4k, TvOverscan1080p };
