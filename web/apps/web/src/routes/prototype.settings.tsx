// PROTOTYPE — throwaway (Direction mocks, 2026-11-08, #2786)
// /prototype/settings?variant=A|B|C&state=some-issues|all-ready&device=desktop|phone&open=<card id>
// The Settings index as seven cards plus Advanced. `open` (connections, ai, channels, filler,
// people, notifications, server) opens a card in A and B; C lists its pages inline.
import { createFileRoute } from "@tanstack/react-router";
import { AppChrome } from "@/prototype-direction/home/chrome";
import {
  cardById,
  SETTINGS_STATES,
  SETTINGS_VARIANTS,
  type SettingsState,
} from "@/prototype-direction/settings/data";
import { SettingsIndex } from "@/prototype-direction/settings/settings-index";
import { ProtoFrame, type ProtoSearch, ProtoSwitcher, parseSearch } from "@/prototype-direction/shared";

const parseProto = parseSearch(SETTINGS_VARIANTS, SETTINGS_STATES);

type SettingsSearch = ProtoSearch & { open?: string };

const PrototypeSettings = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const change = (next: Partial<SettingsSearch>) =>
    void navigate({ search: (prev) => ({ ...prev, ...next }) as typeof prev, replace: true });
  return (
    <>
      <ProtoSwitcher
        variants={SETTINGS_VARIANTS}
        states={SETTINGS_STATES}
        search={search}
        onChange={change}
      />
      <ProtoFrame device={search.device}>
        <AppChrome device={search.device} current="Settings">
          <SettingsIndex
            variant={search.variant}
            state={search.state as SettingsState}
            phone={search.device === "phone"}
            open={cardById(search.open)}
            onOpen={(open) => change({ open })}
          />
        </AppChrome>
      </ProtoFrame>
    </>
  );
};

const Route = createFileRoute("/prototype/settings")({
  validateSearch: (raw: Record<string, unknown>): SettingsSearch => ({
    ...parseProto(raw),
    open: cardById(typeof raw.open === "string" ? raw.open : undefined)?.id,
  }),
  component: PrototypeSettings,
});

export { Route };
