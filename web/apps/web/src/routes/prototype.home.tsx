// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// /prototype/home?variant=A|B|C&state=normal|first-run|downloading&device=desktop|phone&tint=neutral|era
// Admin additions on C (#2785): &admin=1&layout=A|B|C&needs=some|zero&watching=some|none
import { createFileRoute } from "@tanstack/react-router";
import { AdminSwitcher } from "@/prototype-direction/home/admin";
import { type AdminSearch, parseAdminSearch } from "@/prototype-direction/home/admin-data";
import { AppChrome } from "@/prototype-direction/home/chrome";
import { HOME_STATES, type HomeState } from "@/prototype-direction/home/stub-data";
import { VariantA } from "@/prototype-direction/home/variant-a";
import { VariantB } from "@/prototype-direction/home/variant-b";
import { VariantC } from "@/prototype-direction/home/variant-c";
import { ProtoFrame, type ProtoSearch, ProtoSwitcher, parseSearch } from "@/prototype-direction/shared";

const VARIANTS = [
  { key: "A", name: "Today" },
  { key: "B", name: "Hero" },
  { key: "C", name: "Channel wall" },
] as const;

const parseProto = parseSearch(VARIANTS, HOME_STATES);

const PrototypeHome = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const state = search.state as HomeState;
  const change = (next: Partial<ProtoSearch & AdminSearch>) =>
    void navigate({ search: (prev) => ({ ...prev, ...next }) as typeof prev, replace: true });
  const admin = search.variant === "C" && search.admin === "1";
  return (
    <>
      <ProtoSwitcher variants={VARIANTS} states={HOME_STATES} search={search} onChange={change} />
      {admin && <AdminSwitcher search={search} onChange={change} />}
      <ProtoFrame device={search.device}>
        <AppChrome device={search.device}>
          {search.variant === "C" ? (
            <VariantC state={state} device={search.device} tint={search.tint} admin={search} />
          ) : search.variant === "B" ? (
            <VariantB state={state} device={search.device} tint={search.tint} />
          ) : (
            <VariantA state={state} device={search.device} tint={search.tint} />
          )}
        </AppChrome>
      </ProtoFrame>
    </>
  );
};

const Route = createFileRoute("/prototype/home")({
  validateSearch: (raw: Record<string, unknown>): ProtoSearch & AdminSearch => ({
    ...parseProto(raw),
    ...parseAdminSearch(raw),
  }),
  component: PrototypeHome,
});

export { Route };
