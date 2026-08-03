// Which section is showing. Now a PATH (`/filler`, `/filler/incoming`, `/filler/sources`),
// so the page that owns the route tells FillerPage which one it is rather than the
// component reading `?tab=` itself — three different route files render this component.
type FillerTab = "catalog" | "incoming" | "sources";

type FillerPageProps = {
  tab: FillerTab;
};

export type { FillerPageProps, FillerTab };
