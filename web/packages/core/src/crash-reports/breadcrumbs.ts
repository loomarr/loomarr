import { type ClientCrashBreadcrumb, ClientCrashBreadcrumbCategory } from "./contract";
import { scrubText, scrubUrl, truncateUtf8 } from "./scrub";

const BREADCRUMB_LIMIT = 50;
const BREADCRUMB_BYTES = 256;

/** The last 50 things the app did, scrubbed as they are recorded, for the next crash report. */
class BreadcrumbTrail {
  private readonly entries: ClientCrashBreadcrumb[] = [];

  constructor(private readonly now: () => number = Date.now) {}

  add(category: ClientCrashBreadcrumbCategory, message: string) {
    this.entries.push({
      at: new Date(this.now()).toISOString(),
      category,
      message: truncateUtf8(scrubText(message), BREADCRUMB_BYTES),
    });
    if (this.entries.length > BREADCRUMB_LIMIT)
      this.entries.splice(0, this.entries.length - BREADCRUMB_LIMIT);
  }

  snapshot(): ClientCrashBreadcrumb[] {
    return this.entries.map((entry) => ({ ...entry }));
  }
}

// Spelled out rather than `Parameters<typeof fetch>[0]`: React Native's fetch types omit URL, the
// DOM's include it, and this module type-checks under both.
type FetchInput = string | URL | Request;

const requestUrl = (input: FetchInput) =>
  typeof input === "string" ? input : input instanceof URL ? input.href : input.url;

const requestMethod = (input: FetchInput, init: RequestInit | undefined) =>
  (init?.method ?? (typeof input === "object" && "method" in input ? input.method : "GET")).toUpperCase();

/**
 * Wraps a fetch so each request leaves an `http` breadcrumb: method, path and status. Headers, query
 * strings and bodies are never read.
 */
const createBreadcrumbFetch =
  (fetcher: typeof globalThis.fetch, trail: BreadcrumbTrail): typeof globalThis.fetch =>
  async (input, init) => {
    const request = `${requestMethod(input, init)} ${scrubUrl(requestUrl(input))}`;
    try {
      const response = await fetcher(input, init);
      trail.add(ClientCrashBreadcrumbCategory.http, `${request} ${response.status}`);
      return response;
    } catch (error) {
      trail.add(ClientCrashBreadcrumbCategory.http, `${request} failed`);
      throw error;
    }
  };

export { BreadcrumbTrail, createBreadcrumbFetch };
