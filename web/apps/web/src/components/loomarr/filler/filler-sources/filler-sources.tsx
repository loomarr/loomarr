import { formatRelative } from "@loomarr/core";
import { Badge, Button, Switch } from "@/components/ui";
import { cn } from "@/lib";
import type { FillerSourcesProps } from "./filler-sources.type";

// FillerSources — where the clip catalog comes from (§10, V28; the v2 mock's Filler → Sources).
//
// Reads a READ-MODEL, not a table: the server derives these rows from `filler.dir` plus the
// library connection and counts each row's clips from the catalog. That is why there is no
// add/remove affordance here — there is nothing to add a row TO. A source is configured by
// changing the setting it comes from, and V33 introduces the persisted registry when a remote
// source finally has state a setting cannot hold.
//
// ⚠ An UNCONFIGURED source renders as a row, not as an absence. "No drop-folder configured" is
// the answer to "why is my catalog empty", and hiding the row leaves that question unanswered —
// the operator sees two sources, concludes filler is working, and never finds the third.

// (An `ICONS` map lived here — one lucide glyph per kind. Removed: the mock identifies a source
// by its COLOURED CHIP alone, and an icon repeating what the chip already says is noise in a row
// that is otherwise dense with real information.)

// The mock's fixed-width kind chip (`sv.kindLabel` + its `KC` colour table).
//
// ⚠ Each kind is COLOUR-CODED, and the mock's hexes land exactly on existing tokens — measured
// off the rendered prototype rather than eyeballed: LIBRARY `#4CC9E8` = tune, FOLDER `#3DD68C` =
// lock, ARCHIVE `#D6409F` = suggest, PLAYLIST `#E85A5F` = onair. A tint at 14% carries the fill.
// An earlier pass drew all four as one grey `bg-muted` chip, which threw away the fastest signal
// in the list: an operator scanning ten rows reads the colour before the word.
//
// ⚠ `youtube` reads PLAYLIST because that is what an operator registers — a playlist or channel
// URL, not "YouTube the service". The mock draws the same distinction.
const KINDS: Record<string, { label: string; className: string }> = {
  folder: { label: "FOLDER", className: "bg-lock/15 text-lock" },
  library: { label: "LIBRARY", className: "bg-tune/15 text-tune" },
  archive: { label: "ARCHIVE", className: "bg-suggest/15 text-suggest-300" },
  youtube: { label: "PLAYLIST", className: "bg-onair/15 text-onair-300" },
};

