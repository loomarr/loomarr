// PROTOTYPE — throwaway (#2151 step 2). Settings → Library (decision 29): health strip, scan roots,
// identity sources, metadata. Framed like the real Settings leaves (context nav + page header).
import {
  Action,
  Badge,
  Disclosure,
  Field,
  FieldHint,
  SelectControl,
  StatusDot,
  Surface,
  Text,
  Toggle,
} from "@loomarr/design-system";
import { type ReactNode, useState } from "react";
import { PageHeader } from "@/components/loomarr/shell/page-header";
import { SettingsContextNav } from "@/settings/settings-context-nav";
import { IDENTITY_SOURCES, METADATA, PATH_MAP, type Root, scanStatus } from "./prototype-data";
import type { Device, ScanState } from "./prototype-state";
import { ScanHealth } from "./scan-health";
import { Line, MOUNT_HINT, RootFacts, Stack } from "./scan-health-parts";

const Section = ({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: ReactNode;
}) => (
  <Stack gap="control" role="group" aria-label={title}>
    <Stack gap="inline">
      <Text as="h2" textRole="headline">
        {title}
      </Text>
      {description ? (
        <Text textRole="body" tone="secondary">
          {description}
        </Text>
      ) : null}
    </Stack>
    {children}
  </Stack>
);

const LANGUAGES = ["English (US)", "English (UK)", "Español", "Français", "Deutsch"].map((v) => ({
  label: v,
  value: v,
}));
const COUNTRIES = ["United States", "United Kingdom", "Canada", "Australia"].map((v) => ({
  label: v,
  value: v,
}));

const RootRow = ({
  root,
  confirmed,
  onConfirm,
  onRemove,
}: {
  root: Root;
  confirmed: boolean;
  onConfirm: () => void;
  onRemove: () => void;
}) => (
  <Surface
    padding="control"
    gap="inline"
    borderRadius="control"
    borderColor={confirmed ? "borderDecorative" : "stateWarning"}
  >
    <Line gap="control">
      <Text textRole="data">{root.path}</Text>
      {confirmed ? null : <Badge tone="warning">unconfirmed</Badge>}
      <Stack marginLeft="auto" flexDirection="row" gap="inline">
        {confirmed ? (
          root.readable ? null : (
            <Action variant="outline">Re-check</Action>
          )
        ) : (
          <Action variant="suggest" onPress={onConfirm}>
            Confirm
          </Action>
        )}
        <Action variant="ghost" onPress={onRemove}>
          Remove
        </Action>
      </Stack>
    </Line>
    <RootFacts root={confirmed ? root : { ...root, walked: "not yet" }} />
    {root.readable ? null : (
      <Text textRole="metadata" tone="danger">
        {MOUNT_HINT}
      </Text>
    )}
  </Surface>
);

