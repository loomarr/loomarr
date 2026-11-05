import { LoomarrProvider } from "@loomarr/design-system";
import { QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { Children, isValidElement, type ReactElement, type ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

// The production entry (main.tsx) renders on import. Its root is captured at the createRoot seam
// and inspected as the element tree React would mount.
const mounted = vi.hoisted(() => ({ tree: undefined as unknown }));
vi.mock("react-dom/client", () => ({
  createRoot: () => ({
    render: (tree: unknown) => {
      mounted.tree = tree;
    },
  }),
}));

const childElements = (node: ReactNode): ReactElement<{ children?: ReactNode }>[] =>
  Children.toArray(node).filter(isValidElement) as ReactElement<{ children?: ReactNode }>[];

describe("production application root", () => {
  it("mounts the shared design-system provider, dark, around the query client and the router", async () => {
    document.body.innerHTML = '<div id="root"></div>';
    await import("@/main");

    const [strict] = childElements(mounted.tree as ReactNode);
    const [provider] = childElements(strict?.props.children);
    expect(provider?.type).toBe(LoomarrProvider);
    expect(provider?.props).toMatchObject({ theme: "dark" });

    const [query] = childElements(provider?.props.children);
    expect(query?.type).toBe(QueryClientProvider);
    expect(childElements(query?.props.children).map((el) => el.type)).toContain(RouterProvider);
  });
});
