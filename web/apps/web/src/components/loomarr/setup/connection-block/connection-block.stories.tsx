import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/ui";
import { widthFrame, withRouter } from "@/test/story-utils";
import { ConnectionBlock } from "./connection-block";

// ConnectionBlock — one self-diagnosing connection (config-design §5/§6). The story set is
// the visual contract: the header status dot across pass/fail/untested, collapsed vs open,
// and the inline verdict + Fix link that a failing block carries. withRouter because the
// "Fix →" is a routed Link into the Help center.
const meta = {
  title: "Setup/ConnectionBlock",
  component: ConnectionBlock,
  args: { onToggle: () => {} },
  decorators: [widthFrame(560), withRouter("/settings")],
} satisfies Meta<typeof ConnectionBlock>;

type Story = StoryObj<typeof meta>;

// Stand-in for a SettingsFields group — the block is presentational, so the story doesn't
// need the real field controls, just something in the body to prove the reveal.
const fields = (
  <div className="flex flex-col gap-3 text-muted-foreground text-sm">
    <p>Library URL, token, and flavor render here (a SettingsFields group).</p>
  </div>
);

const testBtn = (
  <Button variant="outline" size="sm">
    Test connection
  </Button>
);

// Healthy + collapsed — the resting state of a passing connection: a green dot, nothing else.
const HealthyCollapsed: Story = {
  args: { title: "Requester (Seerr)", optional: true, verdict: { ok: true }, open: false, children: fields },
};

// Broken + open — the state a Connections page opens in for a failing service.
const BrokenOpen: Story = {
  args: {
    title: "Media server",
    verdict: { ok: false, hint: "could not reach the media server: GET /Users: status 401" },
    docHref: "troubleshooting#media-server",
    open: true,
    action: testBtn,
    children: fields,
  },
};

// Passing + open — fields visible with a green "Connection OK" verdict.
const HealthyOpen: Story = {
  args: {
    title: "TMDB",
    optional: true,
    verdict: { ok: true },
    open: true,
    action: testBtn,
    children: fields,
  },
};

// Untested — a neutral dot, no verdict line (a connection never probed yet).
const Untested: Story = {
  args: { title: "Tunarr", open: true, action: testBtn, children: fields },
};

export default meta;
export { BrokenOpen, HealthyCollapsed, HealthyOpen, Untested };
