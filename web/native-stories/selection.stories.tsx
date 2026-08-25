import { Hint, MenuList, SelectControl, Tabs, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const SelectionWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [section, setSection] = useState("guide");
  const [theme, setTheme] = useState("dark");
  const [open, setOpen] = useState(true);
  return (
    <>
      <Tabs
        density={density}
        label="Viewer sections"
        onValueChange={setSection}
        options={[
          { label: "Watching", value: "watching" },
          { label: "Guide", value: "guide" },
          { label: "Surf", value: "surf" },
        ]}
        value={section}
      />
      <SelectControl
        density={density}
        label="Appearance"
        onOpenChange={setOpen}
        onValueChange={setTheme}
        open={open}
        options={[
          { label: "Dark", value: "dark" },
          { label: "Light", value: "light" },
        ]}
        value={theme}
      />
      <MenuList
        density={density}
        items={[
          { label: "Refresh guide", value: "refresh" },
          { label: "Disconnect device", tone: "danger", value: "disconnect" },
        ]}
        label="Viewer actions"
        onSelect={() => undefined}
      />
      <Hint content="Returns to live playback." density={density} visible>
        <Text density={density} textRole="label">
          Go live
        </Text>
      </Hint>
    </>
  );
};

const meta = {
  title: "Loomarr/Selection",
  component: SelectionWorkshop,
} satisfies Meta<typeof SelectionWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
