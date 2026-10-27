// @vitest-environment jsdom
import type { FillerDefectReportDTO } from "@loomarr/core/filler-defects";
import { LoomarrProvider } from "@loomarr/design-system";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DefectReview } from "../index";

// The package type-checks without the DOM lib, so the test names only the node surface it reads.
type Node = { click(): void; textContent: string | null; getAttribute(name: string): string | null };
type Host = Parameters<typeof createRoot>[0] & {
  querySelectorAll(selector: string): ArrayLike<Node>;
  remove(): void;
  textContent: string | null;
};
const testDocument = (
  globalThis as unknown as {
    document: { body: { append(node: unknown): void }; createElement(tagName: string): Host };
  }
).document;

const report = (over: Partial<FillerDefectReportDTO> = {}): FillerDefectReportDTO => ({
  floorVersion: "floor-v3-provisional",
  clips: 210,
  unmeasured: 3,
  labelled: 12,
  verdicts: { pass: 180, fail: 23, undecided: 0, pending: 4 },
  scores: [
    {
      band: "tape",
      precision: 1,
      caught: 0.83,
      failed: 5,
      falseFails: 0,
      judged: 0,
      missed: 1,
      borderlinePass: 2,
      judgeRight: 3,
      judgeWrong: 1,
      total: 12,
    },
  ],
  total: 41,
  rows: [
    {
      clipId: "c1",
      name: "Sunbeam Juice",
      era: 1988,
      band: "tape",
      outcome: "borderline",
      verdict: "pass",
      verdictReason: "judge",
      findings: [
        { check: "blocks", outcome: "pass", measured: 1.1, bar: 4 },
        { check: "blur", outcome: "borderline", measured: 8.7, bar: 8 },
      ],
      judgement: { ruling: "pass", reason: "soft VHS", model: "m1" },
      measuredAt: "2026-10-27T12:00:00Z",
    },
  ],
  ...over,
});

describe("DefectReview", () => {
  let host: Host;
  let root: Root;
  const mount = (node: React.ReactNode) =>
    act(() => root.render(<LoomarrProvider theme="dark">{node}</LoomarrProvider>));
  const button = (name: string) => {
    const found = Array.from(host.querySelectorAll('[role="button"]')).find((b) => b.textContent === name);
    if (!found) throw new Error(`no ${name} button`);
    return found;
  };

  beforeEach(() => {
    vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
    host = testDocument.createElement("div");
    testDocument.body.append(host);
    root = createRoot(host);
  });

  afterEach(() => {
    act(() => root.unmount());
    host.remove();
    vi.unstubAllGlobals();
  });

  it("shows the standing, the scores and what the floor found on the clip to label", () => {
    mount(
      <DefectReview onLabel={() => {}} onSkip={() => {}} player={<span>player</span>} report={report()} />,
    );
    const text = host.textContent ?? "";
    expect(text).toContain("Would retire 23 · pass 180 · pending 4 · 3 clips not measured yet");
    expect(text).toContain("100%");
    expect(text).toContain("83%");
    expect(text).toContain("3 of 4");
    expect(text).toContain("Sunbeam Juice");
    expect(text).toContain("1988 · Tape era");
    expect(text).toContain("Softness 8.7, borderline (bar 8)");
    expect(text).not.toContain("Compression blocks");
    expect(text).toContain("Judge (m1): pass, “soft VHS”");
    expect(text).toContain("41 clips left to label");
    expect(text).toContain("player");
    expect(Array.from(host.querySelectorAll('[role="row"]'))).toHaveLength(2);
  });

  it("labels the clip it shows, and holds still while a label saves", () => {
    const onLabel = vi.fn();
    const onSkip = vi.fn();
    mount(<DefectReview onLabel={onLabel} onSkip={onSkip} player={null} report={report()} />);
    act(() => button("Fail").click());
    act(() => button("Skip").click());
    expect(onLabel).toHaveBeenCalledWith("c1", "fail");
    expect(onSkip).toHaveBeenCalledTimes(1);

    mount(<DefectReview busy onLabel={onLabel} onSkip={onSkip} player={null} report={report()} />);
    act(() => button("Pass").click());
    expect(onLabel).toHaveBeenCalledTimes(1);
  });

  it("says when nothing is left to label, and when nothing is labelled yet", () => {
    mount(
      <DefectReview
        onLabel={() => {}}
        onSkip={() => {}}
        player={null}
        report={report({ rows: [], scores: [], total: 0 })}
      />,
    );
    const text = host.textContent ?? "";
    expect(text).toContain("Every measured clip is labelled");
    expect(text).toContain("No clips labelled yet");
  });

  it("says a label was given for an era the clip no longer has", () => {
    const base = report();
    const [first] = base.rows;
    if (!first) throw new Error("the fixture has a row");
    const row = { ...first, label: { label: "fail" as const, era: 2012, current: false, labelledAt: "" } };
    mount(
      <DefectReview onLabel={() => {}} onSkip={() => {}} player={null} report={{ ...base, rows: [row] }} />,
    );
    expect(host.textContent).toContain("You labelled it fail for 2012; its era has changed since.");
  });
});
