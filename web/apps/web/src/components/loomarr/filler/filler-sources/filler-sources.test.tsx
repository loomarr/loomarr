import type { FillerSourceDTO } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FillerSources } from "./filler-sources";

// Defaults describe a normal, switched-ON source. `enabled` defaulting to false would make
// every test that does not mention it exercise the disabled path by accident.
const source = (over: Partial<FillerSourceDTO> & Pick<FillerSourceDTO, "kind">): FillerSourceDTO => ({
  id: over.kind,
  target: "/data/filler",
  detail: "watched directly",
  count: 0,
  configured: true,
  fetchable: true,
  enabled: true,
  switchable: true,
  removable: false,
  // Defaults to NOT searchable: only archive has an upstream catalog to query, so a default of
  // true would make every test render a search affordance that the real row would not have.
  searchable: false,
  ...over,
});

describe("FillerSources", () => {
  // ⚠ The reason the read-model returns unconfigured rows at all: "no drop-folder configured"
  // is the answer to "why is my catalog empty". Hiding the row leaves that unanswered.
  it("shows an unconfigured source rather than hiding it", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler", configured: false, fetchable: false })]}
        onFetch={vi.fn()}
      />,
    );
    // The row is present (its target renders) AND it is marked, rather than being omitted.
    expect(screen.getByText("/data/filler")).toBeInTheDocument();
    expect(screen.getByText(/not configured/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /fetch now/i })).not.toBeInTheDocument();
  });

  // The total comes from the server, not from summing rows: a clip whose provenance matches no
  // row still belongs to the catalog.
  // ⚠ The header reads "N of M on" (the mock's `svcOnLine`), not "N sources · M clips". An
  // operator who switched two sources off wants that reflected — a bare count where three are
  // dark is a reassuring lie. The catalog total lives in the page header's pill.
  it("reports how many sources are switched on", () => {
    render(
      <FillerSources
        sources={[
          source({ kind: "folder", count: 2 }),
          source({ id: "s2", kind: "archive", enabled: false }),
        ]}
        onFetch={vi.fn()}
      />,
    );
    expect(screen.getByText(/1 of 2 on/)).toBeInTheDocument();
  });

  it("fetches the row that was clicked", async () => {
    const onFetch = vi.fn();
    render(
      <FillerSources sources={[source({ kind: "folder", target: "/data/filler" })]} onFetch={onFetch} />,
    );
    await userEvent.click(screen.getByRole("button", { name: /fetch now from \/data\/filler/i }));
    expect(onFetch).toHaveBeenCalledWith("folder");
  });

  // One fetch at a time: the sync is global, so two in flight would race for the same catalog.
  it("disables every fetch while one is running", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder" }), source({ kind: "library", target: "library" })]}
        onFetch={vi.fn()}
        fetching="folder"
      />,
    );
    for (const b of screen.getAllByRole("button")) {
      expect(b).toBeDisabled();
    }
  });

  // A configured source contributing zero clips is a real, reportable state — usually an empty
  // folder or a mount that did not come up. It must not read the same as "unconfigured".
  it("distinguishes a configured-but-empty source from an unconfigured one", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler", count: 0, configured: true })]}
        onFetch={vi.fn()}
      />,
    );
    expect(screen.getByText("0 clips")).toBeInTheDocument();
    expect(screen.queryByText(/not configured/i)).not.toBeInTheDocument();
  });
});

// --- registered sources as PEER rows (V37; was "remotes nested under the `remote` row", V33) ---

