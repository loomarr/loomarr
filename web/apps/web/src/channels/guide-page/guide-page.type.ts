import type { OriginationSearch } from "@/suggest/origination-flow";

type GuidePageProps = {
  // The route's search params, which hold channel origination's state: the inline describe
  // panel is open while any of them is present (see OriginationSearch). Absent on an ordinary
  // visit, where the page opens as a plain grid.
  search: OriginationSearch;
};

export type { GuidePageProps };
