import type { Density } from "@loomarr/design-system";
import { librarySections, librarySectionsLong } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { BrowserTabs } from "../client-adapters";

type Section = (typeof librarySections)[number]["value"];

// Tabs, underline appearance: a page's sections as words on a shared rule, the selected one
// underlined in the foreground colour, a count beside each label (spoken as "3,412 titles"). Arrow
// keys, Home and End move between tabs through the browser adapter, as for the default appearance;
// when the tabs do not fit, the list scrolls sideways and a focused tab scrolls into view.
const UnderlineTabs = ({
  counts = true,
  density = "pointer",
  initial = "all",
  long = false,
}: {
  counts?: boolean;
  density?: Density;
  initial?: Section;
  long?: boolean;
}) => {
  const [value, setValue] = useState<Section>(initial);
  const sections = long ? librarySectionsLong : librarySections;
  const options = counts ? sections : sections.map(({ label, value }) => ({ label, value }));
  return (
    <BrowserTabs
      appearance="underline"
      density={density}
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

// Long labels on a phone: the list scrolls sideways rather than wrap or spill; the last tab has focus,
// so it has scrolled into view.
const LongLabelsPhone: Story = {
  args: { long: true },
  decorators: Phone.decorators,
  play: async ({ canvas }) => {
    canvas.getByRole("tab", { name: /Household collections/ }).focus();
  },
};
// TV density at a desktop width: the type is large enough that the list scrolls.
const Tv: Story = { args: { density: "tv" } };

export default meta;
export { Focused, LaterTabSelected, Light, LongLabelsPhone, Phone, Tv, WithCounts, WithoutCounts };
