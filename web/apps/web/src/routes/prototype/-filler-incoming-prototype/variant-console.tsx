import { Check, ChevronDown, CircleAlert, Search, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const rows: [string, string, string, string, string, string, string, string, string][] = [
  ["Coca-Cola Summer", "Commercial", "92", "Pass", "Review", "Pass", "Pass", "Pass", "Spoken conflict"],
  ["KCPQ ident", "Station ID", "87", "Pass", "Pass", "Pass", "Pass", "Pass", "Ready"],
  ["Hot Wheels", "Commercial", "78", "Review", "Pass", "Pass", "Hold", "Pass", "Rights expired"],
  ["Dealer closeout", "Commercial", "64", "Pass", "Pass", "Review", "Pass", "Pass", "Written claim"],
  ["Saturday morning reel", "Compilation", "—", "—", "—", "—", "—", "—", "Needs 3 cuts"],
  ["Community PSA", "PSA", "96", "Pass", "Pass", "Pass", "Pass", "Pass", "Ready"],
];

const tone = (value: string) =>
  value === "Pass"
    ? "text-lock"
    : value === "Review" || value === "Hold"
      ? "text-caution"
      : "text-muted-foreground";

const VariantConsole = () => (
  <div className="flex flex-col gap-4 pb-20">
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div>
        <Badge variant="suggest">Prototype C</Badge>
        <h1 className="mt-2 font-semibold text-2xl">Triage the corpus like an operations console</h1>
        <p className="mt-1 max-w-3xl text-muted-foreground text-sm">
          Dense, sortable evidence for large batches. Select a row only when it deserves deeper playback
          review.
        </p>
      </div>
      <div className="flex gap-2">
        <Button variant="outline">Export evidence</Button>
        <Button>Review selected</Button>
      </div>
    </header>

    <section className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex flex-wrap items-center gap-2 border-border border-b p-3">
        <div className="relative min-w-56 flex-1 sm:max-w-sm">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input className="pl-9" placeholder="Search incoming filler" readOnly />
        </div>
        {["Needs review · 12", "Source · All", "Kind · All", "Oldest first"].map((label) => (
          <Button key={label} variant="outline" size="sm">
            {label}
            <ChevronDown className="size-3" />
          </Button>
        ))}
      </div>

      <div className="grid grid-cols-4 gap-px border-border border-b bg-border sm:grid-cols-8">
        {[
          ["12", "Review"],
          ["7", "Preparing"],
          ["8", "Blocked"],
          ["284", "Admitted"],
        ].map(([value, label]) => (
          <div key={label} className="col-span-1 bg-card p-3 sm:col-span-2">
            <p className="font-mono text-xl">{value}</p>
            <p className="text-muted-foreground text-xs">{label}</p>
          </div>
        ))}
      </div>

      <div className="overflow-x-auto">
        <table className="w-full min-w-[980px] border-collapse text-sm">
          <thead className="bg-muted/40 text-left text-muted-foreground text-xs">
            <tr>
              <th className="w-10 p-3">
                <input type="checkbox" aria-label="Select all" />
              </th>
              <th className="p-3 font-medium">Clip</th>
              <th className="p-3 font-medium">Kind</th>
              <th className="p-3 font-medium">Tags</th>
              <th className="p-3 font-medium">Visual</th>
              <th className="p-3 font-medium">Spoken</th>
              <th className="p-3 font-medium">Written</th>
              <th className="p-3 font-medium">Rights</th>
              <th className="p-3 font-medium">Playback</th>
              <th className="p-3 font-medium">Decision</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-border">
            {rows.map(([name, kind, score, visual, spoken, written, rights, playback, decision]) => (
              <tr key={name} className="hover:bg-muted/30">
                <td className="p-3">
                  <input type="checkbox" aria-label={`Select ${name}`} />
                </td>
                <td className="p-3">
                  <p className="font-medium">{name}</p>
                  <p className="text-muted-foreground text-xs">0:30 · Archive.org</p>
                </td>
                <td className="p-3">{kind}</td>
                <td className="p-3 font-mono">{score}</td>
                {(
                  [
                    ["visual", visual],
                    ["spoken", spoken],
                    ["written", written],
                    ["rights", rights],
                    ["playback", playback],
                  ] satisfies [string, string][]
                ).map(([axis, value]) => (
                  <td key={`${name}-${axis}`} className={`p-3 font-medium text-xs ${tone(value)}`}>
                    {value}
                  </td>
                ))}
                <td className="p-3">
                  <span className="flex items-center gap-1.5">
                    {decision === "Ready" ? (
                      <Check className="size-3.5 text-lock" />
                    ) : decision.includes("Needs") ? (
                      <CircleAlert className="size-3.5 text-caution" />
                    ) : (
                      <X className="size-3.5 text-caution" />
                    )}
                    {decision}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
    <p className="text-muted-foreground text-xs">
      Showing 6 representative rows · Prototype actions do not write data
    </p>
  </div>
);

export { VariantConsole };
