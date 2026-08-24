import {
  Action,
  type Density,
  Hint,
  MenuList,
  Screen,
  SelectControl,
  Surface,
  Tabs,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const SelectionWorkshop = ({
  density = "pointer",
  initialOpen = false,
}: {
  density?: Density;
  initialOpen?: boolean;
}) => {
  const [section, setSection] = useState<"guide" | "surf" | "watching">("guide");
  const [theme, setTheme] = useState<"dark" | "light" | "system">("dark");
  const [open, setOpen] = useState(initialOpen);
  const [lastAction, setLastAction] = useState("No menu action");
  return (
    <Screen density={density} gap="$section">
      <Surface gap="$control" padding="$section">
        <Text density={density} textRole="title">
          Tabs
        </Text>
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
      </Surface>

      <Surface gap="$control" padding="$section">
        <Text density={density} textRole="title">
          Selection and menu
        </Text>
        <SelectControl
          density={density}
          label="Appearance"
          onOpenChange={setOpen}
          onValueChange={setTheme}
          open={open}
          options={[
            { description: "Loomarr's default.", label: "Dark", value: "dark" },
            { label: "Light", value: "light" },
            { label: "Follow system", value: "system" },
          ]}
          value={theme}
        />
        <MenuList
          density={density}
          items={[
            { label: "Refresh guide", value: "refresh" },
            { disabled: true, label: "Favourite channel", value: "favourite" },
            { label: "Disconnect device", tone: "danger", value: "disconnect" },
          ]}
          label="Viewer actions"
          onSelect={setLastAction}
        />
        <Text aria-live="polite" density={density} textRole="metadata">
          {lastAction}
        </Text>
      </Surface>

      <Surface gap="$control" padding="$section">
        <Text density={density} textRole="title">
          Adapter-owned hint
        </Text>
        <Hint content="Returns the time-shifted player to the live edge." density={density} visible>
          <Action density={density} onPress={() => undefined} tone="secondary">
            Go live
          </Action>
        </Hint>
      </Surface>
    </Screen>
  );
};

const meta = {
  title: "Loomarr Foundations/Selection",
  component: SelectionWorkshop,
  args: { density: "pointer", initialOpen: false },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof SelectionWorkshop>;

type Story = StoryObj<typeof meta>;
const Pointer: Story = {};
const OpenSelect: Story = { args: { initialOpen: true } };
const Touch: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };
const FocusedMenu: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("menuitem", { name: "Refresh guide" }).focus();
  },
};

export default meta;
export { FocusedMenu, Light, OpenSelect, Pointer, Touch, Tv };
