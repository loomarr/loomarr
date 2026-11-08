// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// /prototype/home?variant=A|B|C&state=normal|first-run|downloading&device=desktop|phone&tint=neutral|era
import { createFileRoute } from "@tanstack/react-router";
import { AppChrome } from "@/prototype-direction/home/chrome";
import { HOME_STATES, type HomeState } from "@/prototype-direction/home/stub-data";
import { VariantA } from "@/prototype-direction/home/variant-a";
import { VariantB } from "@/prototype-direction/home/variant-b";
import { VariantC } from "@/prototype-direction/home/variant-c";
import { ProtoFrame, ProtoSwitcher, parseSearch } from "@/prototype-direction/shared";

const VARIANTS = [
  { key: "A", name: "Today" },
  { key: "B", name: "Hero" },
  { key: "C", name: "Channel wall" },
] as const;

const PrototypeHome = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const state = search.state as HomeState;
  const Variant = search.variant === "B" ? VariantB : search.variant === "C" ? VariantC : VariantA;
  return (
    <>
      <ProtoSwitcher
        variants={VARIANTS}
        states={HOME_STATES}
        search={search}
        onChange={(next) =>
          void navigate({ search: (prev) => ({ ...prev, ...next }) as typeof prev, replace: true })
        }
      />
      <ProtoFrame device={search.device}>
        <AppChrome device={search.device}>
          <Variant state={state} device={search.device} tint={search.tint} />
        </AppChrome>
      </ProtoFrame>
    </>
  );
};

const Route = createFileRoute("/prototype/home")({
  validateSearch: parseSearch(VARIANTS, HOME_STATES),
  component: PrototypeHome,
});

export { Route };
