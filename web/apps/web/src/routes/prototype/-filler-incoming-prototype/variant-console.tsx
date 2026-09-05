import { Check, ChevronDown, CircleAlert, Search, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const rows = [
  {
    item: "Cereal compilation",
    identity: "1990s · food · 7 candidate children",
    stage: "Preparing",
    structure: "7 cuts agree",
    role: "5 verified",
    audience: "Running",
    rights: "Pass",
    playback: "Pass",
    decision: "Machine working",
  },
  {
    item: "KCPQ station ident",
    identity: "KCPQ · station ID · 1987",
    stage: "Available",
    structure: "Standalone",
    role: "Station ID",
    audience: "General pass",
    rights: "Pass",
    playback: "Pass",
    decision: "Ready",
  },
  {
    item: "Community-service child",
    identity: "Richmond Senior Center · community",
    stage: "Held",
    structure: "2 models agree",
    role: "Unclassified",
    audience: "Pass",
    rights: "Pass",
    playback: "Pass",
    decision: "Role conflict",
  },
  {
    item: "Swimwear promotion",
    identity: "Brand unresolved · apparel · 1990s",
    stage: "Held",
    structure: "Standalone",
    role: "Promotion",
    audience: "Adult only",
    rights: "Pass",
    playback: "Pass",
    decision: "Audience review",
  },
  {
    item: "Radio promotion",
    identity: "Local broadcaster · spoken spot",
    stage: "Rejected",
    structure: "Standalone",
    role: "Promotion",
    audience: "Spoken reject",
    rights: "Pass",
    playback: "Pass",
    decision: "Prohibited language",
  },
  {
    item: "Dealer closeout",
    identity: "Local dealership · automotive",
    stage: "Held",
    structure: "Standalone",
    role: "Commercial",
    audience: "Pass",
    rights: "Expired",
    playback: "Pass",
    decision: "Rights replacement",
  },
];

const tone = (value: string) => {
  if (value === "Pass" || value === "Ready" || value === "General pass") return "text-lock";
  if (
    value.includes("Held") ||
    value.includes("Unclassified") ||
    value.includes("Expired") ||
    value.includes("review") ||
    value.includes("reject") ||
    value.includes("conflict") ||
    value.includes("Prohibited")
  )
    return "text-caution";
  return "text-muted-foreground";
};

const VariantConsole = () => (
  <div className="flex flex-col gap-4 pb-20">
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <div className="flex items-center gap-2">
          <Badge variant="suggest">Prototype C</Badge>
          <Badge variant="neutral">Advanced only</Badge>
        </div>
        <h1 className="mt-2 font-semibold text-2xl">Inspect the evidence ledger</h1>
        <p className="mt-1 max-w-3xl text-muted-foreground text-sm">
          Dense corpus inspection for diagnosis and export—not the daily Filler home and never a bulk-admit
          surface.
        </p>
      </div>
      <div className="flex gap-2">
        <Button variant="outline">Export public evidence</Button>
        <Button>Open selected item</Button>
      </div>
    </header>

    <div className="flex items-start gap-2 rounded-lg border border-caution/35 bg-caution/5 p-3 text-sm">
      <CircleAlert className="mt-0.5 size-4 shrink-0 text-caution" />
      <p>
        Every axis remains independent. A green playback cell does not override missing rights, an audience
        rejection, or an unclassified role.
      </p>
    </div>

    <section className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex flex-wrap items-center gap-2 border-border border-b p-3">
        <div className="relative min-w-56 flex-1 sm:max-w-sm">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input className="pl-9" placeholder="Search source, company, product, logo, or child" readOnly />
        </div>
        {["State · All", "Stage · All", "Role · All", "Audience · All", "Oldest first"].map((label) => (
          <Button key={label} variant="outline" size="sm">
            {label}
            <ChevronDown className="size-3" />
          </Button>
        ))}
      </div>

      <div className="grid grid-cols-2 gap-px border-border border-b bg-border sm:grid-cols-5">
        {[
          ["26", "Moving"],
          ["3", "Needs you"],
          ["8", "Held"],
          ["4", "Rejected"],
          ["284", "Available"],
        ].map(([value, label]) => (
          <div key={label} className="bg-card p-3">
            <p className="font-mono text-xl">{value}</p>
            <p className="text-muted-foreground text-xs">{label}</p>
          </div>
        ))}
      </div>

      <div className="divide-y divide-border md:hidden">
        {rows.map((row) => (
          <article key={row.item} className="space-y-3 p-4">
            <div className="flex items-start gap-3">
              <input className="mt-1" type="checkbox" aria-label={`Select ${row.item}`} />
              <div className="min-w-0 flex-1">
                <div className="flex items-start justify-between gap-2">
                  <p className="font-medium text-sm">{row.item}</p>
                  <Badge
                    variant={
                      row.stage === "Available" ? "lock" : row.stage === "Preparing" ? "neutral" : "caution"
                    }
                  >
                    {row.stage}
                  </Badge>
                </div>
                <p className="mt-1 text-muted-foreground text-xs">{row.identity}</p>
              </div>
            </div>
            <dl className="grid grid-cols-2 gap-x-4 gap-y-3 border-border border-y py-3 text-xs">
              {[
                ["Structure", row.structure],
                ["Role", row.role],
                ["Audience", row.audience],
                ["Rights", row.rights],
                ["Playback", row.playback],
              ].map(([axis, value]) => (
                <div key={`${row.item}-${axis}`}>
                  <dt className="text-muted-foreground">{axis}</dt>
                  <dd className={`mt-1 font-medium ${tone(value ?? "")}`}>{value}</dd>
                </div>
              ))}
            </dl>
            <div className={`flex items-center gap-1.5 text-xs ${tone(row.decision)}`}>
              {row.decision === "Ready" ? (
                <Check className="size-3.5 text-lock" />
              ) : row.decision === "Machine working" ? (
                <CircleAlert className="size-3.5 text-muted-foreground" />
              ) : (
                <X className="size-3.5 text-caution" />
              )}
              Next: {row.decision}
            </div>
          </article>
        ))}
      </div>

      <div className="hidden overflow-x-auto md:block">
        <table className="w-full min-w-[1250px] border-collapse text-sm">
          <thead className="bg-muted/40 text-left text-muted-foreground text-xs">
            <tr>
              <th className="w-10 p-3">
                <input type="checkbox" aria-label="Select all" />
              </th>
              <th className="p-3 font-medium">Exact item</th>
              <th className="p-3 font-medium">Stage</th>
              <th className="p-3 font-medium">Structure</th>
              <th className="p-3 font-medium">Role</th>
              <th className="p-3 font-medium">Audience</th>
              <th className="p-3 font-medium">Rights</th>
              <th className="p-3 font-medium">Playback</th>
              <th className="p-3 font-medium">Next</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {rows.map((row) => (
              <tr key={row.item} className="hover:bg-muted/30">
                <td className="p-3">
                  <input type="checkbox" aria-label={`Select ${row.item}`} />
                </td>
                <td className="p-3">
                  <p className="font-medium">{row.item}</p>
                  <p className="text-muted-foreground text-xs">{row.identity}</p>
                </td>
                {[
                  ["stage", row.stage],
                  ["structure", row.structure],
                  ["role", row.role],
                  ["audience", row.audience],
                  ["rights", row.rights],
                  ["playback", row.playback],
                ].map(([axis, value]) => (
                  <td key={`${row.item}-${axis}`} className={`p-3 font-medium text-xs ${tone(value ?? "")}`}>
                    {value}
                  </td>
                ))}
                <td className={`p-3 text-xs ${tone(row.decision)}`}>
                  <span className="flex items-center gap-1.5">
                    {row.decision === "Ready" ? (
                      <Check className="size-3.5 text-lock" />
                    ) : row.decision === "Machine working" ? (
                      <CircleAlert className="size-3.5 text-muted-foreground" />
                    ) : (
                      <X className="size-3.5 text-caution" />
                    )}
                    {row.decision}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
    <p className="text-muted-foreground text-xs">
      Representative rows · Prototype actions do not write data · Private provider and rights artifacts stay
      server-side
    </p>
  </div>
);

export { VariantConsole };
