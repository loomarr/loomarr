import { people } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PeopleRoster } from "./people-roster";

// A fixed clock so "Last seen" is stable for the visual suite (§5.2). The disabled person has
// never signed in, so their row shows "Never".
const NOW = Date.parse("2026-07-19T12:00:00Z");
const users = [
  { ...people.localAdmin, lastSeenAt: NOW },
  { ...people.importedMember, lastSeenAt: NOW - 2 * 3_600_000 },
  { ...people.offlineReady, lastSeenAt: NOW - 26 * 3_600_000 },
  people.disabled,
];

const meta = {
  title: "People/PeopleRoster",
  component: PeopleRoster,
  args: { users, selfId: people.localAdmin.id, onSelect: () => {}, now: NOW },
  decorators: [widthFrame(960)],
} satisfies Meta<typeof PeopleRoster>;

type Story = StoryObj<typeof meta>;
const Default: Story = {};
const Mobile: Story = { decorators: [widthFrame(390)] };
const Empty: Story = { args: { users: [] } };
const Loading: Story = { args: { users: undefined } };
const FilteredEmpty: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(await canvas.findByLabelText("Search people"), "No such person");
    await canvas.findByText("No matching people");
  },
};

export default meta;
export { Default, Empty, FilteredEmpty, Loading, Mobile };
