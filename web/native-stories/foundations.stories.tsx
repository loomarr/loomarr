import {
  ActivityIndicator,
  BrandLockup,
  Screen,
  SignalLoader,
  Skeleton,
  Surface,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { ScrollView } from "react-native";

const Foundations = ({
  density = "touch",
  reducedMotion = false,
}: {
  density?: "touch" | "tv";
  reducedMotion?: boolean;
}) => (
  <Screen density={density}>
    <ScrollView contentContainerStyle={{ gap: density === "tv" ? 32 : 20 }}>
      <BrandLockup orientation="horizontal" showTagline size={density === "tv" ? "large" : "medium"} />
      <Surface gap="$section" padding={density === "tv" ? 32 : 20}>
        <Text density={density} textRole="title">
          Loading vocabulary
        </Text>
        <ActivityIndicator
          accessibilityLabel="Refreshing guide"
          reducedMotion={reducedMotion}
          size={density === "tv" ? "tv" : "control"}
        />
        <SignalLoader
          density={density}
          detail="Connecting to channel · 00:04"
          reducedMotion={reducedMotion}
        />
        <Skeleton reducedMotion={reducedMotion} shape="media" />
        <Skeleton reducedMotion={reducedMotion} width="72%" />
      </Surface>
    </ScrollView>
  </Screen>
);

const meta = {
  title: "Loomarr Foundations/Overview",
  component: Foundations,
  args: { density: "touch", reducedMotion: false },
} satisfies Meta<typeof Foundations>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const ReducedMotion: Story = { args: { reducedMotion: true } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, ReducedMotion, Touch, Tv };
