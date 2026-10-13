type GuidePageProps = {
  // Seeds the inline describe panel and opens it on arrival. The wizard's guided first channel
  // hands off with `?intent=` (§13's blank-page killer); the route forwards it here. Absent on
  // an ordinary visit, where the page opens as a plain grid.
  initialIntent?: string;
  // Opaque Job id used to resume an authorized recovery flow from My Requests.
  initialJobId?: string;
  // Opens the (empty) describe panel on arrival — the "Request a channel" door the Requests page's
  // empty state links to (`?new=1`).
  openOnArrival?: boolean;
  // Opens straight into the manual/AI-off builder (#1817, G2), skipping the AI describe form
  // even when AI is configured — Home's Channel ideas "Describe your own channel" (`?manual=1`)
  // explicitly chose this path, so retrying AI first would second-guess that choice.
  openManualOnArrival?: boolean;
  // Seeds the manual builder from one channel idea — Home's "Edit first" (`?manual=1&ideaId=`).
  initialIdeaId?: string;
};

export type { GuidePageProps };
