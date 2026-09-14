import type { FillerSourceDTO } from "@loomarr/api";
import { render, screen, within } from "@testing-library/react";
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
  incoming: 0,
  configured: true,
  fetchable: true,
  enabled: true,
  effectiveEnabled: over.effectiveEnabled ?? over.enabled !== false,
  providerEnabled: true,
  switchable: true,
  removable: false,
  // Defaults to NOT searchable: only archive has an upstream catalog to query, so a default of
  // true would make every test render a search affordance that the real row would not have.
  searchable: false,
  readiness:
    over.readiness ??
    (over.configured === false ? "not_configured" : over.enabled === false ? "off" : "ready"),
  ready: over.ready ?? (over.configured !== false && over.enabled !== false),
  locationSource: "installation",
  actions:
    over.actions ??
    (over.configured === false
      ? ["configure"]
      : over.enabled === false
        ? ["enable"]
        : [
            "fetch",
            "disable",
            ...(over.removable ? ["remove"] : []),
            ...(over.searchable ? ["search"] : []),
            ...(over.kind === "archive" || over.kind === "youtube" ? ["edit_location"] : []),
          ]),
  ...over,
});

describe("FillerSources", () => {
  it("keeps the source section in the page heading hierarchy", () => {
    render(<FillerSources sources={[]} />);

    expect(screen.getByRole("heading", { level: 2, name: "Where filler comes from" })).toBeInTheDocument();
  });

  // ⚠ The reason the read-model returns unconfigured rows at all: "no drop-folder configured"
  // is the answer to "why is my catalog empty". Hiding the row leaves that unanswered.
  it("shows an unconfigured source rather than hiding it", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler", configured: false, fetchable: false })]}
      />,
    );
    // The row is present and explains what to add, rather than showing a raw placeholder path.
    expect(screen.getByText("Drop folder")).toBeInTheDocument();
    expect(screen.getByText("Choose a folder for files you add yourself.")).toBeInTheDocument();
    expect(screen.getByText("Add details")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /fetch now/i })).not.toBeInTheDocument();
  });

  // The total comes from the server, not from summing rows: a clip whose provenance matches no
  // row still belongs to the catalog.
  // ⚠ The header reads "N of M on" (the mock's `svcOnLine`), not "N sources · M clips". An
  // operator who switched two sources off wants that reflected — a bare count where three are
  // dark is a reassuring lie. The catalog total lives in the page header's pill.
  it("reports how many sources are ready", () => {
    render(
      <FillerSources
        sources={[
          source({ kind: "folder", count: 2 }),
          source({
            id: "s2",
            kind: "archive",
            enabled: false,
            ready: false,
            readiness: "off",
            actions: ["enable"],
          }),
        ]}
      />,
    );
    expect(screen.getByText(/1 of 2 ready/)).toBeInTheDocument();
  });

  it("opens the workspace for the row that was clicked", async () => {
    const onSelect = vi.fn();
    render(
      <FillerSources sources={[source({ kind: "folder", target: "/data/filler" })]} onSelect={onSelect} />,
    );
    await userEvent.click(screen.getByRole("button", { name: /manage drop folder/i }));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: "folder" }));
  });

  // A configured source contributing zero clips is a real, reportable state — usually an empty
  // folder or a mount that did not come up. It must not read the same as "unconfigured".
  it("distinguishes a configured-but-empty source from an unconfigured one", () => {
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler", count: 0, configured: true })]}
      />,
    );
    expect(screen.getByText("0 clips")).toBeInTheDocument();
    expect(screen.queryByText(/not configured/i)).not.toBeInTheDocument();
  });

  it("shows held clips as being checked instead of making a working source look empty", () => {
    render(
      <FillerSources sources={[source({ kind: "archive", target: "TV Ads", count: 0, incoming: 22 })]} />,
    );

    expect(screen.getByText("0 ready · 22 being checked")).toBeInTheDocument();
    expect(screen.queryByText("0 clips")).not.toBeInTheDocument();
  });

  it("keeps inherited location quiet and shows one direct fix when the installation has none", () => {
    const { rerender } = render(
      <FillerSources
        sources={[
          source({
            kind: "archive",
            effectiveCountry: "US",
            effectiveMarket: "New York",
          }),
        ]}
      />,
    );
    expect(screen.queryByText(/Uses your location/)).not.toBeInTheDocument();

    rerender(
      <FillerSources
        sources={[
          source({
            kind: "archive",
            readiness: "needs_location",
            ready: false,
            locationSource: "missing",
            actions: ["set_location"],
          }),
        ]}
      />,
    );
    expect(screen.getByText("Add your location")).toBeInTheDocument();
    expect(screen.queryByText("Location needed")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Set location" })).toHaveAttribute("href", "/settings/general");
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
      lastCheckedAt: "2026-07-30T12:00:00Z",
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
    render(<FillerSources sources={flat} />);
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
    render(<FillerSources sources={flat} />);
    expect(screen.queryByText(/1970/)).not.toBeInTheDocument();
    expect(screen.queryByText(/never/i)).not.toBeInTheDocument();
  });

  // ⚠ THE property the flattening had to preserve. The config-backed row answers "why is my
  // catalog empty?", which a list of things-that-exist cannot (§10). Flattening deleted the
  // container row; it must not have deleted this.
  it("keeps the config-backed row alongside the peers", () => {
    render(<FillerSources sources={flat} />);
    expect(screen.getByText("/data/filler")).toBeInTheDocument();
  });

  // A fetch time belongs to a REGISTERED source. The config rows are scanned, not fetched, so
  // "never fetched" under the drop-folder would be a category error rather than a fact.
  it("shows no fetch time on a config-backed row", () => {
    render(<FillerSources sources={[flat[0]!]} />);
    expect(screen.getByText("/data/filler")).toBeInTheDocument();
    expect(screen.queryByText(/never fetched/)).not.toBeInTheDocument();
  });

  it("keeps source actions out of the compact index", () => {
    render(<FillerSources sources={flat} onSelect={vi.fn()} />);
    expect(screen.queryByRole("button", { name: /check now/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /remove/i })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /manage classic tv commercials/i })).toBeInTheDocument();
  });
});

