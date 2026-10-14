import { searchResults } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SearchCommand } from "./search-command";

const noop = () => {};

// The ⌘K palette (§3, §7.2): idle (prompt) · results (in-library flag) · empty (no match).
const meta = {
  title: "Shell/SearchCommand",
  component: SearchCommand,
  args: { onQueryChange: noop, onSelect: noop },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof SearchCommand>;

type Story = StoryObj<typeof meta>;

const Idle: Story = { args: { query: "", results: [] } };
const Results: Story = { args: { query: "a", results: searchResults } };
const Empty: Story = { args: { query: "zxcv", results: [] } };

export default meta;
export { Empty, Idle, Results };
