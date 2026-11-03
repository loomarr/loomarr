// PROTOTYPE — throwaway (#2151 step 2). Variant A: the Items table (decision 24).
import { Action, Badge, Field, Surface, Tabs, Text, Toggle } from "@loomarr/design-system";
import { useMemo, useState } from "react";
import { Pressable } from "react-native";
import type { FactsState, IdentitySource, ItemRow } from "./prototype-data";
import { Art, FactsChip, SourceChip } from "./variant-a-parts";

type KindFilter = "all" | "Movie" | "Series";
const IDENTITIES: IdentitySource[] = ["Sonarr", "Radarr", "NFO", "Linked", "Embedded id"];
const FACTS_STATES: FactsState[] = ["waiting", "reading", "read", "measured", "failed"];

// A chip that cycles through "any" and each value: dense, no popup list pushing the table down.
const CycleFilter = <T extends string>({
  label,
  value,
  values,
  onChange,
}: {
  label: string;
  value: T | "any";
  values: readonly T[];
  onChange: (next: T | "any") => void;
}) => {
  const order: (T | "any")[] = ["any", ...values];
  const next = order[(order.indexOf(value) + 1) % order.length] as T | "any";
  return (
    <Action
      onPress={() => onChange(next)}
      selected={value !== "any"}
      tone="secondary"
      accessibilityLabel={`${label}: ${value}. Change`}
    >
      {`${label}: ${value}`}
    </Action>
  );
};

const COLS = { art: 40, year: 56, kind: 68, identity: 108, cert: 64, facts: 132 } as const;

interface ItemsTableProps {
  items: ItemRow[];
  selectedId?: string;
  phone: boolean;
  fresh: boolean;
  onOpen: (row: ItemRow) => void;
  onOpenSettings: () => void;
}

const HeaderCell = ({ children, width }: { children: string; width?: number }) => (
  <Surface backgroundColor="transparent" borderWidth={0} flex={width ? undefined : 1} width={width}>
    <Text shout textRole="metadata">
      {children}
    </Text>
  </Surface>
);

const Cell = ({ children, width }: { children: React.ReactNode; width: number }) => (
  <Surface backgroundColor="transparent" borderWidth={0} width={width}>
    {children}
  </Surface>
);

