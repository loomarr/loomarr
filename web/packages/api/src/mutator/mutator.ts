// The shared fetch mutator orval's generated hooks call (frontend-design §4.2 —
// packages/api is platform-agnostic; TanStack Query runs on RN too). It encodes
// Loomarr's transport contract once:
//   - same-origin, relative URLs (the SPA is embedded; Vite proxies /v1 in dev)
//   - cookie session auth (credentials: include) — §11
//   - the double-submit CSRF header on state-changing requests (§11)
//   - RFC 7807 problem+json surfaced as a typed error (rendered by ErrorState, §3)

import type { ErrorModel } from "../../generated/model";

class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly problem: ErrorModel,
    readonly requestId?: string,
  ) {
    super(problem.title || problem.detail || `HTTP ${status}`);
    this.name = "ApiError";
  }
}

// Normalize any thrown value into an RFC 7807 problem. Lives here beside ApiError, not
// in the component that renders it: an unknown → ErrorModel coercion has no DOM surface,
// and mobile needs the identical mapping the day it renders its first failed request.
const toProblem = (err: unknown): ErrorModel => {
  if (err instanceof ApiError) return err.problem;
  if (err instanceof Error) return { title: err.message };
  return { title: "Something went wrong" };
};

const CSRF_HEADER = "X-Loomarr-Csrf";
type ApiFailure = { requestId: string; status: number };
let apiFailureObserver: ((failure: ApiFailure) => void) | undefined;
const observeApiFailures = (observer: (failure: ApiFailure) => void) => {
  apiFailureObserver = observer;
  return () => {
    if (apiFailureObserver === observer) apiFailureObserver = undefined;
  };
};
type InitialAuthGlobal = typeof globalThis & {
  __loomarrInitialAuthResponse?: Promise<Response>;
};

const customFetch = async <T>(url: string, options: RequestInit = {}): Promise<T> => {
  const method = (options.method ?? "GET").toUpperCase();
  const headers = new Headers(options.headers);
  if (method !== "GET" && method !== "HEAD") headers.set(CSRF_HEADER, "1");

  // The embedded document starts its one inevitable auth read while the entry graph is
  // downloading. Adopt that Response here so the route guard keeps its single-query and
  // error semantics without serializing auth behind JavaScript evaluation. React Native
  // never defines this optional browser bootstrap, so the shared transport remains portable.
  const initialAuthGlobal = globalThis as InitialAuthGlobal;
  const prefetchedAuth =
    method === "GET" && url === "/v1/auth/me" ? initialAuthGlobal.__loomarrInitialAuthResponse : undefined;
  if (prefetchedAuth) delete initialAuthGlobal.__loomarrInitialAuthResponse;
  const res = await (prefetchedAuth ?? fetch(url, { ...options, headers, credentials: "include" }));

  // 204 / empty body → undefined; otherwise parse JSON (problem+json on error).
  const text = await res.text();
  const body = text ? (JSON.parse(text) as unknown) : undefined;

  if (!res.ok) {
    const requestId = res.headers.get("X-Request-Id") ?? undefined;
    if (requestId && url !== "/v1/diagnostics/client-events") apiFailureObserver?.({ requestId, status: res.status });
    throw new ApiError(res.status, (body as ErrorModel) ?? { status: res.status }, requestId);
  }
  // orval's fetch client expects the response ENVELOPE (like axios's response), not the
  // bare body — the generated hooks type `data` as `{ status, data }`, so a caller reads
  // `useX().data?.data`. Returning the raw body here typechecks (via the cast) but is
  // undefined at runtime; return the envelope.
  return { status: res.status, data: body, headers: res.headers } as T;
};

export type { ApiFailure };
export { ApiError, customFetch, observeApiFailures, toProblem };
