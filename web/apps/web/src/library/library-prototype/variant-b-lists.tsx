// PROTOTYPE — throwaway (#2151 step 2). Variant B master lists: items (with compact filters),
// unmatched groups, collections. Every row is two lines so the 40% pane stays scannable.
import { Action, Badge, Disclosure, Field, Text } from "@loomarr/design-system";
import { useState } from "react";
import type { CollectionRow, FactsState, IdentitySource, ItemRow, UnmatchedGroup } from "./prototype-data";
import {
  Art,
  Box,
  Chip,
  EmptyNote,
  FactsMark,
  IdentityChip,
  ListRow,
  Meta,
  SectionLabel,
} from "./variant-b-parts";

type KindFilter = "all" | "Movie" | "Series";
type IdentityFilter = "all" | IdentitySource;
type FactsFilter = "all" | "pending" | "settled" | "failed";

const IDENTITIES: IdentityFilter[] = ["all", "Sonarr", "Radarr", "NFO", "Linked", "Embedded id"];
const FACTS_FILTERS: { value: FactsFilter; label: string }[] = [
  { value: "all", label: "Any facts" },
  { value: "pending", label: "Waiting / reading" },
  { value: "settled", label: "Read / measured" },
  { value: "failed", label: "Failed" },
];
const factsGroup = (f: FactsState): FactsFilter =>
  f === "waiting" || f === "reading" ? "pending" : f === "failed" ? "failed" : "settled";

