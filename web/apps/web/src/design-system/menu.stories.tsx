import { Menu, MenuAction, MenuContent, MenuGroup, MenuTrigger } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { MoreVertical, Pause, Pencil, Trash2, Tv } from "lucide-react";

// The design-system Menu, open, with the channel row's actions. It portals into the story root
// rather than `document.body`, so the visual suite, which snapshots `#storybook-root`, sees the real
// panel. `armed` shows the in-menu delete confirmation the danger action opens.
type MenuWorkshopProps = { armed?: boolean };

const icon = { height: 16, width: 16 };

const MenuWorkshop = ({ armed = false }: MenuWorkshopProps) => (
  <div style={{ height: "100vh", padding: 24, width: "100vw" }}>
    <Menu defaultOpen>
      <MenuTrigger>
        <button
          aria-label="Actions for Morning Cartoons"
          style={{ background: "none", border: 0, color: "var(--contentPrimary)", padding: 8 }}
          type="button"
        >
          <MoreVertical aria-hidden style={icon} />
        </button>
      </MenuTrigger>
      <MenuContent align="start" portalContainer={document.getElementById("storybook-root")} width={236}>
        <MenuAction icon={<Tv aria-hidden style={icon} />} onSelect={() => {}}>
          Watch
        </MenuAction>
        <MenuAction icon={<Pencil aria-hidden style={icon} />} onSelect={() => {}}>
          Edit channel
        </MenuAction>
        <MenuAction icon={<Pause aria-hidden style={icon} />} onSelect={() => {}}>
          Pause
        </MenuAction>
        {armed ? (
          <MenuGroup label="Delete Morning Cartoons for good? This can't be undone." tone="danger">
            <MenuAction closeOnSelect={false} onSelect={() => {}} tone="danger">
              Delete
            </MenuAction>
            <MenuAction closeOnSelect={false} onSelect={() => {}}>
              Cancel
            </MenuAction>
          </MenuGroup>
        ) : (
          <MenuAction
            closeOnSelect={false}
            icon={<Trash2 aria-hidden style={icon} />}
            onSelect={() => {}}
            tone="danger"
          >
            Delete…
          </MenuAction>
        )}
      </MenuContent>
    </Menu>
  </div>
);

const meta = {
  title: "Loomarr Components/Menu",
  component: MenuWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof MenuWorkshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const DeleteArmed: Story = { args: { armed: true } };

export default meta;
export { Default, DeleteArmed };