const ItemsTable = ({ items, selectedId, phone, fresh, onOpen, onOpenSettings }: ItemsTableProps) => {
  const [search, setSearch] = useState("");
  const [kind, setKind] = useState<KindFilter>("all");
  const [identity, setIdentity] = useState<IdentitySource | "any">("any");
  const [facts, setFacts] = useState<FactsState | "any">("any");
  const [problemsOnly, setProblemsOnly] = useState(false);

  const rows = useMemo(
    () =>
      items.filter(
        (r) =>
          (kind === "all" || r.kind === kind) &&
          (identity === "any" || r.identity === identity) &&
          (facts === "any" || r.facts === facts) &&
          (!problemsOnly || r.facts === "failed" || (r.problems?.length ?? 0) > 0) &&
          r.title.toLowerCase().includes(search.trim().toLowerCase()),
      ),
    [items, search, kind, identity, facts, problemsOnly],
  );

  if (fresh) {
    return (
      <Surface alignItems="flex-start" gap="control" padding="section">
        <Text textRole="title">Nothing imported yet</Text>
        <Text textRole="body" tone="secondary">
          Loomarr has found your media folders but has not been told to scan them. Confirm the scan roots in
          Settings → Library to start the import.
        </Text>
        <Action icon="settings" onPress={onOpenSettings} tone="primary">
          Open Settings → Library
        </Action>
      </Surface>
    );
  }

  return (
    <Surface gap="control" padding="control">
      <div className="flex flex-wrap items-end gap-2">
        <div className="w-full sm:w-64">
          <Field
            accessibilityLabel="Search items"
            onChangeText={setSearch}
            placeholder="Search titles"
            value={search}
          />
        </div>
        <Tabs
          label="Kind"
          onValueChange={setKind}
          options={[
            { label: "All", value: "all" },
            { label: "Movies", value: "Movie" },
            { label: "Series", value: "Series" },
          ]}
          value={kind}
        />
        <CycleFilter label="Identity" onChange={setIdentity} value={identity} values={IDENTITIES} />
        <CycleFilter label="Facts" onChange={setFacts} value={facts} values={FACTS_STATES} />
        <Toggle checked={problemsOnly} label="Has problems" onCheckedChange={setProblemsOnly} />
      </div>
      <Text textRole="metadata">{`${rows.length} of ${items.length} shown`}</Text>

      {phone ? null : (
        <Surface
          backgroundColor="transparent"
          borderWidth={0}
          borderBottomWidth={1}
          borderRadius={0}
          flexDirection="row"
          gap="control"
          paddingHorizontal="control"
          paddingBottom={6}
        >
          <Cell width={COLS.art}>{null}</Cell>
          <HeaderCell>Title</HeaderCell>
          <HeaderCell width={COLS.year}>Year</HeaderCell>
          <HeaderCell width={COLS.kind}>Kind</HeaderCell>
          <HeaderCell width={COLS.identity}>Identity</HeaderCell>
          <HeaderCell width={COLS.cert}>Cert</HeaderCell>
          <HeaderCell width={COLS.facts}>Facts</HeaderCell>
        </Surface>
      )}

      {rows.length === 0 ? (
        <Text textRole="body" tone="secondary">
          No items match these filters.
        </Text>
      ) : null}

      {rows.map((r) => {
        const selected = r.id === selectedId;
        const problem = r.facts === "failed" ? (r.problems?.[0] ?? "read failed") : r.problems?.join(" · ");
        return (
          <Pressable
            key={r.id}
            accessibilityLabel={`Open ${r.title}`}
            accessibilityRole="button"
            onPress={() => onOpen(r)}
          >
            <Surface
              alignItems="center"
              backgroundColor={selected ? "surfaceElevated" : "surfaceRaised"}
              borderBottomWidth={1}
              borderColor="borderDecorative"
              borderLeftWidth={0}
              borderRadius={0}
              borderRightWidth={0}
              borderTopWidth={0}
              flexDirection="row"
              gap="control"
              opacity={r.offline ? 0.6 : 1}
              padding={8}
              paddingHorizontal="control"
            >
              <Cell width={COLS.art}>
                <Art hue={r.hue} width={28} />
              </Cell>
              <Surface
                alignItems="flex-start"
                backgroundColor="transparent"
                borderWidth={0}
                flex={1}
                gap={2}
                minWidth={0}
              >
                <Text numberOfLines={1} textAlign="left" textRole="label" tone="primary">
                  {r.title}
                </Text>
                {phone ? (
                  <Text numberOfLines={1} textAlign="left" textRole="metadata">
                    {`${r.year} · ${r.kind} · ${r.certification} · ${r.identity}`}
                  </Text>
                ) : null}
                {problem ? (
                  <Text numberOfLines={1} textAlign="left" textRole="metadata" tone="warning">
                    {problem}
                  </Text>
                ) : null}
                {r.offline ? (
                  <Text numberOfLines={1} textAlign="left" textRole="metadata" tone="danger">
                    offline — /media/movies unreadable
                  </Text>
                ) : null}
              </Surface>
              {phone ? null : (
                <>
                  <Cell width={COLS.year}>
                    <Text textRole="data">{String(r.year)}</Text>
                  </Cell>
                  <Cell width={COLS.kind}>
                    <Text textRole="compact">{r.kind}</Text>
                  </Cell>
                  <Cell width={COLS.identity}>
                    <div className="flex">
                      <SourceChip source={r.identity} />
                    </div>
                  </Cell>
                  <Cell width={COLS.cert}>
                    <div className="flex">
                      <Badge shape="square">{r.certification}</Badge>
                    </div>
                  </Cell>
                </>
              )}
              <Cell width={phone ? 96 : COLS.facts}>
                {r.offline ? (
                  <Badge tone="warning">offline</Badge>
                ) : (
                  <FactsChip detail={r.factsDetail} state={r.facts} />
                )}
              </Cell>
            </Surface>
          </Pressable>
        );
      })}
    </Surface>
  );
};

export { ItemsTable };
