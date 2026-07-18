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
  ) {
    super(problem.title || problem.detail || `HTTP ${status}`);
    this.name = "ApiError";
  }
}

const CSRF_HEADER = "X-Loomarr-Csrf";

const customFetch = async <T>(url: string, options: RequestInit = {}): Promise<T> => {
  const method = (options.method ?? "GET").toUpperCase();
  const headers = new Headers(options.headers);
  if (method !== "GET" && method !== "HEAD") headers.set(CSRF_HEADER, "1");

  const res = await fetch(url, { ...options, headers, credentials: "include" });

  // 204 / empty body → undefined; otherwise parse JSON (problem+json on error).
  const text = await res.text();
  const body = text ? (JSON.parse(text) as unknown) : undefined;

  if (!res.ok) {
    throw new ApiError(res.status, (body as ErrorModel) ?? { status: res.status });
  }
  return body as T;
};

export { ApiError, customFetch };
