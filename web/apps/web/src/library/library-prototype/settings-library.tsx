// PROTOTYPE — throwaway (#2151 step 2). Settings → Library (decision 29): health strip, scan roots,
// identity sources, metadata. Framed like the real Settings leaves (context nav + page header).
import {
  Action,
  Disclosure,
  Field,
  FieldHint,
  Icon,
  icons,
  SelectControl,
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
import { FolderCard, Line, Stack } from "./scan-health-parts";

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
  <FolderCard
    root={root}
    using={confirmed}
    checked={confirmed && root.walked !== "not yet" ? root.walked : "Not checked yet"}
    actions={
      <>
        {confirmed ? (
          root.readable ? null : (
            <Action variant="secondary">Check again</Action>
          )
        ) : (
          <Action onPress={onConfirm} variant="secondary">
            Use this folder
          </Action>
        )}
        {confirmed ? (
          <Action variant="ghost" onPress={onRemove}>
            Stop using
          </Action>
        ) : null}
      </>
    }
  />
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
      <Section title="Status">
        <ScanHealth status={view} density="full" />
      </Section>

      <Section
        title="Media folders"
        description="Loomarr looks for your shows and movies in these folders. It found them through Emby, Sonarr and Radarr."
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
                <Field
                  label="Where is the folder?"
                  value={draft}
                  onChangeText={setDraft}
                  placeholder="/media/…"
                />
              </Stack>
              <Action
                tone="primary"
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
                Add folder
              </Action>
              <Action variant="ghost" onPress={() => setAdding(false)}>
                Cancel
              </Action>
            </Line>
          ) : (
            <Line>
              <Action variant="outline" icon="plus" onPress={() => setAdding(true)}>
                Add a folder
              </Action>
            </Line>
          )}
        </Stack>
        <Disclosure
          label="Folders have different names in other apps"
          expanded={mapOpen}
          onExpandedChange={setMapOpen}
        >
          <Stack gap="control" paddingTop="inline">
            <Text textRole="body" tone="secondary">
              Emby, Sonarr and Radarr each call these folders something different. This matches them up. Only
              change it if something looks wrong.
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

      <Section
        title="Sonarr and Radarr"
        description="They know your shows and movies by name. Let them tell Loomarr what's what."
      >
        <Stack gap="control">
          {IDENTITY_SOURCES.map((source, i) => (
            <Surface key={source.name} padding="section" gap="control" borderRadius="control">
              <Line gap="control">
                <Toggle
                  kind="switch"
                  labelHidden
                  label={source.name === "Sonarr" ? "Let Sonarr name my shows" : "Let Radarr name my movies"}
                  checked={sources[i] ?? false}
                  onCheckedChange={(on) => setSources((prev) => prev.map((v, j) => (j === i ? on : v)))}
                />
                <Text textRole="body" strong>
                  {source.name === "Sonarr" ? "Let Sonarr name my shows" : "Let Radarr name my movies"}
                </Text>
              </Line>
              <Line gap="control">
                <Line gap="inline">
                  <Icon
                    decorative
                    glyph={icons.success}
                    size="compact"
                    tone={source.ok ? "success" : "warning"}
                  />
                  <Text textRole="body">{source.name} tells Loomarr about new downloads</Text>
                  <Text textRole="body" tone="muted">
                    · {source.webhook}
                  </Text>
                </Line>
                <Action variant="outline" onPress={() => setTested((prev) => new Set(prev).add(source.name))}>
                  Send a test
                </Action>
                {tested.has(source.name) ? (
                  <Text textRole="metadata" tone="success">
                    Test message received ✓
                  </Text>
                ) : null}
              </Line>
            </Surface>
          ))}
          <FieldHint>Where Sonarr and Radarr live is set up in Settings → Connections.</FieldHint>
        </Stack>
      </Section>

      <Section title="Details" description="Descriptions, posters and ratings for your titles.">
        <Line gap="section" alignItems="flex-start">
          <Stack gap="inline" flexBasis={220} flexGrow={1}>
            <Text textRole="label">Details language</Text>
            <SelectControl
              appearance="field"
              label="Details language"
              options={LANGUAGES}
              value={language}
              open={openSelect === "language"}
              onOpenChange={(o) => setOpenSelect(o ? "language" : undefined)}
              onValueChange={setLanguage}
            />
            <FieldHint>{METADATA.languageFrom}</FieldHint>
          </Stack>
          <Stack gap="inline" flexBasis={220} flexGrow={1}>
            <Text textRole="label">Ratings country</Text>
            <SelectControl
              appearance="field"
              label="Ratings country"
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
            Update all details
          </Action>
          {refreshed ? (
            <Text textRole="metadata" tone="secondary">
              On it — details update in the background.
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
    <PageHeader title="Library" description="Where your media lives and how Loomarr keeps up with it." />
    <div className="flex-1 overflow-auto">
      <Body key={scan} scan={scan} />
    </div>
  </div>
);

export { SettingsLibrary };
