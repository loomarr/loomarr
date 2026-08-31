import { people } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { PersonDetail } from "./person-detail";

const meta = {
  title: "People/PersonDetail",
  component: PersonDetail,
  args: { user: people.localAdmin, sessions: [], onResetPassword: async () => {} },
  decorators: [widthFrame(576)],
} satisfies Meta<typeof PersonDetail>;

type Story = StoryObj<typeof meta>;
const Local: Story = {};
const Imported: Story = { args: { user: people.offlineReady, onResetPassword: undefined } };
const Disabled: Story = { args: { user: people.disabled } };
const Self: Story = { args: { isSelf: true } };
const Busy: Story = { args: { busy: true } };

export default meta;
export { Busy, Disabled, Imported, Local, Self };
