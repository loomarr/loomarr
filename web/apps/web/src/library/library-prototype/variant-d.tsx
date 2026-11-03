// PROTOTYPE — throwaway (#2151 step 2). Variant D, "Appliance": an artwork-first Library in plain
// words. A shelf of posters, a "Needs you" strip, title pages that look like title pages.
import { useState } from "react";
import type { VariantProps } from "./prototype-state";
import { CollectionEditor } from "./variant-d-collections";
import { type Kind, LibraryPage } from "./variant-d-library";
import { TitlePage } from "./variant-d-title";

const VariantD = ({ screen, scan, go }: VariantProps) => {
  const [kind, setKind] = useState<Kind>("all");
  const [q, setQ] = useState("");

  if (screen === "item-series" || screen === "item-movie" || screen === "provenance") {
    return <TitlePage go={go} key={screen} screen={screen} />;
  }
  if (screen === "collection-edit") return <CollectionEditor go={go} />;
  return <LibraryPage go={go} kind={kind} q={q} scan={scan} screen={screen} setKind={setKind} setQ={setQ} />;
};

export { VariantD };
