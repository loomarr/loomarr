import type { UserBody } from "@loomarr/api";
import {
  getCreateLocalUserMockHandler,
  getImportCandidatesMockHandler,
  getListUserSessionsMockHandler,
  getListUsersMockHandler,
  getMeMockHandler,
  getPatchUserMockHandler,
  getResetUserPasswordMockHandler,
  getSettingsListMockHandler,
  getSyncUsersMockHandler,
} from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRouter, RouterProvider } from "@tanstack/react-router";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { routeTree } from "@/routeTree.gen";
import { me, user } from "@/test/fixtures/users";
import { appHandlers } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

// ⚠ The allowlist used to be built by SPREADING the signed-in user —
// `USERS = [{ ...ADMIN, local: true }, { ...MEMBER, local: false }]` — but `MeBody` and `UserBody`
// are different types: `UserBody` also requires `effectiveQuota` and `pendingAcquisitions`. Both
// rows were served without either field, and the untyped stub was happy to. `pendingAcquisitions`
// is a number the quota column reads.
const ADA = user({ id: "u1", name: "Ada", role: "admin", local: true });
const GRACE = user({
  id: "u2",
  name: "Grace",
  role: "member",
  local: false,
  autoApprove: false,
  quota: 5,
  effectiveQuota: 5,
});

// ⚠ FOUR endpoints in this file were being matched by URL SUBSTRING, and three of those matches
// were wrong in a way no assertion could see:
//
//   • `u.includes("/sessions")` matched BOTH `GET /v1/users/:id/sessions` (a user's live sessions)
//     and `DELETE /v1/sessions/:hash` (revoking one). Two endpoints, one branch.
//   • `u.includes("/v1/users") && method === "PATCH"` matched a PATCH to ANY path under
//     `/v1/users` and always answered with Grace.
//   • `u.includes("/v1/users/candidates")` had to be tested BEFORE `u.includes("/v1/users")`,
//     because `includes` has no notion of a more specific route. So did `/v1/users/import` and
//     `/v1/users/sync`. The ordering was load-bearing and nothing enforced it.
//
// Every one of those is now a route-bound handler: MSW parses the path, so `POST /v1/users`
// (create) and `POST /v1/users/:id/password` (admin reset) can no longer answer for each other.
const stubUsers = ({
  who = me(),
  userSync = true,
  candidates = [],
  sessions = [],
}: {
  who?: ReturnType<typeof me>;
  userSync?: boolean;
  candidates?: Array<{
    id: string;
    name: string;
    imported: boolean;
    disabled: boolean;
    isAdmin: boolean;
  }>;
  sessions?: Array<{
    id: string;
    userId: string;
    createdAt: number;
    expiresAt: number;
    current: boolean;
  }>;
} = {}) => {
  // Resolver-recorded request bodies. The old assertions dug through `fetchMock.mock.calls` for a
  // url substring, which only ever proved the TEST's own spelling; landing in a route-bound
  // resolver proves the request reached that route.
  const patches: unknown[] = [];
  const creates: unknown[] = [];
  const resets: Array<{ id: string; body: unknown }> = [];

  server.use(
    getMeMockHandler(who),
    getListUsersMockHandler({ users: [ADA, GRACE] }),
    getImportCandidatesMockHandler({ candidates }),
    getListUserSessionsMockHandler({ sessions }),
    getSyncUsersMockHandler({ synced: 2 }),
    getPatchUserMockHandler(async ({ request }) => {
      patches.push(await request.json());
      return { ...GRACE, role: "admin" } satisfies UserBody;
    }),
    getCreateLocalUserMockHandler(async ({ request }) => {
      creates.push(await request.json());
      return user({ id: "u3", name: "newcomer", role: "member" });
    }),
    getResetUserPasswordMockHandler(async ({ request, params }) => {
      resets.push({ id: String(params.id), body: await request.json() });
    }),
    // ⚠ `user_sync` is a COMPUTED feature, not a setting: POST /v1/users/sync is registered only
    // when a media server is configured, so the page reads the feature set before offering it.
    getSettingsListMockHandler({ settings: [], features: { user_sync: userSync } }),
    // Spread LAST — see handlers.ts: MSW takes the FIRST match and `use()` prepends, so a
    // baseline spread first would shadow every override above it.
    ...appHandlers(),
  );

  return { patches, creates, resets };
};

const renderAt = (path: string) => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const router = createRouter({
    routeTree,
    context: { queryClient },
    history: createMemoryHistory({ initialEntries: [path] }),
  });
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
};

