// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// /prototype/guide?variant=A|B|C&state=normal|shorts|downloading&device=desktop|phone&tint=neutral|era
import { createFileRoute } from "@tanstack/react-router";
import { useMemo, useState } from "react";
import { buildGuide, STATES, type Zoom } from "@/prototype-direction/guide/data";
import { GuideA } from "@/prototype-direction/guide/guide-a";
import { GuideB } from "@/prototype-direction/guide/guide-b";
import { GuideC } from "@/prototype-direction/guide/guide-c";
import { ProtoFrame, ProtoSwitcher, parseSearch } from "@/prototype-direction/shared";

const VARIANTS = [
  { key: "A", name: "Today" },
  { key: "B", name: "Colour blocks + now strip" },
  { key: "C", name: "Now / next list" },
] as const;

const GuidePrototype = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const [zoomPick, setZoomPick] = useState<Zoom | undefined>();
  // The hard case by default: 2 hours on a phone, 4 on a desktop.
  const zoom = zoomPick ?? (search.device === "phone" ? 120 : 240);
  const { eras, layout } = useMemo(
    () => buildGuide(search.state as (typeof STATES)[number], zoom),
    [search.state, zoom],
  );
  const props = {
    device: search.device,
    eras,
    layout,
    onZoom: setZoomPick,
    state: search.state as (typeof STATES)[number],
    tint: search.tint,
    zoom,
  };
  return (
    <>
      <ProtoFrame device={search.device}>
        {search.variant === "B" ? (
          <GuideB key={`${search.state}${zoom}`} {...props} />
        ) : search.variant === "C" ? (
          <GuideC key={`${search.state}${zoom}`} {...props} />
        ) : (
          <GuideA key={`${search.state}${zoom}`} {...props} />
        )}
      </ProtoFrame>
      <ProtoSwitcher
        variants={VARIANTS}
        states={STATES}
        search={search}
        onChange={(next) => void navigate({ replace: true, search: (prev) => ({ ...prev, ...next }) })}
      />
    </>
  );
};

const Route = createFileRoute("/prototype/guide")({
  component: GuidePrototype,
  validateSearch: parseSearch(VARIANTS, STATES),
});

export { Route };
