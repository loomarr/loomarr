import type { ReactElement, ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ render: vi.fn(), toaster: vi.fn() }));

vi.mock("@/diagnostics/client-reporter", () => ({
  clientDiagnostics: { record: vi.fn() },
  installGlobalClientDiagnostics: vi.fn(),
}));
vi.mock("@/routeTree.gen", () => ({ routeTree: {} }));
// The entry watches the stand-in query client for a session that ends; that is session-expiry's test.
vi.mock("@/auth/session-expiry", () => ({ watchSessionExpiry: vi.fn() }));
vi.mock("@/components/loomarr/feedback/route-error", () => ({ RouteError: () => null }));
vi.mock("@/components/loomarr/feedback/route-pending", () => ({ RoutePending: () => null }));
vi.mock("@/components/loomarr/feedback/app-toaster", () => ({ AppToaster: mocks.toaster }));
vi.mock("@loomarr/design-system", () => ({
  LoomarrProvider: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("@tanstack/react-query", () => ({
  QueryClient: class QueryClient {},
  QueryClientProvider: ({ children }: { children: ReactNode }) => children,
}));
vi.mock("@tanstack/react-router", () => ({
  // The entry subscribes route timing to the router, so the stand-in can be subscribed to.
  createRouter: vi.fn(() => ({ subscribe: () => () => {} })),
  RouterProvider: () => null,
}));
vi.mock("react-dom/client", () => ({ createRoot: () => ({ render: mocks.render }) }));

const findToaster = (node: ReactNode): ReactElement | undefined => {
  if (!node || typeof node !== "object") return undefined;
  const element = node as ReactElement<{ children?: ReactNode }>;
  if (element.type === mocks.toaster) return element;
  const children = element.props?.children;
  return Array.isArray(children)
    ? children.map(findToaster).find((child) => child !== undefined)
    : findToaster(children);
};

describe("startup notice layout", () => {
  beforeEach(() => {
    vi.resetModules();
    mocks.render.mockReset();
  });

  // Where a notice sits on each width is AppToaster's own test; the entry mounts that one region.
  it("mounts the app's toast region beside the routes", async () => {
    await import("@/main");

    const app = mocks.render.mock.calls[0]?.[0] as ReactNode;
    expect(findToaster(app)).toBeDefined();
  });
});
