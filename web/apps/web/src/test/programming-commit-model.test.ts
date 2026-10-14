import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const here = dirname(fileURLToPath(import.meta.url));
const src = (rel: string) => readFileSync(join(here, "..", rel), "utf8");

const PROGRAMMING = "routes/_authed/channels/$id/-channel-programming.tsx";

// design.md §12 names the review-before-apply surfaces, and scheduling RULES are the third —
// the only block of the Programming page that drafts. Rules resolve first-match-by-priority,
// so a rule's effect depends on every rule above it and each intermediate state is a different
// schedule; inline-saving each step would reconcile half-finished rule sets to Tunarr.
//
// ⚠ **This is a SOURCE-level check, and deliberately, for the same reason
// settings-inline-commit.test.ts is one.** The property is architectural — "the rules editor
// reads the draft, everything else reads the saved policy" — and the hook's own tests cannot
// see it. Sabotaging the page to wire `ChannelRulesEditor` back to `onPolicyChange` left all
// 278 cross-cutting tests green, which is the "built, tested, and mounted by nothing" shape
// this repo has been bitten by seven times.
describe("the Programming surface's commit model", () => {
  it("wires the rules editor to the DRAFT, not the saved policy", () => {
    const s = src(PROGRAMMING);
    // The editor's props come from the draft hook…
    expect(s).toMatch(/<ChannelRulesEditor[\s\S]{0,200}policy=\{rules\.draft\}/);
    expect(s).toMatch(/<ChannelRulesEditor[\s\S]{0,200}onChange=\{rules\.setDraft\}/);
  });

  it("offers Apply and Discard, gated on the draft being dirty", () => {
    const s = src(PROGRAMMING);
    // Without this the draft would be unreachable: edits would accumulate with no way to ship
    // them, which is worse than inline-saving.
    expect(s).toContain("rules.isDirty");
    expect(s).toContain("rules.apply");
    expect(s).toContain("rules.discard");
  });

  // ⚠ The CONTROL case, so the assertions above test a real distinction rather than "this file
  // mentions a draft somewhere". §12 scopes the exception to the interdependent surface: every
  // other block on this page is self-contained and stays seamless. A change that swept them all
  // into the draft "for consistency" would restore the ceremony §12's first version removed.
  const seamless = [
    ["scope + era/ceiling", /<ChannelPolicyFields[\s\S]{0,120}show="scope"/],
    ["ordering", /<ChannelPolicyFields[\s\S]{0,200}show="ordering"/],
    ["series scope", /<ChannelSeriesScope[\s\S]{0,120}onChange=\{onPolicyChange\}/],
    ["seasonal", /<ChannelSeasonal[\s\S]{0,200}onChange=\{onPolicyChange\}/],
    ["auto-curate", /<ChannelAutoCurate[\s\S]{0,200}onChange=\{onPolicyChange\}/],
  ] as const;

  for (const [what, pattern] of seamless) {
    it(`${what} stays seamless — it saves inline, not through the draft`, () => {
      expect(src(PROGRAMMING)).toMatch(pattern);
    });
  }

  it("the preview follows the draft while it is dirty", () => {
    // The whole point of drafting rules is seeing what they do BEFORE they ship. A draft with a
    // preview still showing saved state would be strictly worse than inline-saving: the operator
    // would stage a change and watch a pane that disagrees with it.
    expect(src(PROGRAMMING)).toMatch(/draftPolicy=\{rules\.isDirty \? rules\.draft : undefined\}/);
  });
});
