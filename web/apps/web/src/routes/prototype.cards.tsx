// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// /prototype/cards — one card grammar for six list layouts? A specimen sheet per variant.
import { createFileRoute } from "@tanstack/react-router";
import { Specimen, STATES, VARIANTS } from "@/prototype-direction/cards/specimen";
import { ProtoFrame, ProtoSwitcher, parseSearch } from "@/prototype-direction/shared";

const CardsPrototype = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  return (
    <>
      <ProtoSwitcher
        variants={VARIANTS}
        states={STATES}
        search={search}
        onChange={(next) =>
          navigate({ search: (prev) => ({ ...prev, ...next }) as typeof prev, replace: true })
        }
      />
      <ProtoFrame device={search.device}>
        <Specimen
          variant={search.variant}
          state={search.state}
          phone={search.device === "phone"}
          tint={search.tint}
        />
      </ProtoFrame>
    </>
  );
};

const Route = createFileRoute("/prototype/cards")({
  validateSearch: parseSearch(VARIANTS, STATES),
  component: CardsPrototype,
});

export { Route };
