import { Field } from "@loomarr/design-system";
import { libraryQuery } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";

// Field as a title search: what someone types is words, so it is set in the body face (mono is for
// numbers, clocks and file names). This is Field's default; the story pins the look a Library search
// relies on.
const meta = {
  title: "Loomarr Foundations/Field title search",
  component: Field,
  args: {
    accessibilityLabel: "Search the library",
    inputMode: "search",
    onChangeText: () => undefined,
    placeholder: "Search titles, years, genres",
    value: "",
  },
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof Field>;

type Story = StoryObj<typeof meta>;

// The wall's search box is 256 wide beside the sort and genre pickers.
const besidePickers: Story["decorators"] = [
  (Story) => (
    <div style={{ maxWidth: 256 }}>
      <Story />
    </div>
  ),
];

const Empty: Story = { decorators: besidePickers };
const Typed: Story = { args: { value: libraryQuery }, decorators: besidePickers };
const Phone: Story = {
  args: { value: libraryQuery },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 390 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
export { Empty, Phone, Typed };
