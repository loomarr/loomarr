// @vitest-environment jsdom

import { act, cleanup, renderUi } from "@loomarr/test-support";
import { afterEach, describe, expect, it } from "vitest";

import { AboutPage, type AboutPageProps, TMDB_NOTICE } from "../index";

afterEach(cleanup);

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
  const { container: host, unmount } = renderUi(<AboutPage {...props} />);
  return { host, cleanup: unmount };
};

describe("AboutPage", () => {
  it("leads with Loomarr and the version it runs", () => {
    const { host } = render();
    expect(host.querySelector('[aria-label="Loomarr"]')?.textContent).toContain("Version v0.9.3");
  });

  // TMDB's terms require their notice word for word, whatever key is in use (#2853).
  it("names TMDB under Data sources with its required notice, verbatim", () => {
    expect(TMDB_NOTICE).toBe("This product uses the TMDB API but is not endorsed or certified by TMDB.");
    const { host } = render();
    const sources = host.querySelector('[aria-label="Data sources"]');
    expect(sources?.textContent).toContain("Data sources");
    expect(sources?.textContent).toContain(TMDB_NOTICE);
    expect(sources?.textContent).toContain("Film and TV details, posters and artwork come from TMDB.");
  });

  it("shows TMDB's logo when the host supplies it, and the notice without it", () => {
    const withLogo = render({ tmdbLogo: <img alt="TMDB" src="/tmdb.svg" /> });
    expect(withLogo.host.querySelector('[aria-label="Data sources"] img[alt="TMDB"]')).not.toBeNull();
    withLogo.cleanup();
    const without = render();
    expect(without.host.querySelector("img")).toBeNull();
    expect(without.host.textContent).toContain(TMDB_NOTICE);
  });

  // The demo pack's credits live in each title's Info; About says where the sources are (#3409).
  it("says where the demo's sources are listed only on the demo site", () => {
    const demo = render({ demo: true });
    const sources = demo.host.querySelector('[aria-label="Data sources"]')?.textContent;
    expect(sources).toContain("The demo's films, cartoons and ads are openly licensed.");
    expect(sources).toContain("Demo content page");
    demo.cleanup();
    const install = render();
    expect(install.host.textContent).not.toContain("openly licensed");
  });

  // The bug-report facts wait behind one disclosure, like a title's technical details.
  it("keeps the technical details closed until asked", () => {
    const { host } = render();
    // Closed, the details are mounted but hidden until found (find-in-page can still reach them).
    const hiddenCommit = () =>
      Array.from(host.querySelectorAll('[hidden="until-found"]')).some((n) =>
        n.textContent?.includes("9871fef2ac10"),
      );
    expect(hiddenCommit()).toBe(true);
    const toggle = Array.from(host.querySelectorAll<HTMLElement>("[aria-expanded]")).find((n) =>
      n.textContent?.includes("Technical details"),
    );
    if (!toggle) throw new Error("missing Technical details disclosure");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    act(() => toggle.click());
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(hiddenCommit()).toBe(false);
    expect(host.textContent).toContain("9871fef2ac10");
  });
});