describe("FillerSources registered peers", () => {
  // ⚠ The flat shape: the `remote` CONTAINER row is gone and each collection is a peer carrying
  // its own kind, switch and fetch time. `folder` stays in the list because the assertions below
  // depend on the config row surviving the flattening.
  const flat: FillerSourceDTO[] = [
    source({ kind: "folder", target: "/data/filler", count: 12 }),
    source({
      kind: "archive",
      id: "archive:classic_tv",
      target: "Classic TV commercials",
      detail: "an archive.org collection",
      removable: true,
      searchable: true,
      lastFetchedAt: "2026-07-30T12:00:00Z",
    }),
    source({
      kind: "youtube",
      id: "youtube:vintage_ads",
      target: "vintage_ads",
      detail: "a playlist you added",
      removable: true,
    }),
  ];

  it("lists the specific sources an operator added", () => {
    render(<FillerSources sources={flat} onFetch={() => {}} />);
    expect(screen.getByText("Classic TV commercials")).toBeInTheDocument();
    expect(screen.getByText("vintage_ads")).toBeInTheDocument();
  });

  // ⚠ "never fetched", not an epoch date. A source added but not yet pulled is a real state,
  // and rendering it as 1 Jan 1970 would read as a bug.
  // ⚠ A source that has never been fetched shows its clip count with NO time clause — not
  // "never fetched". Most sources are SCANNED rather than fetched, so an absent timestamp is the
  // ordinary state, and "never" reads as a fault on a folder working exactly as intended. What
  // must never appear is an epoch date nobody meant.
  it("omits the scan time rather than showing a zero date", () => {
    render(<FillerSources sources={flat} onFetch={() => {}} />);
    expect(screen.queryByText(/1970/)).not.toBeInTheDocument();
    expect(screen.queryByText(/never/i)).not.toBeInTheDocument();
  });

  // ⚠ THE property the flattening had to preserve. The config-backed row answers "why is my
  // catalog empty?", which a list of things-that-exist cannot (§10). Flattening deleted the
  // container row; it must not have deleted this.
  it("keeps the config-backed row alongside the peers", () => {
    render(<FillerSources sources={flat} onFetch={() => {}} />);
    expect(screen.getByText("/data/filler")).toBeInTheDocument();
  });

  // A fetch time belongs to a REGISTERED source. The config rows are scanned, not fetched, so
  // "never fetched" under the drop-folder would be a category error rather than a fact.
  it("shows no fetch time on a config-backed row", () => {
    render(<FillerSources sources={[flat[0]!]} onFetch={() => {}} />);
    expect(screen.getByText("/data/filler")).toBeInTheDocument();
    expect(screen.queryByText(/never fetched/)).not.toBeInTheDocument();
  });

  // Removing forgets the REGISTRATION. ⚠ Offered only on a removable row: the server 409s a
  // delete of a config-backed singleton, so a button there would be a control that cannot work.
  it("offers remove on a registered source and not on a config-backed one", async () => {
    const onRemove = vi.fn();
    render(<FillerSources sources={flat} onFetch={() => {}} onRemove={onRemove} />);
    expect(screen.queryByRole("button", { name: /remove \/data\/filler/i })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /remove classic tv commercials/i }));
    expect(onRemove).toHaveBeenCalledWith("archive:classic_tv");
  });
});

// --- the on/off switch (V35) ---

describe("FillerSources switches", () => {
  it("switches a source off through the handler", async () => {
    const onToggleEnabled = vi.fn();
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler" })]}
        onFetch={() => {}}
        onToggleEnabled={onToggleEnabled}
      />,
    );

    await userEvent.click(screen.getByRole("switch", { name: "Use /data/filler" }));

    expect(onToggleEnabled).toHaveBeenCalledWith("folder", false);
  });

  // ⚠ THE promise. A switched-off source keeps its clips, and the row has to say so — an
  // operator who reads "disabled" as "my clips are gone" will switch it back on and re-download
  // everything they already have.
  it("says a switched-off source keeps its clips", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler", enabled: false })]}
        onFetch={() => {}}
        onToggleEnabled={() => {}}
      />,
    );

    // ⚠ The mock marks an off row with a greyed `off` STAT and a dimmer border, not a
    // "switched off" badge — the badge duplicated what the switch already showed. What must
    // survive is the SENTENCE, because an operator who reads "off" as "my clips are gone" will
    // switch it back on and re-download everything they already have.
    expect(screen.getByText("off")).toBeInTheDocument();
    expect(screen.getByText(/still in your catalog/i)).toBeInTheDocument();
  });

  // ⚠ Nothing scans a media-server library for clips since §10 took the media server out of the
  // filler path, so a switch there would change nothing — and the server refuses it with a 409.
  // A control that cannot work is worse than no control.
  it("gives no switch to a row with nothing running behind it", () => {
    render(
      <FillerSources
        sources={[source({ kind: "library", target: "media server filler library", switchable: false })]}
        onFetch={() => {}}
        onToggleEnabled={() => {}}
      />,
    );

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  // A caller that cannot mutate shows the same rows without a dead control.
  it("renders no switches at all without a handler", () => {
    render(
      <FillerSources sources={[source({ kind: "folder", target: "/data/filler" })]} onFetch={() => {}} />,
    );

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });
});
