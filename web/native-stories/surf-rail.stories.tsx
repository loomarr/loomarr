import { surfGroups } from "@loomarr/fixtures";
import { SurfRail, type SurfSelection } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const SurfWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [selection, setSelection] = useState<SurfSelection>({
    channelId: "ch-springfield",
    group: "recent",
  });
  // The TV rail's channel-number wait row (#1659 decision N4); the TV app passes the ui-tv choices.
  const [autoTuneMs, setAutoTuneMs] = useState(1_200);
  return (
    <SurfRail
      autoTune={
        density === "tv"
          ? { choicesMs: [1_200, 2_000, 3_000, 5_000, 10_000], onChange: setAutoTuneMs, valueMs: autoTuneMs }
          : undefined
      }
      clientVersion="0.2.0"
      density={density}
      groups={surfGroups}
      onFocusSelection={setSelection}
      onTune={() => undefined}
      selection={selection}
      serverVersion="0.2.1"
    />
  );
};

const meta = {
  title: "Loomarr Components/Surf Rail",
  component: SurfWorkshop,
} satisfies Meta<typeof SurfWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
