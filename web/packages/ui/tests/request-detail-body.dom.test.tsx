// @vitest-environment jsdom
import type { RequestEntry } from "@loomarr/core/requests";
import { requestFixtures, requestsSnapshot } from "@loomarr/fixtures";
import { cleanup, renderUi, screen, within } from "@loomarr/test-support";
import { afterEach, describe, expect, it } from "vitest";

import { RequestDetailBody, type RequestDetailBodyProps } from "../index";

afterEach(cleanup);

const { journeys, titles } = requestFixtures;

const entryOf = (key: keyof typeof journeys): RequestEntry => {
  const entry = requestsSnapshot().entries.find((e) => e.journey.jobId === journeys[key].jobId);
  if (!entry) throw new Error(`no fixture entry for ${key}`);
  return entry;
};

const show = (key: keyof typeof journeys, props: Partial<RequestDetailBodyProps> = {}) =>
  renderUi(<RequestDetailBody entry={entryOf(key)} titles={titles} whenText={() => "some day"} {...props} />);

describe("RequestDetailBody", () => {
  it("lists the decision, the lineup and the outstanding titles", () => {
    show("downloading");
    expect(screen.getByText("Decision")).toBeTruthy();
    expect(screen.getByText("Proposed lineup")).toBeTruthy();
    expect(screen.getByText("Titles being added")).toBeTruthy();
  });

  it("dates the approval in the caller's words", () => {
    const seen: string[] = [];
    show("downloading", {
      whenText: (iso) => {
        seen.push(iso);
        return "some day";
      },
    });
    expect(screen.getByText(/Approved by Adela Hale some day/)).toBeTruthy();
    expect(seen).toContain(entryOf("downloading").journey.proposal?.approvedAt);
  });

  it("makes section titles headings only when asked", () => {
    show("downloading");
    expect(screen.queryByRole("heading", { name: "Decision" })).toBeNull();
    cleanup();
    show("downloading", { headingLevel: "h2" });
    expect(screen.getByRole("heading", { level: 2, name: "Decision" })).toBeTruthy();
  });

  it("puts the caller's decision controls inside the decision", () => {
    show("awaitingApproval", { decisionActions: <button type="button">Approve it</button> });
    const decision = screen.getByText("Decision").closest('[role="group"]') as HTMLElement;
    expect(within(decision).getByRole("button", { name: "Approve it" })).toBeTruthy();
  });

  it("lets the caller draw each title's state, control and error", () => {
    show("downloading", {
      titleAction: (title) => <button type="button">{`Act on ${title.name}`}</button>,
      titleBadge: (title) => <span>{`state:${title.state}`}</span>,
      titlesError: <p>retry failed</p>,
    });
    expect(screen.getByText("state:downloading")).toBeTruthy();
    expect(screen.queryByText("Downloading")).toBeNull();
    expect(screen.getAllByRole("button", { name: /^Act on / }).length).toBeGreaterThan(0);
    expect(screen.getByText("retry failed")).toBeTruthy();
  });

  it("replaces the live stage with the caller's while generating", () => {
    show("generating", { progress: <p>my progress</p> });
    expect(screen.getByText("my progress")).toBeTruthy();
    expect(screen.queryByText("Choosing titles…")).toBeNull();
  });

  it("explains a failure and places the caller's actions", () => {
    show("failed", { actions: <button type="button">Fix it</button> });
    expect(screen.getByText("Only 2 titles matched that description.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Fix it" })).toBeTruthy();
  });
});
