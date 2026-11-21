// PROTOTYPE — throwaway (#4476 A1). /prototype/library draws the redesigned Library wall and Needs
// you inside the real AppShell from stub data (no backend, no auth). Every view is a URL:
//
//   /prototype/library?screen=wall&state=needs&role=admin&tab=all&device=desktop
//
// screen: wall | needs | nav (the phone-navigation proposal)
// state (wall): needs | clear | importing | loading | search-empty | home-video
// state (needs): several | clear | undo | ignored | proposals
// device=phone frames the same URL in a 390×844 iframe, so the shell switches to its bottom bar.
import { TabBar, Text } from "@loomarr/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { AppShell } from "@/components/loomarr/shell/app-shell";
import { NEEDS } from "@/library/library-prototype/mock-data";
import { type NeedsState, NeedsYou } from "@/library/library-prototype/mock-needs";
import { Proposal } from "@/library/library-prototype/mock-parts";
import { LibraryWall, type Role, type Tab, type WallState } from "@/library/library-prototype/mock-wall";

const SCREENS = ["wall", "needs", "nav"] as const;
type Screen = (typeof SCREENS)[number];

interface Search {
  screen: Screen;
  state: string;
  role: Role;
  tab: Tab;
  device: "desktop" | "phone";
  framed?: boolean;
}

const pick = <T extends string>(raw: unknown, allowed: readonly T[], fallback: T): T =>
  allowed.includes(raw as T) ? (raw as T) : fallback;

// PROPOSAL: Library earns a slot in the phone's bar; Requests moves into More, and its count
// rides on More. A member asks for a title from the Library's empty search ("Ask for it").
const NavProposal = ({ role }: { role: Role }) => (
  <div className="bg-static-950 flex min-h-dvh flex-col pb-24">
    <div className="p-4">
      <Proposal question="Where Library sits in the phone's navigation (under More today)">
        <Text textRole="compact" tone="secondary">
          Home · Watch · Guide · Library · More. Requests moves into More and its count shows on More.
          Asking for a title starts from a search that finds nothing.
        </Text>
      </Proposal>
    </div>
    <LibraryWall onReview={() => undefined} phone role={role} state="clear" tab="all" />
    <div className="fixed inset-x-0 bottom-0 z-30">
      <TabBar
        accessibilityLabel="Primary navigation"
        idiom="ios"
        items={[
          { value: "home", label: "Home", icon: "home" },
          { value: "watch", label: "Watch", icon: "play" },
          { value: "guide", label: "Guide", icon: "guide" },
          { value: "library", label: "Library", icon: "library" },
          { value: "more", label: "More", icon: "more", kind: "disclosure", badge: role === "admin" ? 2 : undefined },
        ]}
        onSelect={() => undefined}
        selected="library"
      />
    </div>
  </div>
);

const PrototypeLibrary = () => {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const set = (next: Partial<Search>) => void navigate({ search: (prev) => ({ ...prev, ...next }), replace: true });

  if (search.device === "phone" && !search.framed) {
    const qs = new URLSearchParams({ ...search, framed: "true" } as unknown as Record<string, string>);
    return (
      <div className="bg-static-950 flex min-h-dvh items-start justify-center py-10">
        <iframe
          className="ring-static-800 h-[844px] w-[390px] rounded-[36px] bg-black ring-8"
          src={`/prototype/library?${qs.toString()}`}
          title="Phone frame"
        />
      </div>
    );
  }

  if (search.screen === "nav") return <NavProposal role={search.role} />;

  const admin = search.role === "admin";
  const phone = search.device === "phone";
  const body =
    search.screen === "needs" && admin ? (
      <NeedsYou
        key={search.state}
        onBack={() => set({ screen: "wall", state: "needs" })}
        state={pick(search.state, ["several", "clear", "undo", "ignored", "proposals"] as NeedsState[], "several")}
      />
    ) : (
      <LibraryWall
        key={`${search.state}-${search.tab}-${search.role}`}
        onReview={() => set({ screen: "needs", state: "several" })}
        phone={phone}
        role={search.role}
        state={pick(search.state, ["needs", "clear", "importing", "loading", "search-empty", "home-video"] as WallState[], "needs")}
        tab={search.tab}
      />
    );

  const needs = search.state === "clear" ? 0 : search.state === "importing" ? 3 : NEEDS.length;
  return (
    <AppShell
      badges={admin ? { "/library": needs, "/requests": 2 } : {}}
      isAdmin={admin}
      userName={admin ? "Adela Hale" : "Rory Hale"}
      watchChannelId="c1"
    >
      {body}
    </AppShell>
  );
};

const Route = createFileRoute("/prototype/library")({
  validateSearch: (raw: Record<string, unknown>): Search => ({
    screen: pick(raw.screen, SCREENS, "wall"),
    state: typeof raw.state === "string" ? raw.state : "needs",
    role: pick(raw.role, ["admin", "member"] as const, "admin"),
    tab: pick(raw.tab, ["all", "movies", "shows", "collections"] as const, "all"),
    device: pick(raw.device, ["desktop", "phone"] as const, "desktop"),
    framed: raw.framed === true || raw.framed === "true",
  }),
  component: PrototypeLibrary,
});

export { Route };
