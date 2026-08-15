import type { SettingEntry } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { AllSettingsTable } from "./all-settings-table";

const entry = (over: Partial<SettingEntry> = {}): SettingEntry =>
  ({
    key: "library.url",
    group: "connections.media_server",
    kind: "url",
    value: "http://emby:8096",
    provenance: "db",
    secret: false,
    set: true,
    advanced: false,
    ...over,
  }) as SettingEntry;

const ENTRIES: SettingEntry[] = [
  entry(),
  entry({ key: "job.workers", group: "advanced", value: "2", provenance: "env", advanced: true }),
  entry({
    key: "tunarr.url",
    group: "connections.tunarr",
    value: "http://tunarr:8000",
    provenance: "default",
  }),
  entry({
    key: "library.token",
    group: "connections.media_server",
    secret: true,
    value: "",
    preview: "…a1b2",
  }),
];

// Controlled component, so the harness owns the query the way the page does.
const Harness = ({ entries = ENTRIES }: { entries?: SettingEntry[] }) => {
  const [q, setQ] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  return (
    <AllSettingsTable
      entries={entries}
      query={q}
      onQueryChange={setQ}
      values={values}
      onEdit={(k, v) => setValues((prev) => ({ ...prev, [k]: v }))}
    />
  );
};

describe("AllSettingsTable", () => {
  it("lists every key with its group and provenance", () => {
    render(<Harness />);
    expect(screen.getByText("library.url")).toBeInTheDocument();
    expect(screen.getByText("SET VIA ENV")).toBeInTheDocument();
    expect(screen.getByText("4 of 4 shown")).toBeInTheDocument();
  });

  // The phase gate: search matches key AND group AND value. Three separate assertions because
  // each is a different way of already knowing what you want — the key from a config file, the
  // group from remembering roughly where it lives, the value when hunting for whichever
  // setting points at the wrong URL.
  it("matches on the key", async () => {
    render(<Harness />);
    await userEvent.type(screen.getByLabelText("Search settings"), "job.workers");
    expect(screen.getByText("job.workers")).toBeInTheDocument();
    expect(screen.queryByText("library.url")).not.toBeInTheDocument();
  });

  it("matches on the group", async () => {
    render(<Harness />);
    await userEvent.type(screen.getByLabelText("Search settings"), "tunarr");
    expect(screen.getByText("tunarr.url")).toBeInTheDocument();
    expect(screen.queryByText("job.workers")).not.toBeInTheDocument();
  });

  it("matches on the value", async () => {
    render(<Harness />);
    await userEvent.type(screen.getByLabelText("Search settings"), "emby:8096");
    expect(screen.getByText("library.url")).toBeInTheDocument();
    expect(screen.queryByText("tunarr.url")).not.toBeInTheDocument();
  });

  it("searches case-insensitively", async () => {
    render(<Harness />);
    await userEvent.type(screen.getByLabelText("Search settings"), "JOB.WORKERS");
    expect(screen.getByText("job.workers")).toBeInTheDocument();
  });

  // The gate's second clause. ADV belongs on this surface especially: it is where someone lands
  // BECAUSE the key was not where they looked, and "it's advanced" is the answer.
  it("marks advanced keys, and only those", () => {
    render(<Harness />);
    const chips = screen.getAllByText("ADV");
    expect(chips).toHaveLength(1);
    expect(chips[0]?.title).toMatch(/advanced/i);
  });

  it("counts what the search narrowed to", async () => {
    render(<Harness />);
    await userEvent.type(screen.getByLabelText("Search settings"), "connections");
    expect(screen.getByText("3 of 4 shown")).toBeInTheDocument();
  });

  // A query matching nothing must SAY so — otherwise it is indistinguishable from a page that
  // failed to load.
  it("says when nothing matched, naming the query", async () => {
    render(<Harness />);
    await userEvent.type(screen.getByLabelText("Search settings"), "zzz");
    expect(screen.getByText(/No setting matches/)).toHaveTextContent("zzz");
    expect(screen.getByText("0 of 4 shown")).toBeInTheDocument();
  });

  // ⚠ A secret's VALUE never renders (§4). The masked preview is all there is — and an empty
  // cell would read as "unset" for a key that is very much set.
  it("shows a secret's masked preview, never its value", () => {
    render(<Harness />);
    expect(screen.getByText("…a1b2")).toBeInTheDocument();
  });

  // An UNSET secret is enterable rather than a read-only "not set" line — this is the one
  // surface some keys have left, so refusing to accept a first value would strand them.
  // (A STORED secret still masks: the case above.)
  it("lets an unset secret be entered", () => {
    const { container } = render(
      <Harness entries={[entry({ key: "tmdb.api_key", secret: true, set: false, value: "" })]} />,
    );
    // By the field's own id — the search box is a textbox too, so an unscoped role query
    // matches two.
    expect(container.querySelector("#setting-tmdb\\.api_key")).toBeEnabled();
  });

  // ⚠ An env-pinned key is locked HERE too. This table is the editor of last resort, which
  // makes it exactly where someone would try to work around the lock — and §3 says env wins.
  // Compact controls render no <label> of their own, so without an explicit association the
  // input is named only by `title` — axe rates that a SERIOUS violation (`label-title-only`)
  // and a screen-reader user hears an unnamed text box. The Key cell is the visible label; the
  // control points at it.
  it("names each control by its visible key cell", () => {
    const { container } = render(<Harness entries={[entry({ key: "job.workers" })]} />);
    const input = container.querySelector("#setting-job\\.workers");
    expect(input).toHaveAttribute("aria-labelledby", "job.workers-label");
    expect(container.querySelector("#job\\.workers-label")).toHaveTextContent("job.workers");
  });

  it("locks an env-pinned key", () => {
    const { container } = render(
      <Harness entries={[entry({ key: "job.workers", value: "2", provenance: "env" })]} />,
    );
    expect(container.querySelector("#setting-job\\.workers")).toBeDisabled();
  });

  it("links a group to the workflow that owns it", () => {
    render(<Harness entries={[entry()]} />);
    expect(screen.getByRole("link", { name: "connections.media_server" })).toHaveAttribute(
      "href",
      "/settings/connections",
    );
  });

  it("can clear a database value back to its default", async () => {
    const onClear = vi.fn();
    const dbEntry = entry();
    render(
      <AllSettingsTable
        entries={[dbEntry]}
        query=""
        onQueryChange={vi.fn()}
        values={{}}
        onEdit={vi.fn()}
        onClear={onClear}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Use default" }));
    expect(onClear).toHaveBeenCalledWith(dbEntry);
  });
});
