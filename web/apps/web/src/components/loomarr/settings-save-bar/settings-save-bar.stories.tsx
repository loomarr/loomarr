import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { SettingsSaveBar } from "./settings-save-bar";

const noop = () => {};

// Sonarr's sticky save bar (config-design §5): explicit per-page saving, because
// connection settings change together and half-saved pairs are a footgun. It renders
// nothing at all when the page is clean.
const meta = {
  title: "Loomarr/SettingsSaveBar",
  component: SettingsSaveBar,
  args: { dirtyCount: 2, onSave: noop, onDiscard: noop },
  decorators: [widthFrame(560)],
} satisfies Meta<typeof SettingsSaveBar>;

type Story = StoryObj<typeof meta>;

const Dirty: Story = {};
const SingleChange: Story = { args: { dirtyCount: 1 } };
const Saving: Story = { args: { saving: true } };

export default meta;
export { Dirty, Saving, SingleChange };
