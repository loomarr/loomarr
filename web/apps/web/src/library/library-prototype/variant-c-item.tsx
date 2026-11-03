// PROTOTYPE — throwaway (#2151 step 2). Variant C · Item view: its own full page, breadcrumb and
// section tabs (Overview · Files · Episodes · Used by) like a channel's detail tabs.
import { Action, Badge, Disclosure, Surface, Tabs, Text } from "@loomarr/design-system";
import { useState } from "react";
import { FARGO, type FieldValue, type ItemDetail, type SourceFile, WEST_WING } from "./prototype-data";
import type { Screen } from "./prototype-state";
import { Art, Crumbs, FactsChip, Kv, SectionLabel, SourceChip } from "./variant-c-parts";

type Section = "overview" | "files" | "episodes" | "usedby";

const FieldRow = ({
  field,
  popOpen,
  onToggle,
}: {
  field: FieldValue;
  popOpen: boolean;
  onToggle: () => void;
}) => (
  <Kv label={field.label}>
    <div className="relative flex flex-wrap items-start gap-2">
      <div className="min-w-0 flex-1">
        <Text textRole="body">{field.value}</Text>
      </div>
      {field.candidates ? (
        <Action aria-expanded={popOpen} onPress={onToggle} variant="ghost">
          {field.source}
        </Action>
      ) : (
        <SourceChip source={field.source} />
      )}
      {popOpen && field.candidates ? (
        <Surface
          aria-label={`Where ${field.label} comes from`}
          className="z-10 w-full gap-2 p-3 md:w-96"
          level="overlay"
          role="dialog"
        >
          <SectionLabel>Every provider says</SectionLabel>
          {field.candidates.map((c) => (
            <div className="flex items-start gap-2" key={c.source}>
              <SourceChip source={c.source} />
              <div className="min-w-0 flex-1">
                <Text textRole="compact">{c.value}</Text>
              </div>
              {c.source === field.source ? <Badge tone="signal">used</Badge> : null}
            </div>
          ))}
          {field.rule ? (
            <Text textRole="metadata" tone="secondary">
              Rule · {field.rule}
            </Text>
          ) : null}
        </Surface>
      ) : null}
    </div>
  </Kv>
);

const FileCard = ({ file }: { file: SourceFile }) => (
  <Surface gap="$2" padding="$3" style={file.stray ? { opacity: 0.8 } : undefined}>
    <div className="flex flex-wrap items-center gap-2">
      <Badge tone={file.stray ? "caution" : "success"}>{file.stray ? "stray copy" : "current file"}</Badge>
      {file.stray ? (
        <Text textRole="metadata" tone="secondary">
          never plays
        </Text>
      ) : null}
      <FactsChip facts={file.facts} />
    </div>
    <Text textRole="data" tone="secondary">
      {file.path}
    </Text>
    <div className="grid grid-cols-1 gap-x-6 gap-y-1 sm:grid-cols-2">
      <Kv label="Size">
        <Text textRole="compact">{file.size}</Text>
      </Kv>
      <Kv label="Duration">
        <Text textRole="compact">{file.duration}</Text>
      </Kv>
      <Kv label="Video">
        <Text textRole="compact">{file.video}</Text>
      </Kv>
      <Kv label="Audio">
        <Text textRole="compact">{file.audio.join(" · ")}</Text>
      </Kv>
      <Kv label="Subtitles">
        <Text textRole="compact">{file.subtitles.length ? file.subtitles.join(" · ") : "none"}</Text>
      </Kv>
      <Kv label="Chapters">
        <Text textRole="compact">{file.chapters}</Text>
      </Kv>
      <Kv label="Revision">
        <Text textRole="compact">{file.revision}</Text>
      </Kv>
    </div>
  </Surface>
);

const FixMatch = ({ item }: { item: ItemDetail }) => {
  const [open, setOpen] = useState(false);
  const arr = item.identity === "Sonarr" || item.identity === "Radarr";
  return (
    <div className="flex flex-col gap-2">
      <Action onPress={() => setOpen(!open)} variant="outline">
        Fix match
      </Action>
      {open ? (
        <Surface gap="$2" padding="$3">
          {arr ? (
            <>
              <Text textRole="compact">
                {item.identity} owns this identity. Correct the match there; Loomarr follows on the next sync.
              </Text>
              <div>
                <Action variant="secondary">Open in {item.identity}</Action>
              </div>
            </>
          ) : (
            <Text textRole="compact">Search TMDB and link this title to a different match.</Text>
          )}
        </Surface>
      ) : null}
    </div>
  );
};

