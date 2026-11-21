import { librarySections } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { BrowserTabs } from "../client-adapters";

type Section = (typeof librarySections)[number]["value"];

// Tabs, underline appearance: a page's sections as words on a shared rule, the selected one
// underlined in the foreground colour, a count beside each label. Arrow keys, Home and End move
// between tabs through the browser adapter, as for the default appearance.
const UnderlineTabs = ({ counts = true, initial = "all" }: { counts?: boolean; initial?: Section }) => {
  const [value, setValue] = useState<Section>(initial);
  const options = counts ? librarySections : librarySections.map(({ label, value }) => ({ label, value }));
  return (
    <BrowserTabs
      appearance="underline"
      label="Library sections"
      onValueChange={setValue}
      options={options}
      value={value}
    />
  );
};

const meta = {
  title: "Loomarr Foundations/Tabs underline",
  component: UnderlineTabs,
  args: { counts: true, initial: "all" },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof UnderlineTabs>;

type Story = StoryObj<typeof meta>;

const WithCounts: Story = {};
const WithoutCounts: Story = { args: { counts: false } };
const LaterTabSelected: Story = { args: { initial: "shows" } };
const Light: Story = { globals: { theme: "light" } };
const Focused: Story = {
  play: async ({ canvas }) => {
    canvas.getByRole("tab", { name: /All/ }).focus();
  },
};
// A phone: the four sections still fit one line at 390.
const Phone: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 390 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
export { Focused, LaterTabSelected, Light, Phone, WithCounts, WithoutCounts };
