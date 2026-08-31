import { Screen } from "@loomarr/design-system";
import { GuideExperience, type GuideUnavailableState } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

const GuideStateWorkshop = ({
  density = "pointer",
  state,
}: {
  density?: "pointer" | "touch" | "tv";
  state: GuideUnavailableState;
}) => (
  <Screen density={density} justifyContent="center">
    <GuideExperience density={density} onRetry={() => undefined} state={state} />
  </Screen>
);

const meta = {
  title: "Loomarr Components/Guide Experience",
  component: GuideStateWorkshop,
  args: { density: "pointer", state: "loading" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof GuideStateWorkshop>;

type Story = StoryObj<typeof meta>;
const Loading: Story = {};
const Empty: Story = { args: { state: "empty" } };
const ErrorState: Story = { args: { state: "error" } };
const Offline: Story = { args: { state: "offline" } };
const TouchLoading: Story = { args: { density: "touch" } };
const TvError: Story = { args: { density: "tv", state: "error" } };
const LightEmpty: Story = { args: { state: "empty" }, globals: { theme: "light" } };

export default meta;
export { Empty, ErrorState, LightEmpty, Loading, Offline, TouchLoading, TvError };
