import { people } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PeopleRoster } from "./people-roster";

const meta = {
  title: "People/PeopleRoster",
  component: PeopleRoster,
  args: { users: Object.values(people), selfId: people.localAdmin.id, onSelect: () => {} },
  decorators: [widthFrame(960)],
} satisfies Meta<typeof PeopleRoster>;

type Story = StoryObj<typeof meta>;
const Default: Story = {};
const Mobile: Story = { decorators: [widthFrame(390)] };
const Empty: Story = { args: { users: [] } };
const Loading: Story = { args: { users: undefined } };

export default meta;
export { Default, Empty, Loading, Mobile };