// --- the on/off switch (V35) ---

describe("FillerSources switches", () => {
  it("switches a source off through the handler", async () => {
    const onToggleEnabled = vi.fn();
    render(
      <FillerSources
        sources={[source({ kind: "folder", target: "/data/filler" })]}
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
        onToggleEnabled={() => {}}
      />,
    );

    // ⚠ The mock marks an off row with a greyed `off` STAT and a dimmer border, not a
    // "switched off" badge — the badge duplicated what the switch already showed. What must
    // survive is the SENTENCE, because an operator who reads "off" as "my clips are gone" will
    // switch it back on and re-download everything they already have.
    expect(screen.getByText(/^off$/i)).toBeInTheDocument();
    expect(screen.getByText(/existing clips stay/i)).toBeInTheDocument();
  });

  // ⚠ Nothing scans a media-server library for clips since §10 took the media server out of the
  // filler path, so a switch there would change nothing — and the server refuses it with a 409.
  // A control that cannot work is worse than no control.
  it("gives no switch to a row with nothing running behind it", () => {
    render(
      <FillerSources
        sources={[source({ kind: "library", target: "media server filler library", switchable: false })]}
        onToggleEnabled={() => {}}
      />,
    );

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });

  // A caller that cannot mutate shows the same rows without a dead control.
  it("renders no switches at all without a handler", () => {
    render(<FillerSources sources={[source({ kind: "folder", target: "/data/filler" })]} />);

    expect(screen.queryByRole("switch")).not.toBeInTheDocument();
  });
});

