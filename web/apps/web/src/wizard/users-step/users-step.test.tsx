import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UsersStep } from "./users-step";

const CANDIDATES = [
  { id: "u-ada", name: "Ada", isAdmin: true, disabled: false, imported: false },
  { id: "u-bo", name: "Bo", isAdmin: false, disabled: false, imported: true },
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const stubFetch = (opts: { fail?: boolean } = {}) => {
  const mock = vi.fn((url: string, _init?: RequestInit) => {
    const u = String(url);
    if (u.includes("/v1/users/candidates")) {
      return opts.fail
        ? Promise.resolve(json({ title: "no media server configured" }, 502))
        : Promise.resolve(json({ candidates: CANDIDATES }));
    }
    return Promise.resolve(json({ imported: 1 }));
  });
  vi.stubGlobal("fetch", mock);
  return mock;
};

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    {children}
  </QueryClientProvider>
);

afterEach(() => vi.restoreAllMocks());

describe("UsersStep", () => {
  it("lists candidates by name and locks the already-imported", async () => {
    stubFetch();
    render(<UsersStep />, { wrapper });

    expect(await screen.findByLabelText("Ada")).toBeEnabled();
    // Imported people stay visible but locked — "where did they go?" is worse.
    const bo = screen.getByLabelText("Bo");
    expect(bo).toBeDisabled();
    expect(bo).toBeChecked();
  });

  it("imports only the people the admin picked", async () => {
    const fetchMock = stubFetch();
    render(<UsersStep />, { wrapper });

    await userEvent.click(await screen.findByLabelText("Ada"));
    await userEvent.click(screen.getByRole("button", { name: /import/i }));

    const posted = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/users/import"));
    expect(posted).toBeTruthy();
    expect(JSON.parse(String(posted?.[1]?.body))).toEqual({ ids: ["u-ada"] });
  });

  it("cannot import nobody", async () => {
    stubFetch();
    render(<UsersStep />, { wrapper });
    await screen.findByLabelText("Ada");
    expect(screen.getByRole("button", { name: /import/i })).toBeDisabled();
  });

  it("treats no-media-server as a reason to skip, not a wall", async () => {
    stubFetch({ fail: true });
    render(<UsersStep />, { wrapper });
    expect(await screen.findByText(/skip this and import people later/i)).toBeInTheDocument();
  });
});
