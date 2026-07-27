type GuidePageProps = {
  // Seeds the inline describe panel and opens it on arrival. The wizard's guided first channel
  // hands off with `?intent=` (§13's blank-page killer); the route forwards it here. Absent on
  // an ordinary visit, where the page opens as a plain grid.
  initialIntent?: string;
};

export type { GuidePageProps };
