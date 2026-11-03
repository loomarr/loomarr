// PROTOTYPE — throwaway (#2151 step 2). Variant A: the item view inside the right side sheet (decision 25).
import { Action, Badge, Surface, Text } from "@loomarr/design-system";
import { useState } from "react";
import { Pressable } from "react-native";
import type { Episode, FieldValue, ItemDetail, SourceFile } from "./prototype-data";
import { Art, FactsChip, SectionLabel, SourceChip } from "./variant-a-parts";

const Section = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
    <SectionLabel>{label}</SectionLabel>
    {children}
  </Surface>
);

const Provenance = ({ field }: { field: FieldValue }) => (
  <Surface gap="inline" level="overlay" padding="control">
    <Text textRole="label" tone="primary">{`Why ${field.value}?`}</Text>
    {field.candidates?.map((c) => (
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="control"
        key={c.source}
      >
        <div className="w-20 shrink-0">
          <SourceChip source={c.source} />
        </div>
        <Surface backgroundColor="transparent" borderWidth={0} flex={1}>
          <Text numberOfLines={2} textRole="compact" tone="primary">
            {c.value}
          </Text>
        </Surface>
        {c.source === field.source ? <Badge tone="success">won</Badge> : null}
      </Surface>
    ))}
    {field.rule ? <Text textRole="metadata">{`Rule · ${field.rule}`}</Text> : null}
  </Surface>
);

const FieldRow = ({ field, open, onToggle }: { field: FieldValue; open: boolean; onToggle: () => void }) => {
  const hasWhy = (field.candidates?.length ?? 0) > 0;
  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
      <Surface
        alignItems="flex-start"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="control"
      >
        <div className="w-24 shrink-0">
          <Text textRole="metadata">{field.label}</Text>
        </div>
        <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={0}>
          <Text textRole="compact" tone="primary">
            {field.value}
          </Text>
        </Surface>
        {hasWhy ? (
          <Pressable
            accessibilityLabel={`Why this ${field.label}?`}
            aria-expanded={open}
            accessibilityRole="button"
            onPress={onToggle}
          >
            <Badge shape="square" tone={open ? "override" : "neutral"}>{`${field.source} · why`}</Badge>
          </Pressable>
        ) : (
          <SourceChip source={field.source} />
        )}
      </Surface>
      {open && hasWhy ? <Provenance field={field} /> : null}
    </Surface>
  );
};

const FileCard = ({ file }: { file: SourceFile }) => (
  <Surface gap={4} opacity={file.stray ? 0.85 : 1} padding="control">
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderWidth={0}
      flexDirection="row"
      flexWrap="wrap"
      gap="inline"
    >
      {file.stray ? <Badge tone="warning">stray copy</Badge> : <Badge tone="success">current file</Badge>}
      {file.stray ? <Text textRole="metadata">never plays</Text> : null}
      <FactsChip state={file.facts} />
    </Surface>
    <Text textRole="metadata" tone="primary" wordWrap="break-word">
      {file.path}
    </Text>
    <Text textRole="compact">{`${file.size} · ${file.container} · ${file.duration} · ${file.chapters} chapters`}</Text>
    <Text textRole="compact">{`Video · ${file.video}`}</Text>
    <Text textRole="compact">{`Audio · ${file.audio.join(" / ")}`}</Text>
    <Text textRole="compact">{`Subs · ${file.subtitles.length ? file.subtitles.join(", ") : "none"}`}</Text>
    <Text textRole="metadata">{`Revision · ${file.revision}`}</Text>
  </Surface>
);

const EpisodeLine = ({ ep }: { ep: Episode }) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderBottomWidth={1}
    borderRadius={0}
    borderWidth={0}
    flexDirection="row"
    flexWrap="wrap"
    gap="inline"
    paddingVertical={6}
  >
    <div className="w-16 shrink-0">
      <Text textRole="data">{ep.code}</Text>
    </div>
    <Surface backgroundColor="transparent" borderWidth={0} flex={1} minWidth={120}>
      <Text numberOfLines={1} textRole="compact" tone="primary">
        {ep.title}
      </Text>
      {ep.special ? (
        <Text textRole="metadata" tone="info">
          {ep.special}
        </Text>
      ) : null}
    </Surface>
    <Text textRole="data">{ep.runtime}</Text>
    <Badge shape="square">{ep.ratingInherited ? `${ep.rating} ↑ series` : ep.rating}</Badge>
    <FactsChip state={ep.facts} />
  </Surface>
);

interface ItemViewProps {
  item: ItemDetail;
  provenanceOpen: boolean;
}

