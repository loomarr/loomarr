import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { anchorOf, parseDocHref } from "./anchor";

// Every deep-link the API emits on a setup check (internal/api/setup.go). Duplicated here
// deliberately — the point is to prove the FE's slugger agrees with the BE's, and
// importing one side's source of truth would only prove it agrees with itself.
//
// docs/embed_test.go asserts these SAME hrefs resolve using the GO slugger. Both tests
// read the same markdown, so if the two implementations ever diverge, one of them fails.
const DOC_HREFS = [
  "troubleshooting#media-server",
  "troubleshooting#seerr",
  "troubleshooting#tunarr",
  "troubleshooting#tunarr-library",
  "troubleshooting#llm",
  "troubleshooting#tmdb",
  "troubleshooting#filler",
  "troubleshooting#livetv",
  "troubleshooting#webhooks",
];

const repoRoot = join(dirname(fileURLToPath(import.meta.url)), "../../../../..");

const headingsOf = (markdown: string): string[] =>
  markdown
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => /^#{1,6}\s/.test(line))
    .map((line) => line.replace(/^#+\s*/, ""));

describe("anchorOf", () => {
  it("matches GitHub-style slugs", () => {
    expect(anchorOf("Media server")).toBe("media-server");
    expect(anchorOf("Tunarr library")).toBe("tunarr-library");
    expect(anchorOf("LiveTV")).toBe("livetv");
    expect(anchorOf("LLM")).toBe("llm");
    expect(anchorOf("Webhooks")).toBe("webhooks");
  });

  it("collapses runs of punctuation and drops a trailing hyphen", () => {
    expect(anchorOf("Trailing dash — ")).toBe("trailing-dash");
    expect(anchorOf("A -- B")).toBe("a-b");
  });

  // The cross-language guard: the FE renders heading ids with THIS function, while the Go
  // side validated the same anchors with ITS function. A dangling deep-link is worse than
  // none — it promises help and delivers a blank page at the moment the operator is stuck.
  it("resolves every docHref the API emits, against the real troubleshooting page", () => {
    const markdown = readFileSync(join(repoRoot, "docs/help/troubleshooting.md"), "utf8");
    const anchors = new Set(headingsOf(markdown).map(anchorOf));

    for (const href of DOC_HREFS) {
      const [slug, fragment] = href.split("#");
      expect(slug, "every docHref targets the troubleshooting page today").toBe("troubleshooting");
      expect(
        anchors.has(fragment as string),
        `${href} has no heading anchoring to "${fragment}". Available: ${[...anchors].join(", ")}`,
      ).toBe(true);
    }
  });
});

describe("parseDocHref", () => {
  it("splits an API deep-link into page + section", () => {
    expect(parseDocHref("troubleshooting#tunarr")).toEqual({ page: "troubleshooting", section: "tunarr" });
  });
  it("handles a page with no section", () => {
    expect(parseDocHref("concepts")).toEqual({ page: "concepts", section: undefined });
  });
  it("treats a leading # as a same-page anchor (no page)", () => {
    expect(parseDocHref("#approval-the-one-gate")).toEqual({ section: "approval-the-one-gate" });
  });
  it("keeps a hyphenated slug and anchor intact", () => {
    expect(parseDocHref("member-guide#reading-channel-status")).toEqual({
      page: "member-guide",
      section: "reading-channel-status",
    });
  });
});
