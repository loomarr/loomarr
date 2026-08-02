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
  ...over,
});

describe("FillerSources", () => {
  // ⚠ The reason the read-model returns unconfigured rows at all: "no drop-folder configured"
  // is the answer to "why is my catalog empty". Hiding the row leaves that unanswered.
  it("shows an unconfigured source rather than hiding it", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler", configured: false, fetchable: false })]}
        total={0}
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
  it("reports the server's total, not the sum of the rows", () => {
    render(<FillerSources sources={[source({ kind: "folder", count: 2 })]} total={5} onFetch={vi.fn()} />);
    expect(screen.getByText(/1 source · 5 clips/)).toBeInTheDocument();
  });

  it("fetches the row that was clicked", async () => {
    const onFetch = vi.fn();
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler" })]}
        total={0}
        onFetch={onFetch}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /fetch now from \/data\/filler/i }));
    expect(onFetch).toHaveBeenCalledWith("folder");
  });

  // One fetch at a time: the sync is global, so two in flight would race for the same catalog.
  it("disables every fetch while one is running", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder" }), source({ kind: "library", target: "library" })]}
        total={0}
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
        total={0}
        onFetch={vi.fn()}
      />,
    );
    expect(screen.getByText("0 clips")).toBeInTheDocument();
    expect(screen.queryByText(/not configured/i)).not.toBeInTheDocument();
  });
});

// --- registered remotes, nested under the `remote` row (V33) ---

describe("FillerSources remotes", () => {
  const withRemotes: FillerSourceDTO[] = [
    {
      id: "remote",
      enabled: true,
      switchable: false,
      removable: false,
      kind: "remote" as const,
      target: "downloads",
      detail: "fetches into the watched folder",
      count: 12,
      configured: true,
      fetchable: false,
      remotes: [
        {
          id: "classic_tv",
          label: "Classic TV commercials",
          uri: "https://archive.org/details/classic_tv",
          lastFetchedAt: "2026-07-30T12:00:00Z",
          enabled: true,
        },
        {
          id: "vintage_ads",
          label: "vintage_ads",
          uri: "https://archive.org/details/vintage_ads",
          enabled: true,
        },
      ],
    },
  ];

  it("lists the specific sources an operator added", () => {
    render(<FillerSources sources={withRemotes} total={12} onFetch={() => {}} />);
    expect(screen.getByText("Classic TV commercials")).toBeInTheDocument();
    expect(screen.getByText("vintage_ads")).toBeInTheDocument();
  });

  // ⚠ "never fetched", not an epoch date. A source added but not yet pulled is a real state,
  // and rendering it as 1 Jan 1970 would read as a bug.
  it("says never rather than showing a zero date", () => {
    render(<FillerSources sources={withRemotes} total={12} onFetch={() => {}} />);
    expect(screen.getByText("never fetched")).toBeInTheDocument();
    expect(screen.queryByText(/1970/)).not.toBeInTheDocument();
  });

  // The three config rows must survive: they answer "why is my catalog empty", which a list of
  // things that exist cannot (build plan §6.1).
  it("keeps the configuration row it nests under", () => {
    render(<FillerSources sources={withRemotes} total={12} onFetch={() => {}} />);
    expect(screen.getByText("downloads")).toBeInTheDocument();
  });

  it("renders nothing extra when no remotes are registered", () => {
    // Declared without `remotes` rather than spreading it as undefined: the spread widens
    // every field to optional and stops matching the generated DTO.
    const bare = [
      {
        id: "remote",
        kind: "remote" as const,
        target: "downloads",
        detail: "fetches into the watched folder",
        count: 12,
        configured: true,
        fetchable: false,
        enabled: true,
        switchable: false,
        removable: false,
      },
    ];
    render(<FillerSources sources={bare} total={12} onFetch={() => {}} />);
    expect(screen.getByText("downloads")).toBeInTheDocument();
    expect(screen.queryByText(/never fetched/)).not.toBeInTheDocument();
  });
});

// --- the on/off switch (V35) ---

describe("FillerSources switches", () => {
  it("switches a source off through the handler", async () => {
    const onToggleEnabled = vi.fn();
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler" })]}
        total={1}
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
        total={1}
        onFetch={() => {}}
        onToggleEnabled={() => {}}
      />,
    );

    expect(screen.getByText("switched off")).toBeInTheDocument();
    expect(screen.getByText(/still in your catalog/i)).toBeInTheDocument();
  });

  // ⚠ Nothing scans a media-server library for clips since §10 took the media server out of the
  // filler path, so a switch there would change nothing — and the server refuses it with a 409.
  // A control that cannot work is worse than no control.
  it("gives no switch to a row with nothing running behind it", () => {
    render(
      <FillerSources
        sources={[source({ kind: "library", target: "media server filler library", switchable: false })]}
        total={0}
        onFetch={() => {}}
        onToggleEnabled={() => {}}
      />,
    );

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  // A caller that cannot mutate shows the same rows without a dead control.
  it("renders no switches at all without a handler", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler" })]}
        total={1}
        onFetch={() => {}}
      />,
    );

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });
});