const ItemView = ({ item, provenanceOpen }: ItemViewProps) => {
  const [openField, setOpenField] = useState<string | null>(provenanceOpen ? "Certification" : null);
  const [seasonOpen, setSeasonOpen] = useState(true);
  const [fixOpen, setFixOpen] = useState(false);
  const [note, setNote] = useState<string | null>(null);
  const arr = item.identity === "Sonarr" || item.identity === "Radarr";
  const cert = item.fields.find((f) => f.label === "Certification")?.value;

  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap="section" padding="control">
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" gap="control">
        <Art hue={item.hue} width={72} />
        <Surface backgroundColor="transparent" borderWidth={0} flex={1} gap={6} minWidth={0}>
          <Text textRole="title">{item.title}</Text>
          <Surface
            backgroundColor="transparent"
            borderWidth={0}
            flexDirection="row"
            flexWrap="wrap"
            gap="inline"
          >
            <Text textRole="data">{String(item.year)}</Text>
            {cert ? <Badge shape="square">{cert}</Badge> : null}
            <Badge tone="neutral">{item.kind}</Badge>
            {item.status ? <Badge tone="info">{item.status}</Badge> : null}
            <SourceChip source={item.identity} label={`Identity · ${item.identity}`} />
          </Surface>
          <Text textRole="metadata">{item.ids.map((i) => `${i.namespace}:${i.value}`).join("  ")}</Text>
        </Surface>
      </Surface>

      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap="inline">
        <Action onPress={() => setNote("Metadata refresh queued.")} tone="secondary">
          Refresh metadata
        </Action>
        <Action onPress={() => setNote("File headers will be re-read.")} tone="secondary">
          Re-read files
        </Action>
        <Action onPress={() => setFixOpen((v) => !v)} selected={fixOpen} tone="secondary">
          Fix match
        </Action>
      </Surface>
      {note ? (
        <Text textRole="metadata" tone="success">
          {note}
        </Text>
      ) : null}
      {fixOpen ? (
        <Surface gap="inline" level="overlay" padding="control">
          {arr ? (
            <>
              <Text textRole="body" tone="primary">
                {`The identity of this title is owned by ${item.identity}. Fix the match there and Loomarr follows.`}
              </Text>
              <div className="flex">
                <Action icon="disclosure" tone="primary">
                  {`Open in ${item.identity}`}
                </Action>
              </div>
            </>
          ) : (
            <Text textRole="body" tone="primary">
              Pick the right title; Loomarr writes an explicit link you can undo.
            </Text>
          )}
        </Surface>
      ) : null}

      <Section label="Metadata · each value with its source">
        {item.fields.map((f) => (
          <FieldRow
            field={f}
            key={f.label}
            onToggle={() => setOpenField((cur) => (cur === f.label ? null : f.label))}
            open={openField === f.label}
          />
        ))}
      </Section>

      <Section label="Media sources">
        {item.files?.map((f) => (
          <FileCard file={f} key={f.path} />
        ))}
      </Section>

      {item.seasons ? (
        <Section label="Seasons">
          {item.seasons.map((s) => {
            const open = s.number === 4 && seasonOpen;
            const expandable = s.number === 4;
            return (
              <Surface backgroundColor="transparent" borderWidth={0} key={s.number}>
                <Pressable
                  accessibilityRole="button"
                  aria-expanded={expandable ? open : undefined}
                  disabled={!expandable}
                  onPress={() => setSeasonOpen((v) => !v)}
                >
                  <Surface
                    alignItems="center"
                    flexDirection="row"
                    gap="control"
                    padding={8}
                    paddingHorizontal="control"
                  >
                    <Text textRole="label" tone="primary">
                      {s.label}
                    </Text>
                    <Surface backgroundColor="transparent" borderWidth={0} flex={1}>
                      <Text textRole="metadata">{`${s.episodes} episodes · ${s.read} read`}</Text>
                    </Surface>
                    {expandable ? <Text textRole="metadata">{open ? "Hide" : "Show"}</Text> : null}
                  </Surface>
                </Pressable>
                {open && item.openSeason ? (
                  <Surface backgroundColor="transparent" borderWidth={0} paddingHorizontal="control">
                    {item.openSeason.map((e) => (
                      <EpisodeLine ep={e} key={e.code} />
                    ))}
                  </Surface>
                ) : null}
              </Surface>
            );
          })}
        </Section>
      ) : null}

      <Section label="Used by">
        {item.usedBy.map((u) => (
          <Surface
            alignItems="center"
            backgroundColor="transparent"
            borderWidth={0}
            flexDirection="row"
            gap="control"
            key={u.channel}
          >
            <Text textRole="compact" tone="primary">
              {u.channel}
            </Text>
            <Text textRole="metadata">{u.how}</Text>
          </Surface>
        ))}
      </Section>

      <Section label="Artwork">
        <Surface
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          flexWrap="wrap"
          gap="inline"
        >
          {item.artwork.map((a) => (
            <SourceChip key={a.role} label={`${a.role} · ${a.source}`} source={a.source} />
          ))}
        </Surface>
      </Section>
    </Surface>
  );
};

export { ItemView };
