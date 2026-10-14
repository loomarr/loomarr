import { getImportCandidatesMockHandler, getImportUsersMockHandler } from "@loomarr/api/msw";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { server } from "@/test/msw/server";
import { UsersStep } from "./users-step";

const CANDIDATES = [
  { id: "u-ada", name: "Ada", isAdmin: true, disabled: false, imported: false },
  { id: "u-bo", name: "Bo", isAdmin: false, disabled: false, imported: true },
];

// ⚠ Request capture changed SHAPE here, not just mechanism. The old test reached into
// `fetchMock.mock.calls` and searched for a url containing "/v1/users/import" — which means it was
// asserting against a string it wrote itself, and a call to the wrong endpoint would have been
// indistinguishable from no call at all. Now the handler is bound to the route, so arriving in the
// resolver IS the proof the right endpoint was hit, and `imports` only records the body.
const stubUsers = () => {
  const imports: unknown[] = [];
  server.use(
    getImportCandidatesMockHandler({ candidates: CANDIDATES }),
    getImportUsersMockHandler(async ({ request }) => {
      imports.push(await request.json());
      return { imported: 1 };
    }),
  );
  return { imports };
};

// ⚠ Hand-written because the failure is a STATUS, and this spec declares errors with `default:`
// (RFC 7807) rather than enumerating 4xx/5xx — there are zero explicit error codes in the whole
// document, so orval has no status to generate a handler from. Safe against a rename anyway: a
// stale path stops matching, the component's real request goes unhandled, and the guard in
// `src/test/msw/server.ts` fails the test by name rather than quietly serving nothing.
const noMediaServer = () =>
  http.get("*/v1/users/candidates", () =>
    HttpResponse.json({ title: "no media server configured" }, { status: 502 }),
  );

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

describe("UsersStep", () => {
  it("lists candidates by name and locks the already-imported", async () => {
    stubUsers();
    render(<UsersStep />, { wrapper });

    expect(await screen.findByLabelText("Ada")).toBeEnabled();
    // Imported people stay visible but locked — "where did they go?" is worse.
    const bo = screen.getByLabelText("Bo");
    expect(bo).toBeDisabled();
    expect(bo).toBeChecked();
  });

  it("imports only the people the admin picked", async () => {
    const { imports } = stubUsers();
    render(<UsersStep />, { wrapper });

    await userEvent.click(await screen.findByLabelText("Ada"));
    await userEvent.click(screen.getByRole("button", { name: /import/i }));

    await screen.findByLabelText("Ada");
    expect(imports).toEqual([{ ids: ["u-ada"] }]);
  });

  it("cannot import nobody", async () => {
    stubUsers();
    render(<UsersStep />, { wrapper });
    await screen.findByLabelText("Ada");
    expect(screen.getByRole("button", { name: /import/i })).toBeDisabled();
  });

  it("treats no-media-server as a reason to skip, not a wall", async () => {
    server.use(noMediaServer());
    render(<UsersStep />, { wrapper });
    expect(await screen.findByText(/skip this and import people later/i)).toBeInTheDocument();
  });
});
