import type { Meta, StoryObj } from "@storybook/react-vite";
import { StateBadge } from "./state-badge";

// The provisioning lifecycle chips (§3, §4): wanted · requested · downloading ·
// available · unavailable · drift.
const meta = {
  title: "Loomarr/StateBadge",
  component: StateBadge,
} satisfies Meta<typeof StateBadge>;

type Story = StoryObj<typeof meta>;

const Wanted: Story = { args: { state: "wanted" } };
const Requested: Story = { args: { state: "requested" } };
const Downloading: Story = { args: { state: "downloading" } };
const Available: Story = { args: { state: "available" } };
const Unavailable: Story = { args: { state: "unavailable" } };
const Drift: Story = { args: { state: "drift" } };

export default meta;
export { Available, Downloading, Drift, Requested, Unavailable, Wanted };