// These are PAGE-level assertions on purpose. Four times in this phase a component was
// built, unit-tested, and never actually mounted by any route — the component tests all
// passed while the feature was absent. Component tests pin behavior; these pin the wiring.
describe("Users page", () => {
  it("lists the allowlist with each row's credential path", async () => {
    stubUsers();
    renderAt("/people");
    // Await Grace, not Ada: the signed-in admin's name also renders in the app shell's
    // user menu, so awaiting "Ada" resolves before the table has loaded at all.
    expect(await screen.findByText("Grace")).toBeInTheDocument();
    expect(screen.getAllByText("Ada").length).toBeGreaterThan(0);
    // Exact strings, not regexes: a partial matcher also matches the ancestor <div>,
    // which contains the same text plus the user's name.
    expect(screen.getByText("Local account")).toBeInTheDocument();
    expect(screen.getByText("Media-server account")).toBeInTheDocument();
  });

  it("patches a single field without a save step", async () => {
    const { patches } = stubUsers();
    renderAt("/people");
    await screen.findByText("Grace");

    // Grace's row — the second Role select. Open it, then pick admin from its listbox
    // (only the opened Select mounts its options, so the query is unambiguous).
    const roles = screen.getAllByLabelText("Role");
    await userEvent.click(roles[1] as HTMLElement);
    await userEvent.click(await screen.findByRole("option", { name: "admin" }));

    expect(patches, "changing a role should PATCH immediately").toEqual([{ role: "admin" }]);
  });

  it("opens a user's live sessions", async () => {
    stubUsers({
      sessions: [
        {
          id: "hash-a",
          userId: "u2",
          createdAt: Date.now() - 3_600_000,
          expiresAt: Date.now() + 3_600_000,
          current: false,
        },
      ],
    });
    renderAt("/people");
    await screen.findByText("Grace");

    const buttons = await screen.findAllByRole("button", { name: /sessions/i });
    await userEvent.click(buttons[1] as HTMLElement);
    expect(await screen.findByText(/1 active session for Grace/)).toBeInTheDocument();
  });

  // The import panel is the whole point of §11's "explicit import, never implicit". If it
  // is not on the page, an admin has no way to grant access at all.
  it("mounts the import panel with real candidate names", async () => {
    stubUsers({
      candidates: [
        { id: "e1", name: "Hopper", imported: false, disabled: false, isAdmin: true },
        { id: "e2", name: "Lovelace", imported: true, disabled: false, isAdmin: false },
      ],
    });
    renderAt("/people");
    expect(await screen.findByText("Hopper")).toBeInTheDocument();
    // Already-imported accounts stay visible, checked and locked — hiding them reads as
    // "missing", and re-offering them implies a no-op does something.
    expect(screen.getByText(/already imported/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/lovelace/i)).toBeDisabled();
  });

  // POST /v1/users/sync is registered only when a media server is configured, so the FE
  // must read the computed feature set first — otherwise it offers a call that 404s.
  it("explains rather than offering import when no media server is connected", async () => {
    stubUsers({ userSync: false });
    renderAt("/people");
    await screen.findByText("Ada");
    expect(await screen.findByText(/connect emby or jellyfin/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /sync existing/i })).not.toBeInTheDocument();
  });

  // V7c: the admin half of local accounts. V7 shipped the endpoints; without these the
  // capability was reachable only by hand-crafting an API call — the same gap the
  // Account screen (V7b) closed on the self-service side.
  it("mounts the create-local-account panel and sends the typed role", async () => {
    // ⚠ The old assertion looked for a POST whose url merely CONTAINED "/v1/users" — which is
    // also true of `/v1/users/import`, `/v1/users/sync` and `/v1/users/u1/password`, all POSTs.
    // It happened to find the right one; it did not assert the right one. `creates` is fed only
    // by the handler bound to `POST /v1/users`.
    const { creates } = stubUsers();
    renderAt("/people");
    await userEvent.click(await screen.findByRole("button", { name: /create local account/i }));
    await userEvent.type(screen.getByLabelText(/username/i), "newcomer");
    await userEvent.type(screen.getByLabelText(/^password$/i), "a-good-password");
    await userEvent.click(screen.getByRole("button", { name: /create account/i }));

    expect(creates).toEqual([
      expect.objectContaining({ username: "newcomer", password: "a-good-password", role: "member" }),
    ]);
  });

  it("offers Reset password on a local row and not on an imported one", async () => {
    // The row's local/media-server label has always existed to explain "whether a
    // password reset is even meaningful". This is that action — and the negative is the
    // point: Loomarr never held an imported user's credential, so offering to reset it
    // would imply it could change their media-server password.
    stubUsers();
    renderAt("/people");
    // Wait on a ROW affordance, not on "Ada" — that name also renders in the nav footer
    // as the signed-in user, so it resolves before the users query settles.
    await screen.findAllByRole("button", { name: /sessions/i });
    const resets = screen.getAllByRole("button", { name: /reset password/i });
    expect(resets).toHaveLength(1); // Ada is local; Grace is imported
  });

  it("sends the new password to the admin reset route", async () => {
    const { resets } = stubUsers();
    renderAt("/people");
    await screen.findAllByRole("button", { name: /sessions/i });
    await userEvent.click(screen.getByRole("button", { name: /reset password/i }));
    await userEvent.type(await screen.findByLabelText(/new password/i), "an-admin-set-pw");
    await userEvent.click(screen.getByRole("button", { name: /set new password/i }));

    // The id comes from the PATH PARAM the resolver parsed, so this pins which user was reset —
    // the old `u.includes("/v1/users/u1/password")` could only confirm the string it was given.
    expect(resets).toEqual([{ id: "u1", body: { next: "an-admin-set-pw" } }]);
  });

  it("refuses the page to a member with an explanation, not failed requests", async () => {
    // ⚠ Was `{ ...MEMBER } as typeof ADMIN` — a cast that existed only to make an incomplete
    // object typecheck. §19's negative case deserves better than a silenced type.
    stubUsers({ who: me({ id: "u2", name: "Grace", role: "member", autoApprove: false, quota: 5 }) });
    renderAt("/people");
    expect(await screen.findByText(/admins only/i)).toBeInTheDocument();
  });
});