const Body = ({ scan }: { scan: ScanState }) => {
  const status = scanStatus(scan);
  const [confirmed, setConfirmed] = useState<ReadonlySet<string>>(
    () => new Set(status.roots.filter((r) => r.confirmed).map((r) => r.path)),
  );
  const [removed, setRemoved] = useState<ReadonlySet<string>>(new Set());
  const [added, setAdded] = useState<Root[]>([]);
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState("");
  const [mapOpen, setMapOpen] = useState(false);
  const [mapRows, setMapRows] = useState(PATH_MAP);
  const [sources, setSources] = useState(() => IDENTITY_SOURCES.map((s) => s.useAsLibrarySource));
  const [tested, setTested] = useState<ReadonlySet<string>>(new Set());
  const [language, setLanguage] = useState(METADATA.language);
  const [country, setCountry] = useState(METADATA.country);
  const [openSelect, setOpenSelect] = useState<"language" | "country" | undefined>();
  const [refreshed, setRefreshed] = useState(false);

  const roots = [...status.roots, ...added].filter((r) => !removed.has(r.path));
  // Confirming a root on a fresh install starts the import.
  const view = scan === "fresh" && confirmed.size > 0 ? scanStatus("importing") : status;

  return (
    <Stack
      flex={1}
      gap="screen"
      paddingHorizontal="section"
      paddingVertical="section"
      maxWidth={880}
      width="100%"
      alignSelf="center"
    >
      <ScanHealth status={view} density="full" />

      <Section
        title="Scan roots"
        description="Loomarr reads your media files directly from these folders. Each was found in your media server or Sonarr/Radarr."
      >
        <Stack gap="control">
          {roots.map((root) => (
            <RootRow
              key={root.path}
              root={root}
              confirmed={confirmed.has(root.path)}
              onConfirm={() => setConfirmed((prev) => new Set(prev).add(root.path))}
              onRemove={() => setRemoved((prev) => new Set(prev).add(root.path))}
            />
          ))}
          {adding ? (
            <Line gap="control" alignItems="flex-end">
              <Stack flexGrow={1} flexBasis={240}>
                <Field label="Folder path" value={draft} onChangeText={setDraft} placeholder="/media/…" />
              </Stack>
              <Action
                variant="secondary"
                disabled={draft.trim() === ""}
                onPress={() => {
                  setAdded((prev) => [
                    ...prev,
                    {
                      path: draft.trim(),
                      foundIn: ["added by hand"],
                      readable: true,
                      confirmed: false,
                      videos: 0,
                      walked: "not yet",
                    },
                  ]);
                  setDraft("");
                  setAdding(false);
                }}
              >
                Add
              </Action>
              <Action variant="ghost" onPress={() => setAdding(false)}>
                Cancel
              </Action>
            </Line>
          ) : (
            <Line>
              <Action variant="outline" icon="plus" onPress={() => setAdding(true)}>
                Add root
              </Action>
            </Line>
          )}
        </Stack>
        <Disclosure
          label="Advanced: path mappings (derived)"
          expanded={mapOpen}
          onExpandedChange={setMapOpen}
        >
          <Stack gap="control" paddingTop="inline">
            <Text textRole="body" tone="secondary">
              Derived from your media server and Sonarr/Radarr, so each one's paths resolve to the folders
              above. Edit only if a mapping is wrong.
            </Text>
            {mapRows.map((row, i) => (
              <Line key={`${row.authority}-${row.from}`} gap="control" alignItems="flex-end">
                <Stack width={72} paddingBottom="inline">
                  <Text textRole="label">{row.authority}</Text>
                </Stack>
                <Stack flexGrow={1} flexBasis={160}>
                  <Field
                    accessibilityLabel={`${row.authority} path`}
                    value={row.from}
                    onChangeText={(from) =>
                      setMapRows((prev) => prev.map((r, j) => (j === i ? { ...r, from } : r)))
                    }
                  />
                </Stack>
                <Text textRole="body" tone="muted">
                  →
                </Text>
                <Stack flexGrow={1} flexBasis={160}>
                  <Field
                    accessibilityLabel={`${row.authority} maps to`}
                    value={row.to}
                    onChangeText={(to) =>
                      setMapRows((prev) => prev.map((r, j) => (j === i ? { ...r, to } : r)))
                    }
                  />
                </Stack>
              </Line>
            ))}
          </Stack>
        </Disclosure>
      </Section>

      <Section title="Identity sources" description="Which tools tell Loomarr what each file is.">
        <Stack gap="control">
          {IDENTITY_SOURCES.map((source, i) => (
            <Surface key={source.name} padding="control" gap="inline" borderRadius="control">
              <Toggle
                kind="switch"
                label={`Use ${source.name} as library source`}
                checked={sources[i] ?? false}
                onCheckedChange={(on) => setSources((prev) => prev.map((v, j) => (j === i ? on : v)))}
              />
              <Line gap="control">
                <StatusDot
                  tone={source.ok ? "ok" : "error"}
                  label={source.ok ? "webhook working" : "webhook failing"}
                  size="compact"
                />
                <Text textRole="metadata" tone="secondary">
                  {source.name} webhook {source.webhook}
                </Text>
                <Action variant="outline" onPress={() => setTested((prev) => new Set(prev).add(source.name))}>
                  Test
                </Action>
                {tested.has(source.name) ? (
                  <Text textRole="metadata" tone="success">
                    Test event received ✓
                  </Text>
                ) : null}
              </Line>
            </Surface>
          ))}
          <FieldHint>
            Sonarr and Radarr connection details (URL, API key) live in Settings → Connections.
          </FieldHint>
        </Stack>
      </Section>

      <Section title="Metadata">
        <Line gap="section" alignItems="flex-start">
          <Stack gap="inline" flexBasis={220} flexGrow={1}>
            <Text textRole="label">Language</Text>
            <SelectControl
              appearance="field"
              label="Language"
              options={LANGUAGES}
              value={language}
              open={openSelect === "language"}
              onOpenChange={(o) => setOpenSelect(o ? "language" : undefined)}
              onValueChange={setLanguage}
            />
            <FieldHint>{METADATA.languageFrom}</FieldHint>
          </Stack>
          <Stack gap="inline" flexBasis={220} flexGrow={1}>
            <Text textRole="label">Country</Text>
            <SelectControl
              appearance="field"
              label="Country"
              options={COUNTRIES}
              value={country}
              open={openSelect === "country"}
              onOpenChange={(o) => setOpenSelect(o ? "country" : undefined)}
              onValueChange={setCountry}
            />
            <FieldHint>{METADATA.languageFrom}</FieldHint>
          </Stack>
        </Line>
        <Line gap="control">
          <Action variant="outline" onPress={() => setRefreshed(true)}>
            Refresh all metadata
          </Action>
          {refreshed ? (
            <Text textRole="metadata" tone="secondary">
              Queued — metadata refreshes in the background.
            </Text>
          ) : null}
        </Line>
      </Section>
    </Stack>
  );
};

const SettingsLibrary = ({ scan }: { scan: ScanState; device: Device }) => (
  <div className="flex h-full flex-col">
    <SettingsContextNav section="Library" />
    <PageHeader
      title="Library"
      description="Where Loomarr reads your media, who identifies it, and how it is described."
    />
    <div className="flex-1 overflow-auto">
      <Body key={scan} scan={scan} />
    </div>
  </div>
);

export { SettingsLibrary };
