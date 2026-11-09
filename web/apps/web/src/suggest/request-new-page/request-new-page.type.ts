import type { OriginationSearch } from "../origination-flow";

type RequestNewPageProps = {
  // The route's search params, which hold the flow's state: the Job it follows, the manual builder,
  // a seeded description or channel idea. No params is the empty describe form.
  search: OriginationSearch;
};

export type { RequestNewPageProps };