// The PROVIDER ROLL-UP (§10 V51c), rendered for the first time in V54 phase B.
//
// ⚠ **`GET /v1/filler/sources` has returned `group`/`parentId` since PR #201 and NOTHING rendered
// them** — the tab drew one flat list, so three archive collections read as three unrelated
// services. Every test in this file was green over the flat V37 shape the whole time, because no
// fixture anywhere carried a group row. These exist so that cannot happen twice.
describe("FillerSources provider roll-up", () => {
  // Pre-ordered exactly as the server sends it: each group node immediately followed by its own
  // children. The nesting is the component's; the wire is flat.
  const grouped = (over: Partial<FillerSourceDTO> = {}): FillerSourceDTO[] => [
    source({ kind: "folder", id: "folder", target: "Drop folder", count: 9 }),
    {
      ...source({ kind: "archive", id: "provider:archive", target: "Archive.org" }),
      group: true,
      switchable: true,
      removable: false,
      fetchable: false,
      searchable: false,
      count: 179,
      lastCheckedAt: "2026-07-30T09:14:00Z",
      ...over,
    },
    source({
      kind: "archive",
      id: "archive:classic",
      parentId: "provider:archive",
      target: "Classic TV Commercials",
      count: 137,
      searchable: true,
    }),
    source({
      kind: "archive",
      id: "archive:psas",
      parentId: "provider:archive",
      target: "Vintage PSAs",
      count: 42,
      enabled: false,
      searchable: true,
    }),
  ];

  it("nests a provider's sources under it, in their own list", () => {
    render(<FillerSources sources={grouped()} />);

    // ⚠ The children are in a list OF THEIR OWN, not loose among the top-level rows. A screen
    // reader then announces "list, 2 items" for what is under this service instead of walking one
    // flat list of unrelated peers.
    const nested = screen.getByRole("list", { name: /sources under Archive\.org/i });
    expect(nested).toBeInTheDocument();
    expect(screen.getByText("Classic TV Commercials")).toBeInTheDocument();
    expect(screen.getByText("Vintage PSAs")).toBeInTheDocument();
  });

  it("keeps provider sources visible without another disclosure control", () => {
    render(<FillerSources sources={grouped()} />);

    expect(screen.getByText("Classic TV Commercials")).toBeVisible();
    expect(
      screen.queryByRole("button", { name: /show the sources under Archive\.org/i }),
    ).not.toBeInTheDocument();
  });

  // ⚠ THE denominator bug (§10 V54 B2). The derived node is a summary of rows already in this
  // list, so counting it showed "3 of 4 on" beside a page-header pill saying "2 of 3" — two
  // summaries of one list disagreeing on screen.
  it("counts sources, not the provider rows that summarise them", () => {
    render(<FillerSources sources={grouped()} />);
    // folder (on) + classic (on) + psas (off) = 2 of 3. NOT 3 of 4.
    expect(screen.getByText(/2 of 3 ready/)).toBeInTheDocument();
  });

  it("summarises how many sources were added", () => {
    render(<FillerSources sources={grouped()} />);
    expect(screen.getByText(/2 sources/)).toBeInTheDocument();
    expect(screen.queryByText(/needs attention/i)).not.toBeInTheDocument();
  });

  it("counts an actionable source problem as attention", () => {
    const sources = grouped().map((item) =>
      item.id === "archive:classic" ? { ...item, readiness: "unavailable" as const, ready: false } : item,
    );
    render(<FillerSources sources={sources} />);

    expect(screen.getByText("2 sources · 1 needs attention")).toBeInTheDocument();
  });

  it("reads as paused when its master switch is off", () => {
    const sources = grouped({ enabled: false, effectiveEnabled: false }).map((s) =>
      s.parentId
        ? { ...s, effectiveEnabled: false, providerEnabled: false, readiness: "provider_off" as const }
        : s,
    );
    render(<FillerSources sources={sources} />);

    expect(screen.getByText(/Archive\.org is paused/i)).toBeInTheDocument();
  });

  it("folds and disables child sources without changing their saved switches", () => {
    const paused = grouped({ enabled: false, effectiveEnabled: false }).map((item) =>
      item.parentId
        ? {
            ...item,
            effectiveEnabled: false,
            providerEnabled: false,
            readiness: "provider_off" as const,
          }
        : item,
    );
    const { rerender } = render(
      <FillerSources sources={paused} onToggleEnabled={vi.fn()} onToggleProvider={vi.fn()} />,
    );

    const classic = screen.getByRole("switch", { name: "Use Classic TV Commercials", hidden: true });
    const psas = screen.getByRole("switch", { name: "Use Vintage PSAs", hidden: true });
    expect(classic).toBeDisabled();
    expect(psas).toBeDisabled();
    expect(screen.getByRole("list", { name: "Sources under Archive.org", hidden: true })).not.toBeVisible();

    rerender(<FillerSources sources={grouped()} onToggleEnabled={vi.fn()} onToggleProvider={vi.fn()} />);

    expect(screen.getByRole("switch", { name: "Use Classic TV Commercials" })).toBeChecked();
    expect(screen.getByRole("switch", { name: "Use Vintage PSAs" })).not.toBeChecked();
    expect(screen.getByRole("list", { name: "Sources under Archive.org" })).toBeVisible();
  });

  it("starts folding children while the parent off request is pending", () => {
    render(
      <FillerSources
        sources={grouped()}
        onToggleEnabled={vi.fn()}
        onToggleProvider={vi.fn()}
        togglingProvider="archive"
      />,
    );

    expect(screen.getByRole("switch", { name: "Use Classic TV Commercials", hidden: true })).toBeDisabled();
    expect(screen.getByRole("list", { name: "Sources under Archive.org", hidden: true })).not.toBeVisible();
  });

  it("does not repeat provider context with type chips", () => {
    render(<FillerSources sources={grouped()} />);
    expect(screen.queryByText("SERVICE")).not.toBeInTheDocument();
    expect(screen.queryByText("ARCHIVE")).not.toBeInTheDocument();
  });

  // ⚠ A provider with nothing under it is an INVITATION (§10, store/fillersources.go), and it
  // rendered the same red `not configured` caution Badge a broken drop-folder gets — telling an
  // operator something is wrong when nothing is.
  it("invites rather than faults an empty provider", () => {
    render(
      <FillerSources
        sources={[
          source({ kind: "folder", id: "folder", target: "Drop folder" }),
          {
            ...source({ kind: "archive", id: "provider:archive", target: "Archive.org" }),
            group: true,
            configured: false,
            enabled: true,
            effectiveEnabled: true,
            switchable: true,
            removable: false,
            fetchable: false,
            searchable: false,
            count: 0,
          },
        ]}
      />,
    );

    expect(screen.getByText(/nothing added yet/i)).toBeInTheDocument();
    expect(screen.getByText(/^nothing added$/i)).toBeInTheDocument();
    // The caution Badge belongs to a source that is genuinely misconfigured, never to a service
    // nobody has added anything to yet.
    expect(screen.queryByText(/^not configured$/i)).not.toBeInTheDocument();
  });

  it("switches provider policy without calling the child-source handler", async () => {
    const onToggleProvider = vi.fn();
    const onToggleEnabled = vi.fn();
    render(
      <FillerSources
        sources={grouped()}
        onToggleEnabled={onToggleEnabled}
        onToggleProvider={onToggleProvider}
      />,
    );

    await userEvent.click(screen.getByRole("switch", { name: /use Archive\.org/i }));
    expect(onToggleProvider).toHaveBeenCalledWith("archive", false);
    expect(onToggleEnabled).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: /fetch now from Archive\.org/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /remove Archive\.org/i })).not.toBeInTheDocument();
  });

  it("puts provider-specific setup inside the provider", () => {
    render(
      <FillerSources
        sources={grouped()}
        renderProviderSetup={(provider) => <button type="button">Add to {provider.target}</button>}
      />,
    );
    expect(screen.getByRole("button", { name: "Add to Archive.org" })).toBeInTheDocument();
  });

  it("puts local setup inside Your files before the remote providers", () => {
    const remoteOnly = grouped().filter((item) => item.id !== "folder");
    render(
      <FillerSources
        sources={remoteOnly}
        renderLocalSetup={<button type="button">Add a folder or library</button>}
      />,
    );

    const local = screen.getByRole("region", { name: "Your files" });
    const archive = screen.getByRole("region", { name: "Archive.org" });
    expect(within(local).getByRole("button", { name: "Add a folder or library" })).toBeInTheDocument();
    expect(within(local).getByText("Nothing added")).toBeInTheDocument();
    expect(local.compareDocumentPosition(archive) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("keeps paused sources visible without calling them attention in a long list", async () => {
    const provider = {
      ...source({ kind: "archive", id: "provider:archive", target: "Archive.org" }),
      group: true,
    };
    const children = Array.from({ length: 20 }, (_, index) =>
      source({
        kind: "archive",
        id: `archive:${index + 1}`,
        parentId: provider.id,
        target: index === 12 ? "Paused collection" : `Healthy collection ${index + 1}`,
        enabled: index !== 12,
        effectiveEnabled: index !== 12,
        ready: index !== 12,
        readiness: index === 12 ? "off" : "ready",
      }),
    );

    render(<FillerSources sources={[provider, ...children]} onSelect={vi.fn()} />);

    expect(screen.getByText("20 sources", { exact: true })).toBeInTheDocument();
    expect(screen.queryByText(/needs attention/i)).not.toBeInTheDocument();
    expect(screen.getByText("Paused collection")).toBeInTheDocument();
    expect(screen.getByText("Healthy collection 1")).toBeInTheDocument();
    expect(screen.getByText("Healthy collection 5")).toBeInTheDocument();
    expect(screen.queryByText("Healthy collection 6")).not.toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: /^Manage / })).toHaveLength(6);

    await userEvent.click(screen.getByRole("button", { name: "Show 14 more under Archive.org" }));

    expect(screen.getAllByRole("button", { name: /^Manage / })).toHaveLength(20);
    expect(screen.getByRole("button", { name: "Show fewer under Archive.org" })).toBeInTheDocument();
  });

  it("filters registered sources without filtering the provider catalog", async () => {
    const provider = {
      ...source({ kind: "youtube", id: "provider:youtube", target: "YouTube" }),
      group: true,
    };
    const children = Array.from({ length: 12 }, (_, index) =>
      source({
        kind: "youtube",
        id: `youtube:${index + 1}`,
        parentId: provider.id,
        target: index === 10 ? "Museum of Classic Commercials" : `Channel ${index + 1}`,
      }),
    );

    render(<FillerSources sources={[provider, ...children]} onSelect={vi.fn()} />);

    const filter = screen.getByRole("searchbox", { name: "Filter YouTube sources" });
    await userEvent.type(filter, "museum");

    expect(screen.getByText("Museum of Classic Commercials")).toBeInTheDocument();
    expect(screen.queryByText("Channel 1")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Show .* more under YouTube/ })).not.toBeInTheDocument();
  });
});
