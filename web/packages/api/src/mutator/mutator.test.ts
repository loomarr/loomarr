import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, customFetch, observeApiFailures, toProblem } from "./mutator";

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

afterEach(() => {
  delete (globalThis as typeof globalThis & { __loomarrInitialAuthResponse?: Promise<Response> })
    .__loomarrInitialAuthResponse;
  vi.restoreAllMocks();
});

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

  it("reports only the bounded request correlation from failed operations", async () => {
    const observed = vi.fn();
    const stop = observeApiFailures(observed);
    vi.stubGlobal(
      "fetch",
      vi.fn(() =>
        Promise.resolve(
          new Response(JSON.stringify({ title: "Unavailable", detail: "private upstream detail" }), {
            status: 503,
            headers: { "X-Request-Id": "request_1" },
          }),
        ),
      ),
    );

    await expect(customFetch("/v1/channels")).rejects.toMatchObject({ requestId: "request_1" });
    expect(observed).toHaveBeenCalledWith({ requestId: "request_1", status: 503 });
    expect(JSON.stringify(observed.mock.calls)).not.toContain("private upstream detail");
    stop();
  });

  it("returns the { status, data } envelope orval's fetch client expects", async () => {
    vi.stubGlobal("fetch", mockFetch(200, { id: "u1", name: "Ada" }));
    await expect(customFetch("/v1/auth/me")).resolves.toMatchObject({
      status: 200,
      data: { id: "u1", name: "Ada" },
    });
  });

  it("adopts the document's in-flight initial auth request exactly once", async () => {
    const fetchSpy = mockFetch(200, { id: "later" });
    vi.stubGlobal("fetch", fetchSpy);
    (
      globalThis as typeof globalThis & {
        __loomarrInitialAuthResponse?: Promise<Response>;
      }
    ).__loomarrInitialAuthResponse = Promise.resolve(
      new Response(JSON.stringify({ id: "prefetched", name: "Ada" }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
    );

    await expect(customFetch("/v1/auth/me")).resolves.toMatchObject({
      status: 200,
      data: { id: "prefetched", name: "Ada" },
    });
    expect(fetchSpy).not.toHaveBeenCalled();

    await expect(customFetch("/v1/auth/me")).resolves.toMatchObject({ data: { id: "later" } });
    expect(fetchSpy).toHaveBeenCalledOnce();
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

// toProblem is what every error surface renders through, so its FALLBACKS are the
// contract: a thrown string or a non-ApiError must still produce a titled problem, or
// ErrorState renders an empty box on the failure the operator most needs explained.
describe("toProblem", () => {
  it("passes an ApiError's problem through untouched", () => {
    const problem = { title: "Conflict", detail: "already exists" };
    expect(toProblem(new ApiError(409, problem))).toEqual(problem);
  });

  it("falls back to the message for a plain Error", () => {
    expect(toProblem(new Error("plain")).title).toBe("plain");
  });

  it("falls back to a generic title for a non-Error throw", () => {
    expect(toProblem("weird").title).toBe("Something went wrong");
    expect(toProblem(undefined).title).toBe("Something went wrong");
  });
});
