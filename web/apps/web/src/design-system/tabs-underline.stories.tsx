import type { Density } from "@loomarr/design-system";
import { librarySections, librarySectionsLong } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { BrowserTabs } from "../client-adapters";

type Section = (typeof librarySections)[number]["value"];

// Tabs, underline appearance: a page's sections as words on a shared rule, the selected one
// underlined in the foreground colour, a count beside each label (spoken as "3,412 titles"). Arrow
// keys, Home and End move between tabs through the browser adapter, as for the default appearance;
// when the tabs do not fit, the list scrolls sideways with its scroll bar, and the selected tab is
// brought into view when the list mounts and when the selection changes.
const UnderlineTabs = ({
  counts = true,
  density = "pointer",
  growing = false,
  initial = "all",
  long = false,
}: {
  counts?: boolean;
  density?: Density;
  /** A button below the list that adds to the first count, as a scan finding titles does. */
  growing?: boolean;
  initial?: Section;
  long?: boolean;
}) => {
  const [value, setValue] = useState<Section>(initial);
  const [added, setAdded] = useState(0);
  const sections = (long ? librarySectionsLong : librarySections).map((section, index) =>
    index === 0 && added > 0 && "count" in section
      ? { ...section, count: `${section.count} + ${added}` }
      : section,
  );
  const options = counts ? sections : sections.map(({ label, value }) => ({ label, value }));
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <BrowserTabs
        appearance="underline"
        density={density}
        label="Library sections"
        onValueChange={setValue}
        options={options}
        value={value}
      />
      {growing ? (
        <button onClick={() => setAdded((now) => now + 1000)} type="button">
          Add titles
        </button>
      ) : null}
    </div>
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

// Long labels on a phone: the list scrolls sideways rather than wrap or spill, with its scroll bar;
// the last tab has focus, so the browser has scrolled it into view.
const LongLabelsPhone: Story = {
  args: { long: true },
  decorators: Phone.decorators,
  play: async ({ canvas }) => {
    canvas.getByRole("tab", { name: /Household collections/ }).focus();
  },
};
// Long labels on a phone with a later tab selected when the list mounts: the list starts scrolled
// so the selected tab and its underline show.
const LongLabelsLaterSelected: Story = {
  args: { initial: "shows", long: true },
  decorators: Phone.decorators,
};
// A count that grows while the list is scrolled: the list stays where the reader left it.
const CountGrows: Story = {
  args: { growing: true, initial: "shows", long: true },
  decorators: Phone.decorators,
};
// TV density at a desktop width: the type is large enough that the list scrolls.
const Tv: Story = { args: { density: "tv" } };

export default meta;
export {
  CountGrows,
  Focused,
  LaterTabSelected,
  Light,
  LongLabelsLaterSelected,
  LongLabelsPhone,
  Phone,
  Tv,
  WithCounts,
  WithoutCounts,
};
