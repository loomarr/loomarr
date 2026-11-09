import { Icon, icons, Menu, MenuAction, MenuContent, MenuGroup, MenuTrigger } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The design-system Menu, open, with the channel row's actions. It portals into the story root
// rather than `document.body`, so the visual suite, which snapshots `#storybook-root`, sees the real
// panel. `armed` shows the in-menu delete confirmation the danger action opens.
type MenuWorkshopProps = { armed?: boolean };

const MenuWorkshop = ({ armed = false }: MenuWorkshopProps) => (
  <div style={{ height: "100vh", padding: 24, width: "100vw" }}>
    <Menu defaultOpen>
      <MenuTrigger>
        <button
          aria-label="Actions for Morning Cartoons"
          style={{ background: "none", border: 0, color: "var(--contentPrimary)", padding: 8 }}
          type="button"
        >
          <Icon decorative glyph={icons.more} size="compact" tone="inherit" />
        </button>
      </MenuTrigger>
      <MenuContent align="start" portalContainer={document.getElementById("storybook-root")} width={236}>
        <MenuAction
          icon={<Icon decorative glyph={icons.channels} size="compact" tone="primary" />}
          onSelect={() => {}}
        >
          Watch
        </MenuAction>
        <MenuAction
          icon={<Icon decorative glyph={icons.edit} size="compact" tone="muted" />}
          onSelect={() => {}}
        >
          Edit channel
        </MenuAction>
        <MenuAction
          icon={<Icon decorative glyph={icons.pause} size="compact" tone="inherit" />}
          onSelect={() => {}}
        >
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
            icon={<Icon decorative glyph={icons.delete} size="compact" tone="inherit" />}
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
