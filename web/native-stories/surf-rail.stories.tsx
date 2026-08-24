import { surfGroups } from "@loomarr/fixtures";
import { SurfRail, type SurfSelection } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const SurfWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [selection, setSelection] = useState<SurfSelection>({
    channelId: "ch-springfield",
    group: "recent",
  });
  return (
    <SurfRail
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
  title: "Loomarr/Surf Rail",
  component: SurfWorkshop,
} satisfies Meta<typeof SurfWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };

export default meta;
export { Touch, Tv };