const ItemsList = ({
  items,
  selectedId,
  onSelect,
  onOpenSettings,
}: {
  items: ItemRow[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onOpenSettings: () => void;
}) => {
  const [search, setSearch] = useState("");
  const [kind, setKind] = useState<KindFilter>("all");
  const [identity, setIdentity] = useState<IdentityFilter>("all");
  const [facts, setFacts] = useState<FactsFilter>("all");
  const [problems, setProblems] = useState(false);
  const [open, setOpen] = useState(false);

  if (items.length === 0)
    return (
      <EmptyNote
        action={{ label: "Open Settings → Library", icon: "settings", onPress: onOpenSettings }}
        body="Nothing has been imported yet. Confirm where your media lives in Settings → Library and Loomarr starts reading it."
        title="Your library is empty"
      />
    );

  const q = search.trim().toLowerCase();
  const shown = items.filter(
    (it) =>
      (!q || it.title.toLowerCase().includes(q)) &&
      (kind === "all" || it.kind === kind) &&
      (identity === "all" || it.identity === identity) &&
      (facts === "all" || factsGroup(it.facts) === facts) &&
      (!problems || Boolean(it.problems?.length) || it.facts === "failed"),
  );
  const extra = (identity !== "all" ? 1 : 0) + (facts !== "all" ? 1 : 0);

  return (
    <Box gap="control">
      <Field
        accessibilityLabel="Search items"
        onChangeText={setSearch}
        placeholder="Search titles"
        value={search}
      />
      <Box flexDirection="row" flexWrap="wrap" gap="inline">
        {(["all", "Movie", "Series"] as const).map((k) => (
          <Chip
            key={k}
            label={k === "all" ? "All" : `${k}s`}
            onPress={() => setKind(k)}
            selected={kind === k}
          />
        ))}
        <Chip label="Has problems" onPress={() => setProblems((p) => !p)} selected={problems} />
      </Box>
      <Disclosure
        description={extra ? `${extra} active` : undefined}
        expanded={open}
        label="Identity & facts"
        onExpandedChange={setOpen}
      >
        <Box gap="inline">
          <SectionLabel>Identity source</SectionLabel>
          <Box flexDirection="row" flexWrap="wrap" gap="inline">
            {IDENTITIES.map((i) => (
              <Chip
                key={i}
                label={i === "all" ? "Any" : i}
                onPress={() => setIdentity(i)}
                selected={identity === i}
              />
            ))}
          </Box>
          <SectionLabel>Facts</SectionLabel>
          <Box flexDirection="row" flexWrap="wrap" gap="inline">
            {FACTS_FILTERS.map((f) => (
              <Chip
                key={f.value}
                label={f.label}
                onPress={() => setFacts(f.value)}
                selected={facts === f.value}
              />
            ))}
          </Box>
        </Box>
      </Disclosure>
      <Meta>{`${shown.length} of ${items.length} shown`}</Meta>
      {shown.length === 0 ? <Meta>No items match these filters.</Meta> : null}
      <Box gap={6}>
        {shown.map((it) => (
          <ListRow
            key={it.id}
            label={`${it.title}, ${it.year}`}
            onPress={() => onSelect(it.id)}
            selected={it.id === selectedId}
          >
            <Art hue={it.hue} title={it.title} width={32} />
            <Box flex={1} gap={4} minWidth={0}>
              <Box alignItems="center" flexDirection="row" gap="inline" justifyContent="space-between">
                <Box alignItems="baseline" flex={1} flexDirection="row" gap="inline" minWidth={0}>
                  <Text numberOfLines={1} textRole="label">
                    {it.title}
                  </Text>
                  <Meta>{String(it.year)}</Meta>
                </Box>
                <FactsMark detail={it.factsDetail} facts={it.facts} />
              </Box>
              <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
                <Meta>{it.kind}</Meta>
                <IdentityChip source={it.identity} />
                <Meta>{it.certification}</Meta>
                {it.offline ? <Badge tone="danger">offline</Badge> : null}
                {it.problems?.map((p) => (
                  <Meta key={p} tone="warning">{`! ${p}`}</Meta>
                ))}
              </Box>
            </Box>
          </ListRow>
        ))}
      </Box>
    </Box>
  );
};

const UnmatchedList = ({
  groups,
  selectedId,
  onSelect,
  decided,
}: {
  groups: UnmatchedGroup[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  decided: Record<string, string>;
}) =>
  groups.length === 0 ? (
    <EmptyNote body="Every file Loomarr found is grounded in an identity." title="Nothing to review" />
  ) : (
    <Box gap={6}>
      {groups.map((g) => (
        <ListRow key={g.id} label={g.parsed} onPress={() => onSelect(g.id)} selected={g.id === selectedId}>
          <Box flex={1} gap={4} minWidth={0}>
            <Box alignItems="center" flexDirection="row" gap="inline" justifyContent="space-between">
              <Text numberOfLines={1} textRole="label">
                {g.parsed}
              </Text>
              {decided[g.id] ? (
                <Badge tone="success">{decided[g.id] as string}</Badge>
              ) : g.best ? null : (
                <Badge tone="warning">no candidate</Badge>
              )}
            </Box>
            <Meta>{`${g.files.length} ${g.files.length === 1 ? "file" : "files"} · ${g.folder} · ${g.found}`}</Meta>
          </Box>
        </ListRow>
      ))}
    </Box>
  );

const CollectionsList = ({
  rows,
  selectedId,
  onSelect,
  onNew,
  onCopy,
}: {
  rows: CollectionRow[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onNew: () => void;
  onCopy: () => void;
}) => {
  const group = (kind: "TMDB" | "Mine") => rows.filter((r) => r.kind === kind);
  const render = (r: CollectionRow) => (
    <ListRow key={r.id} label={r.name} onPress={() => onSelect(r.id)} selected={r.id === selectedId}>
      <Art hue={r.hue} title={r.name} width={28} />
      <Box flex={1} gap={4} minWidth={0}>
        <Text numberOfLines={1} textRole="label">
          {r.name}
        </Text>
        <Box alignItems="center" flexDirection="row" flexWrap="wrap" gap="inline">
          <Meta>{r.total ? `${r.owned} of ${r.total} owned` : `${r.owned} titles`}</Meta>
          {r.fromEmby ? <Badge tone="neutral">from Emby</Badge> : null}
          {r.usedBy?.map((u) => (
            <Badge key={u} tone="info">{`used by ${u.split(" · ")[0]}`}</Badge>
          ))}
        </Box>
      </Box>
    </ListRow>
  );
  return (
    <Box gap="control">
      <Box gap={6}>
        <SectionLabel>Loomarr collections · editable</SectionLabel>
        <Box flexDirection="row" flexWrap="wrap" gap="inline">
          <Action icon="plus" onPress={onNew}>
            New collection
          </Action>
          <Action onPress={onCopy} variant="outline">
            Copy from a TMDB collection
          </Action>
        </Box>
        {group("Mine").map(render)}
      </Box>
      <Box gap={6}>
        <SectionLabel>TMDB collections · read-only</SectionLabel>
        {group("TMDB").map(render)}
      </Box>
    </Box>
  );
};

export { CollectionsList, ItemsList, UnmatchedList };
