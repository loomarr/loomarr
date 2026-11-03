// PROTOTYPE — throwaway (#2151 step 2). Variant C · Items: a full-page table.
import {
  Action,
  Badge,
  Field,
  SegmentedControl,
  SelectControl,
  Surface,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { useState } from "react";
import { type FactsState, type IdentitySource, type ItemRow, itemsFor } from "./prototype-data";
import type { ScanState, Screen } from "./prototype-state";
import { Art, FactsChip, SourceChip, Warn } from "./variant-c-parts";

type KindFilter = "all" | "Movie" | "Series";
const IDENTITIES: IdentitySource[] = ["Sonarr", "Radarr", "NFO", "Linked", "Embedded id"];
const FACTS: FactsState[] = ["waiting", "reading", "read", "measured", "failed"];

// Same grid on every row so the columns line up; phones drop to a two-line row.
const COLS = "md:grid md:grid-cols-[minmax(0,1fr)_56px_72px_110px_72px_150px] md:items-center md:gap-3";

const Row = ({ item, onOpen }: { item: ItemRow; onOpen: () => void }) => (
  <div className={`border-border flex flex-col gap-1 border-b px-3 py-2 ${COLS}`}>
    <div className="flex min-w-0 items-center gap-3">
      <Art hue={item.hue} title={item.title} width={28} />
      <div className="flex min-w-0 flex-col">
        <Action onPress={onOpen} variant="link">
          {item.title}
        </Action>
        <div className="flex flex-wrap items-center gap-2">
          {item.offline ? <Badge tone="danger">offline</Badge> : null}
          {item.problems?.map((p) => (
            <Warn key={p}>{p}</Warn>
          ))}
        </div>
      </div>
    </div>
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1 pl-10 md:contents md:pl-0">
      <Text textRole="data" tone="secondary">
        {item.year}
      </Text>
      <Text textRole="metadata" tone="secondary">
        {item.kind}
      </Text>
      <span>
        <SourceChip source={item.identity} />
      </span>
      <Text textRole="data" tone="secondary">
        {item.certification}
      </Text>
      <FactsChip detail={item.factsDetail} facts={item.facts} />
    </div>
  </div>
);

const ItemsPage = ({ scan, go }: { scan: ScanState; go: (s: Screen) => void }) => {
  const all = itemsFor(scan);
  const [q, setQ] = useState("");
  const [kind, setKind] = useState<KindFilter>("all");
  const [identity, setIdentity] = useState<IdentitySource | "any">("any");
  const [facts, setFacts] = useState<FactsState | "any">("any");
  const [open, setOpen] = useState<"identity" | "facts" | null>(null);
  const [problems, setProblems] = useState(false);

  if (scan === "fresh") {
    return (
      <Surface gap="control" padding="section">
        <Text textRole="headline">Nothing imported yet</Text>
        <Text textRole="body" tone="secondary">
          Loomarr found media folders but you have not confirmed them, so there is nothing to list. Confirm
          where your media is in Settings → Library and the import starts.
        </Text>
        <div>
          <Action onPress={() => go("settings")} tone="primary">
            Open Settings → Library
          </Action>
        </div>
      </Surface>
    );
  }

  const rows = all.filter(
    (it) =>
      (kind === "all" || it.kind === kind) &&
      (identity === "any" || it.identity === identity) &&
      (facts === "any" || it.facts === facts) &&
      (!problems || it.problems || it.facts === "failed" || it.offline) &&
      it.title.toLowerCase().includes(q.trim().toLowerCase()),
  );

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-3 md:flex-row md:flex-wrap md:items-end">
        <div className="md:w-64">
          <Field
            accessibilityLabel="Search titles"
            onChangeText={setQ}
            placeholder="Search titles"
            value={q}
          />
        </div>
        <SegmentedControl
          accessibilityLabel="Kind"
          onValueChange={setKind}
          options={[
            { label: "All", value: "all" },
            { label: "Movies", value: "Movie" },
            { label: "Series", value: "Series" },
          ]}
          value={kind}
        />
        <div className="md:w-44">
          <SelectControl
            appearance="field"
            label="Identity"
            onOpenChange={(o) => setOpen(o ? "identity" : null)}
            onValueChange={(v) => {
              setIdentity(v);
              setOpen(null);
            }}
            open={open === "identity"}
            options={[
              { label: "Any identity", value: "any" },
              ...IDENTITIES.map((i) => ({ label: i, value: i })),
            ]}
            value={identity}
          />
        </div>
        <div className="md:w-40">
          <SelectControl
            appearance="field"
            label="Facts"
            onOpenChange={(o) => setOpen(o ? "facts" : null)}
            onValueChange={(v) => {
              setFacts(v);
              setOpen(null);
            }}
            open={open === "facts"}
            options={[{ label: "Any facts", value: "any" }, ...FACTS.map((f) => ({ label: f, value: f }))]}
            value={facts}
          />
        </div>
        <Toggle checked={problems} label="Has problems" onCheckedChange={setProblems} />
      </div>

      <Surface overflow="hidden">
        <div className={`border-border hidden border-b px-3 py-2 ${COLS}`}>
          {["Title", "Year", "Kind", "Identity", "Cert", "Facts"].map((h) => (
            <Text key={h} shout textRole="metadata" tone="muted">
              {h}
            </Text>
          ))}
        </div>
        {rows.length === 0 ? (
          <div className="p-4">
            <Text textRole="body" tone="secondary">
              No titles match these filters.
            </Text>
          </div>
        ) : (
          rows.map((it) => (
            <Row
              item={it}
              key={it.id}
              onOpen={() => go(it.kind === "Series" ? "item-series" : "item-movie")}
            />
          ))
        )}
      </Surface>
      <Text textRole="metadata" tone="muted">
        Showing {rows.length} of 2,812 titles · the first {all.length} are loaded in this prototype
      </Text>
    </div>
  );
};

export { ItemsPage };
