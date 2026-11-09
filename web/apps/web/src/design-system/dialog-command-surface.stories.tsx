import { Dialog, DialogContent } from "@loomarr/design-system";
import { searchResults } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { SearchCommand } from "@/components/loomarr/shell/search-command";

// The design-system Dialog's `top` placement, open, holding the ⌘K palette's search: near the top
// of the screen, no panel or corner close of its own, named by its label. It portals into the story
// root so the visual suite snapshots the real popup.
// `fill` is the phone's full-screen search: it covers the screen and the search draws its own close.
const CommandSurfaceWorkshop = ({ query = "", fill = false }: { query?: string; fill?: boolean }) => (
  <div style={{ height: "100vh", width: "100vw" }}>
    <Dialog defaultOpen>
      <DialogContent
        accessibilityLabel="Search Loomarr"
        placement={fill ? "fill" : "top"}
        portalContainer={document.getElementById("storybook-root")}
      >
        <SearchCommand
          fill={fill}
          onClose={fill ? () => undefined : undefined}
          onQueryChange={() => undefined}
          query={query}
          results={query ? searchResults : []}
        />
      </DialogContent>
    </Dialog>
  </div>
);

const meta = {
  title: "Loomarr Components/Dialog command surface",
  component: CommandSurfaceWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof CommandSurfaceWorkshop>;

type Story = StoryObj<typeof meta>;

const Idle: Story = {};
const Results: Story = { args: { query: "a" } };
const PhoneFullScreen: Story = { args: { query: "a", fill: true } };

export default meta;
export { Idle, PhoneFullScreen, Results };
