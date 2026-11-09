// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act, type ReactNode } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, describe, expect, it } from "vitest";

import { SettingsOverview, type SettingsOverviewCard } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// This package has no DOM lib (it targets React Native too), so the slice of the jsdom document
// the test touches is typed here.
type Node = {
  getAttribute(name: string): string | null;
  querySelector(s: string): Node | null;
  querySelectorAll(s: string): ArrayLike<Node>;
  textContent: string | null;
};
type Doc = {
  body: Node & { append(node: unknown): void; replaceChildren(): void };
  createElement(tag: string): Parameters<typeof createRoot>[0];
};
const doc = (globalThis as unknown as { document: Doc }).document;

const mount = async (node: ReactNode) => {
  const container = doc.createElement("div");
  doc.body.append(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>);
  });
};
afterEach(() => {
  doc.body.replaceChildren();
});

const page = (id: string, label: string, note?: string) => ({
  id,
  label,
  note,
  link: <a href={`/${id}`}>{label}</a>,
});

const ai: SettingsOverviewCard = {
  id: "ai",
  title: "AI",
  description: "The service and models behind suggestions and clip checks.",
  status: "ok",
  pages: [page("ai", "AI setup")],
};

const server: SettingsOverviewCard = {
  id: "server",
  title: "Server",
  description: "Playback, storage, backups and what runs in the background.",
  status: "attention",
  problem: { text: "Backups missed 2 nights.", pageLabel: "Backups", link: <a href="/backup">x</a> },
  pages: [
    page("playback", "Playback"),
    page("storage", "Artwork storage"),
    page("backup", "Backups"),
    page("tasks", "Background tasks"),
    page("diagnostics", "Diagnostics"),
    page("database", "Database"),
  ],
};

const headings = () => Array.from(doc.body.querySelectorAll("h2")).map((h) => h.textContent);

describe("SettingsOverview", () => {
  it("puts cards with a problem first and counts them above the cards", async () => {
    await mount(<SettingsOverview cards={[ai, server]} search={null} />);

    expect(headings()).toEqual(["Server", "AI"]);
    expect(doc.body.textContent).toContain("1 thing needs a look.");
  });

  it("states the problem with a link to the page that fixes it", async () => {
    await mount(<SettingsOverview cards={[server]} search={null} />);

    const open = Array.from(doc.body.querySelectorAll("a")).find((a) => a.textContent === "Open Backups");
    expect(open?.getAttribute("href")).toBe("/backup");
    expect(doc.body.textContent).toContain("Backups missed 2 nights.");
    expect(doc.body.textContent).toContain("Needs attention");
    // The description gives way to the problem.
    expect(doc.body.textContent).not.toContain("what runs in the background");
  });

  it("lists the first four pages and links the rest as a count", async () => {
    await mount(<SettingsOverview cards={[server]} search={null} />);

    const region = doc.body.querySelector('[role="region"]');
    const items = region?.querySelectorAll('[role="listitem"]');
    expect(items?.length).toBe(5);
    const more = Array.from(region?.querySelectorAll("a") ?? []).find((a) => a.textContent === "2 more");
    expect(more?.getAttribute("href")).toBe("/playback");
  });

  it("names the page a part lives on", async () => {
    await mount(
      <SettingsOverview
        cards={[
          {
            id: "channels",
            title: "Channels",
            description: "What every channel starts with unless you change it.",
            status: "ok",
            pages: [
              page("defaults", "Channel defaults"),
              page("access", "Your location", "on Sign-in, devices and sharing"),
            ],
          },
        ]}
        search={null}
      />,
    );

    expect(doc.body.textContent).toContain("Your location · on Sign-in, devices and sharing");
  });

  it("says everything is ready, and waits for every check before counting", async () => {
    await mount(<SettingsOverview cards={[ai]} search={null} />);
    expect(doc.body.textContent).toContain("Everything is ready.");

    doc.body.replaceChildren();
    await mount(
      <SettingsOverview cards={[ai, { ...server, problem: undefined, status: undefined }]} search={null} />,
    );
    expect(doc.body.textContent).not.toContain("Everything is ready.");
    expect(doc.body.textContent).not.toContain("a look.");
  });

  it("keeps Advanced as one row under the cards", async () => {
    await mount(
      <SettingsOverview
        advanced={{
          label: "Advanced settings",
          description: "Find and edit a setting by its exact key or environment variable.",
          link: <a href="/settings/advanced">x</a>,
        }}
        cards={[ai]}
        search={<input aria-label="Find a setting" />}
      />,
    );

    const row = Array.from(doc.body.querySelectorAll("a")).find(
      (a) => a.getAttribute("href") === "/settings/advanced",
    );
    expect(row?.textContent).toContain("Advanced settings");
    expect(doc.body.querySelector('input[aria-label="Find a setting"]')).not.toBeNull();
  });
});