const SeasonList = ({ item }: { item: ItemDetail }) => {
  const [openSeason, setOpenSeason] = useState(4);
  return (
    <div className="flex flex-col gap-2">
      {item.seasons?.map((s) => (
        <Disclosure
          description={`${s.read}/${s.episodes} read`}
          expanded={openSeason === s.number}
          key={s.number}
          label={s.label}
          onExpandedChange={(e) => setOpenSeason(e ? s.number : 0)}
        >
          {s.number === 4 && item.openSeason ? (
            <div className="flex flex-col">
              {item.openSeason.map((e) => (
                <div
                  className="border-border flex flex-wrap items-center gap-x-4 gap-y-1 border-b px-1 py-2"
                  key={e.code}
                >
                  <div className="w-16">
                    <Text textRole="data">{e.code}</Text>
                  </div>
                  <div className="min-w-0 flex-1 basis-40">
                    <Text textRole="compact">{e.title}</Text>
                    {e.special ? (
                      <Text textRole="metadata" tone="warning">
                        {e.special}
                      </Text>
                    ) : null}
                  </div>
                  <Text textRole="data" tone="secondary">
                    {e.runtime}
                  </Text>
                  <span className="inline-flex items-center gap-1">
                    <Text textRole="data" tone="secondary">
                      {e.rating}
                    </Text>
                    {e.ratingInherited ? (
                      <Text textRole="metadata" tone="muted">
                        inherited
                      </Text>
                    ) : null}
                  </span>
                  <FactsChip facts={e.facts} />
                </div>
              ))}
            </div>
          ) : (
            <Text textRole="metadata" tone="muted">
              Collapsed in this prototype.
            </Text>
          )}
        </Disclosure>
      ))}
    </div>
  );
};

const ItemPage = ({ screen, go }: { screen: Screen; go: (s: Screen) => void }) => {
  const item = screen === "item-movie" ? FARGO : WEST_WING;
  const series = item.kind === "Series";
  const [section, setSection] = useState<Section>("overview");
  const [popOpen, setPopOpen] = useState<string | null>(screen === "provenance" ? "Certification" : null);

  const sections: { label: string; value: Section }[] = [
    { label: "Overview", value: "overview" },
    { label: "Files", value: "files" },
    ...(series ? [{ label: "Episodes", value: "episodes" as Section }] : []),
    { label: "Used by", value: "usedby" },
  ];

  const files = (
    <div className="flex flex-col gap-3">
      <SectionLabel>Media sources</SectionLabel>
      {item.files?.map((f) => (
        <FileCard file={f} key={f.path} />
      ))}
    </div>
  );
  const usedBy = (
    <div className="flex flex-col gap-2">
      <SectionLabel>Used by channels</SectionLabel>
      {item.usedBy.map((u) => (
        <Surface gap="$1" key={u.channel} padding="$3">
          <Text textRole="label">{u.channel}</Text>
          <Text textRole="metadata" tone="secondary">
            {u.how}
          </Text>
        </Surface>
      ))}
    </div>
  );

  return (
    <div className="flex flex-col gap-4">
      <Crumbs onBack={() => go("items")} trail={["Library", item.title]} />

      <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
        <Art hue={item.hue} title={item.title} width={88} />
        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <Text textRole="title">{item.title}</Text>
          <div className="flex flex-wrap items-center gap-2">
            <Text textRole="data" tone="secondary">
              {item.year}
            </Text>
            <Badge tone="neutral">
              {item.fields.find((f) => f.label === "Certification")?.value ?? "NR"}
            </Badge>
            {item.status ? <Badge tone="info">{item.status}</Badge> : null}
            <SourceChip source={item.identity} />
            <Text textRole="metadata" tone="secondary">
              {item.ids.map((i) => `${i.namespace} ${i.value}`).join(" · ")}
            </Text>
          </div>
          <div className="flex flex-wrap gap-2">
            <Action variant="secondary">Refresh metadata</Action>
            <Action variant="secondary">Re-read files</Action>
          </div>
          <FixMatch item={item} />
        </div>
      </div>

      <Tabs label="Item sections" onValueChange={setSection} options={sections} value={section} />

      {section === "overview" ? (
        <div className="flex flex-col gap-4">
          <Surface gap="$3" padding="$4">
            <SectionLabel>Metadata · each field shows where it came from</SectionLabel>
            {item.fields.map((f) => (
              <FieldRow
                field={f}
                key={f.label}
                onToggle={() => setPopOpen(popOpen === f.label ? null : f.label)}
                popOpen={popOpen === f.label}
              />
            ))}
          </Surface>
          <Surface gap="$3" padding="$4">
            <div className="flex items-center justify-between gap-2">
              <SectionLabel>Files</SectionLabel>
              <Action onPress={() => setSection("files")} variant="link">
                All files
              </Action>
            </div>
            {item.files?.slice(0, 1).map((f) => (
              <FileCard file={f} key={f.path} />
            ))}
            {!series && (item.files?.length ?? 0) > 1 ? (
              <Text textRole="metadata" tone="warning">
                + a stray copy · see Files
              </Text>
            ) : null}
          </Surface>
          {series ? (
            <div className="flex flex-col gap-2">
              <SectionLabel>Seasons</SectionLabel>
              <SeasonList item={item} />
            </div>
          ) : null}
          <Surface gap="$2" padding="$4">
            <SectionLabel>Artwork</SectionLabel>
            {item.artwork.map((a) => (
              <div className="flex items-center gap-2" key={a.role}>
                <div className="w-32">
                  <Text textRole="compact">{a.role}</Text>
                </div>
                <SourceChip source={a.source} />
              </div>
            ))}
          </Surface>
        </div>
      ) : null}
      {section === "files" ? files : null}
      {section === "episodes" ? <SeasonList item={item} /> : null}
      {section === "usedby" ? usedBy : null}
    </div>
  );
};

export { ItemPage };
