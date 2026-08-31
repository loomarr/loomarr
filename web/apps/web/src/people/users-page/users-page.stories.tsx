import type { MeBody, UserBody } from "@loomarr/api";
import { people } from "@loomarr/fixtures";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useMemo } from "react";
import { withRouter } from "@/test/story-utils";
import { UsersPage } from "./users-page";

type RosterState = "empty" | "loading" | "mixed";

interface UsersPageStoryProps {
  roster: RosterState;
}

const signedInAdmin: MeBody = {
  id: people.localAdmin.id,
  name: people.localAdmin.name,
  role: "admin",
  disabled: false,
  local: true,
  offlineLogin: false,
  quota: 0,
  autoApprove: true,
};

const candidates = [
  { id: "server-dorothy", name: "Dorothy Vaughan", imported: false, disabled: false, isAdmin: true },
  { id: "server-mary", name: "Mary Jackson", imported: false, disabled: true, isAdmin: false },
  { id: "server-grace", name: "Grace Hopper", imported: true, disabled: false, isAdmin: false },
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });

const peopleFetch =
  (roster: RosterState): typeof fetch =>
  async (input, init) => {
    const request = input instanceof Request ? input : new Request(input, init);
    const url = new URL(request.url);

    if (url.pathname === "/v1/auth/me") return json(signedInAdmin);
    if (url.pathname === "/v1/settings") {
      return json({ settings: [], features: { user_sync: true } });
    }
    if (url.pathname === "/v1/users/candidates") return json({ candidates });
    if (url.pathname === "/v1/invitations" && request.method === "GET") {
      return json({ invitations: [] });
    }
    if (url.pathname.endsWith("/sessions") && request.method === "GET") return json({ sessions: [] });
    if (url.pathname === "/v1/users" && request.method === "GET") {
      if (roster === "loading") return await new Promise<Response>(() => {});
      return json({ users: roster === "empty" ? [] : Object.values(people) });
    }

    if (url.pathname === "/v1/users/import") return json({ imported: 1 });
    if (url.pathname === "/v1/users/sync") return json({ synced: Object.keys(people).length });
    if (url.pathname === "/v1/users" && request.method === "POST") return json(people.localAdmin, 201);
    if (url.pathname.startsWith("/v1/users/") && request.method === "PATCH") {
      return json(people.importedMember satisfies UserBody);
    }
    if (request.method === "DELETE" || url.pathname.endsWith("/password")) {
      return new Response(null, { status: 204 });
    }

    return json({ title: "Unstubbed People story request", detail: url.pathname, status: 500 }, 500);
  };

const UsersPageStory = ({ roster }: UsersPageStoryProps) => {
  window.fetch = peopleFetch(roster);
  const queryClient = useMemo(() => new QueryClient({ defaultOptions: { queries: { retry: false } } }), []);
  return (
    <QueryClientProvider client={queryClient}>
      <div className="h-screen w-screen bg-background">
        <UsersPage />
      </div>
    </QueryClientProvider>
  );
};

const meta = {
  title: "People/UsersPage",
  component: UsersPageStory,
  args: { roster: "mixed" },
  decorators: [withRouter("/people")],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof UsersPageStory>;

type Story = StoryObj<typeof meta>;

const MixedRoster: Story = {};
const SelectedOfflineReady: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Manage Katherine Johnson" }));
  },
};
const ImportOpen: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Import from Emby/Jellyfin" }));
  },
};
const LocalAccountOpen: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Add local account" }));
  },
};
const InviteOpen: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Invite person" }));
  },
};
const FilteredEmpty: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.type(await canvas.findByLabelText("Search people"), "No such person");
    await canvas.findByText("No matching people");
  },
};
const Empty: Story = { args: { roster: "empty" } };
const Loading: Story = { args: { roster: "loading" } };

export default meta;
export {
  Empty,
  FilteredEmpty,
  ImportOpen,
  InviteOpen,
  Loading,
  LocalAccountOpen,
  MixedRoster,
  SelectedOfflineReady,
};