// ⚠ `total` is deliberately NOT a prop any more. This header used to read "N sources · M clips";
// the mock's `svcOnLine` is "N of M on", and the CATALOG total belongs to the page header's
// `watchLine` pill. Two places reporting the catalog size is how they start disagreeing.
const FillerSources = ({
  sources,
  onFetch,
  fetching,
  onToggleEnabled,
  toggling,
  onRemove,
  removing,
  renderSearch,
  error,
  className,
}: FillerSourcesProps) => (
  <div className={cn("flex flex-col gap-3", className)}>
    {/* The mock's section header: a title, the explanation of what the switches do, and
        `svcOnLine` right-aligned against them (`align-items:flex-start`, 16px gap). */}
    <div className="flex flex-wrap items-start gap-4">
      <div className="min-w-0 flex-1">
        <h3 className="font-semibold text-sm">Where filler comes from</h3>
        <p className="mt-1 max-w-xl text-muted-foreground text-sm">
          Switch a source off and Loomarr stops scanning, searching and downloading from it. Clips already in
          the catalog stay put.
        </p>
      </div>
      {/* ⚠ `svcOnLine` is PLAIN mono text — no border, no background, no padding. Measured off
          the rendered mock rather than assumed: an earlier pass gave it a bordered pill, which is
          the treatment the PAGE HEADER's `watchLine` gets (8px radius, 8/13px padding, a pulsing
          dot). Two different elements, two different jobs; giving this one the header's chrome
          made the section compete with the page title.
          ⚠ "N of M on", not a bare count. An operator who switched two sources off wants that
          reflected — "5 sources" where three are dark is a reassuring lie. */}
      <span className="shrink-0 whitespace-nowrap font-mono text-muted-foreground text-xs">
        {`${sources.filter((s) => s.enabled).length} of ${sources.length} on`}
      </span>
    </div>

    {error && <p className="text-onair-300 text-sm">{error}</p>}

    <ul className="flex flex-col gap-2">
      {sources.map((s) => {
        return (
          <li
            // ⚠ Keyed by ID, not kind. With collections as peers (V37) several rows share a
            // kind, and a kind key would collide — React would reuse one row's DOM for another.
            key={s.id}
            // The mock recedes a switched-off row: a dimmer BORDER (`#1B1E24` against the normal
            // `#2A2E37`) plus `opacity:.45` on the name block.
            //
            // ⚠ The border is taken and the OPACITY IS NOT, and that is a deliberate deviation
            // rather than an oversight. Fading this block composites `text-muted-foreground`
            // toward the background — measured at 2.95:1 against the required 4.5:1, an axe
            // color-contrast failure the Playwright job fails on. The mock's INTENT is "this row
            // is not doing anything", and the border, the greyed switch and the `off` stat all
            // carry that without making the text unreadable. Recording it here because the next
            // person to diff this against the mock will otherwise "fix" it back.
            className={cn(
              "flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3",
              s.switchable && !s.enabled ? "border-border/40" : "border-border",
            )}
          >
            {/* ⚠ The switch renders only on a SWITCHABLE row. The media-server library row has
                nothing running behind it — §10 took the media server out of the filler path —
                so a toggle there would dim a row and change nothing, and the server refuses it
                with a 409. A control that cannot work is worse than no control. */}
            {/* ⚠ The shared `Switch` primitive, not a bare checkbox. This rendered as a 16px
                amber SQUARE where the mock draws a 34×19 green PILL — the control did the right
                thing and looked like something else entirely. It is a primitive rather than local
                markup because the same toggle belongs anywhere the app asks "is this on?", and a
                second hand-rolled copy is how two switches start disagreeing. */}
            {onToggleEnabled && s.switchable && (
              <Switch
                checked={s.enabled}
                disabled={toggling != null}
                onChange={() => onToggleEnabled(s.id, !s.enabled)}
                aria-label={`Use ${s.target}`}
              />
            )}
            {/* The mock's kind chip: 78px fixed width, 4px radius, 3/8px padding, 9.5px mono.
                ⚠ FIXED width so every row's name starts at the same x — a chip that shrinks to
                its text makes the column ragged and much harder to scan.
                ⚠ There is NO per-kind ICON beside it. The mock uses the coloured chip alone, and
                an icon that repeats what the chip already says is noise in a dense row. */}
            <span
              className={cn(
                "w-19.5 shrink-0 rounded-[4px] px-2 py-0.75 text-center font-mono text-[9.5px]",
                KINDS[s.kind]?.className ?? "bg-muted text-muted-foreground",
              )}
            >
              {KINDS[s.kind]?.label ?? s.kind.toUpperCase()}
            </span>
            {/* Name over description, per the mock: 13.5px medium on 11.5px muted, 2px apart,
                both single-line and ellipsised.
                ⚠ The NAME is the human label ("Drop folder"), not the target path — the path
                belongs in the description line beneath it. An earlier pass rendered the raw
                `target` in mono here, which made every row read as a filesystem path even when
                it was a library name or a playlist URL. */}
            <div className="flex min-w-0 flex-1 flex-col gap-0.5">
              <div className="flex min-w-0 items-center gap-2">
                <span className="truncate font-medium text-sm">{s.target}</span>
                {!s.configured && <Badge variant="caution">not configured</Badge>}
              </div>
              <span className="truncate text-muted-foreground text-xs">
                {s.switchable && !s.enabled
                  ? "Not being scanned. Clips it already found are still in your catalog."
                  : s.detail}
              </span>
            </div>
            {/* The mock's `sv.stat` — "6 clips · scanned 2m ago" as ONE line, and just "off" when
                the source is switched off.
                ⚠ The count is the honest signal that a source is doing anything: a configured
                source contributing 0 clips is a real and reportable state, usually an empty folder
                or a mount that did not come up.
                ⚠ The time is omitted rather than rendered as "never" when a source has not been
                fetched. Most sources are SCANNED, not fetched, so "never fetched" would read as a
                fault on a folder that is working exactly as intended. */}
            {/* ⚠ `text-muted-foreground` on BOTH states, not `static-500` when off. The mock
                greys the off row's stat to `#5A6170`, which measures 3.14:1 on `#0B0C0E` at
                12px — an axe `color-contrast` failure at serious impact. The word is already
                "off"; making it hard to read adds nothing, and the dimmer row border carries
                the recede. Same trade-off as the row's opacity, recorded there. */}
            <span className="shrink-0 whitespace-nowrap font-mono text-muted-foreground text-xs">
              {s.switchable && !s.enabled
                ? "off"
                : [
                    `${s.count} ${s.count === 1 ? "clip" : "clips"}`,
                    ...(s.lastFetchedAt ? [`scanned ${formatRelative(s.lastFetchedAt)}`] : []),
                  ].join(" · ")}
            </span>
            {/* The mock's licence chip: 10px mono, a 1px tinted border, 4px radius, 2/8px padding.
                ⚠ Rendered ONLY when the source declared one. Absence means UNKNOWN — about 92% of
                archive.org items declare nothing — so a chip reading "public domain" by default
                would be Loomarr asserting a legal fact nobody checked. No chip is the honest
                rendering of "we don't know".
                ⚠ Green only for an explicitly public-domain declaration; anything else is neutral,
                because "community, mixed" and "you supply it" are not permission. */}
            {s.license && (
              <span
                className={cn(
                  "shrink-0 whitespace-nowrap rounded-[4px] border px-2 py-0.5 font-mono text-[10px]",
                  /public domain/i.test(s.license)
                    ? "border-lock/30 text-lock"
                    : "border-border text-muted-foreground",
                )}
              >
                {s.license}
              </span>
            )}
            {/* ⚠ Keyed by ID, not KIND. It used to pass `s.kind`, which was unambiguous only
                while each kind had exactly one row; V38c allows many folders and many libraries,
                so a kind key would put every folder row into "Fetching…" at once and the operator
                could not tell which one was actually running. */}
            {s.fetchable && (
              <Button
                variant="ghost"
                onClick={() => onFetch(s.id)}
                disabled={fetching != null}
                aria-label={`Fetch now from ${s.target}`}
              >
                {fetching === s.id ? "Fetching…" : "Fetch now"}
              </Button>
            )}

            {/* ⚠ The NESTED remotes list was here and is gone (V37). Registered collections are
                peer rows now, so each renders through this same block — which is why a row needs
                its own fetch time below rather than inheriting a container's.

                The nesting existed for a real reason that OUTLIVED it: the derived rows describe
                CONFIGURATION, including "you could set this up but have not". The flat list keeps
                that as `configured` per row, so flattening did not delete the answer. */}
            {onRemove && s.removable && (
              <Button
                variant="ghost"
                onClick={() => onRemove(s.id)}
                disabled={removing != null}
                aria-label={`Remove ${s.target}`}
                title="Forget this source. Clips it already brought in stay in your catalog."
              >
                {removing === s.id ? "Removing…" : "Remove"}
              </Button>
            )}

            {/* The search expander, when this row has one (V35b). Inside the row, because
                searching is something you do TO a source. */}
            {renderSearch?.(s)}
          </li>
        );
      })}
    </ul>
  </div>
);

export { FillerSources };
