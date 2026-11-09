// @vitest-environment jsdom

import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { describe, expect, it } from "vitest";

import { AboutPage, type AboutPageProps, TMDB_NOTICE } from "../index";

(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

type El = { click(): void; getAttribute(n: string): string | null; textContent: string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelector(s: string): El | null;
  querySelectorAll(s: string): ArrayLike<El>;
  remove(): void;
  textContent: string | null;
};
const doc = (
  globalThis as unknown as {
    document: { body: { append(n: unknown): void }; createElement(t: string): Host };
  }
).document;

const render = (overrides: Partial<AboutPageProps> = {}) => {
  const props: AboutPageProps = {
    version: "v0.9.3",
    details: [
      { label: "Version", value: "v0.9.3" },
      { label: "Commit", value: "9871fef2ac10" },
      { label: "Status", value: "Ready" },
    ],
    ...overrides,
  };
  const host = doc.createElement("div");
  doc.body.append(host);
  const root = createRoot(host);
  act(() =>
    root.render(
      <LoomarrProvider theme="dark">
        <AboutPage {...props} />
      </LoomarrProvider>,
    ),
  );
  const cleanup = () => {
    act(() => root.unmount());
    host.remove();
  };
  return { host, cleanup };
};

describe("AboutPage", () => {
  it("leads with Loomarr and the version it runs", () => {
    const { host, cleanup } = render();
    expect(host.querySelector('[aria-label="Loomarr"]')?.textContent).toContain("Version v0.9.3");
    cleanup();
  });

  // TMDB's terms require their notice word for word, whatever key is in use (#2853).
  it("names TMDB under Data sources with its required notice, verbatim", () => {
    expect(TMDB_NOTICE).toBe("This product uses the TMDB API but is not endorsed or certified by TMDB.");
    const { host, cleanup } = render();
    const sources = host.querySelector('[aria-label="Data sources"]');
    expect(sources?.textContent).toContain("Data sources");
    expect(sources?.textContent).toContain(TMDB_NOTICE);
    expect(sources?.textContent).toContain("Film and TV details, posters and artwork come from TMDB.");
    cleanup();
  });

  it("shows TMDB's logo when the host supplies it, and the notice without it", () => {
    const withLogo = render({ tmdbLogo: <img alt="TMDB" src="/tmdb.svg" /> });
    expect(withLogo.host.querySelector('[aria-label="Data sources"] img[alt="TMDB"]')).not.toBeNull();
    withLogo.cleanup();
    const without = render();
    expect(without.host.querySelector("img")).toBeNull();
    expect(without.host.textContent).toContain(TMDB_NOTICE);
    without.cleanup();
  });

  // The bug-report facts wait behind one disclosure, like a title's technical details.
  it("keeps the technical details closed until asked", () => {
    const { host, cleanup } = render();
    // Closed, the details are mounted but hidden until found (find-in-page can still reach them).
    const hiddenCommit = () =>
      Array.from(host.querySelectorAll('[hidden="until-found"]')).some((n) =>
        n.textContent?.includes("9871fef2ac10"),
      );
    expect(hiddenCommit()).toBe(true);
    const toggle = Array.from(host.querySelectorAll("[aria-expanded]")).find((n) =>
      n.textContent?.includes("Technical details"),
    );
    if (!toggle) throw new Error("missing Technical details disclosure");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    act(() => toggle.click());
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(hiddenCommit()).toBe(false);
    expect(host.textContent).toContain("9871fef2ac10");
    cleanup();
  });
});
