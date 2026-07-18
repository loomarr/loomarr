import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, customFetch } from "./mutator";

const mockFetch = (status: number, body: unknown) =>
  vi.fn((_url: string, _init?: RequestInit) =>
    Promise.resolve(
      // 204/304 are null-body statuses — Response throws on a non-null body there.
      new Response(body === undefined ? null : JSON.stringify(body), {
        status,
        headers: { "content-type": "application/json" },
      }),
    ),
  );

afterEach(() => vi.restoreAllMocks());

describe("customFetch", () => {
  it("sends cookies and a CSRF header on mutations, none on GET", async () => {
    const fetchSpy = mockFetch(200, { ok: true });
    vi.stubGlobal("fetch", fetchSpy);

    await customFetch("/v1/channels", { method: "POST" });
    await customFetch("/v1/channels");

    const postInit = fetchSpy.mock.calls[0]![1]!;
    const getInit = fetchSpy.mock.calls[1]![1]!;
    expect(postInit.credentials).toBe("include");
    expect(new Headers(postInit.headers).get("X-Loomarr-Csrf")).toBe("1");
    expect(new Headers(getInit.headers).get("X-Loomarr-Csrf")).toBeNull();
  });

  it("throws a typed ApiError carrying the RFC7807 problem on non-2xx", async () => {
    vi.stubGlobal("fetch", mockFetch(502, { title: "Bad gateway", detail: "tunarr down" }));
    await expect(customFetch("/v1/setup/tunarr-connect", { method: "POST" })).rejects.toMatchObject({
      status: 502,
      problem: { title: "Bad gateway", detail: "tunarr down" },
    });
  });

  it("returns the { status, data } envelope orval's fetch client expects", async () => {
    vi.stubGlobal("fetch", mockFetch(200, { id: "u1", name: "Ada" }));
    await expect(customFetch("/v1/auth/me")).resolves.toMatchObject({
      status: 200,
      data: { id: "u1", name: "Ada" },
    });
  });

  it("carries an undefined data for an empty (204) body", async () => {
    vi.stubGlobal("fetch", mockFetch(204, undefined));
    await expect(customFetch("/v1/auth/logout", { method: "POST" })).resolves.toMatchObject({
      status: 204,
      data: undefined,
    });
  });

  it("ApiError is an Error subclass with the problem attached", () => {
    const err = new ApiError(404, { title: "Not found" });
    expect(err).toBeInstanceOf(Error);
    expect(err.status).toBe(404);
    expect(err.message).toBe("Not found");
  });
});
