import tokenArtifact from "@loomarr/tokens/tokens.json";

// Tokens — the complete generated set, as data (§2.5, §5.1a).
//
// The Palette/Typography/Spacing pages EXPLAIN the system; this one is the reference you scan
// when you need the exact name of a thing. It reads `tokens.json` — the artifact the Tailwind
// preset and theme.css are generated from — so a token that exists at build time appears here
// and one that does not, cannot.
//
// Deliberately dumb: no curation, no grouping by meaning, no commentary. The moment this page
// starts choosing what to show, it stops being a reliable index of what exists.

// A group is usually name → value, but `tintSteps` is a bare array of percentages. Both are
// rendered rather than one being skipped: an index that quietly omits a token is worse than no
// index, because it teaches you to trust it.
type TokenGroup = Record<string, string | number> | number[];

const asRows = (group: TokenGroup): [string, string | number][] =>
  Array.isArray(group) ? group.map((v, i) => [String(i), v]) : Object.entries(group);

const isColor = (v: string | number) => typeof v === "string" && v.startsWith("#");

const GroupTable = ({ name, group }: { name: string; group: TokenGroup }) => (
  <section className="flex flex-col gap-2">
    <h2 className="font-mono font-semibold text-base">{name}</h2>
    <div className="overflow-x-auto">
      <table className="w-full min-w-96 text-sm">
        <tbody>
          {asRows(group).map(([key, value]) => (
            <tr key={key} className="border-border/60 border-b last:border-b-0">
              <td className="py-1.5 pr-4 align-middle">
                <code className="font-mono text-xs">{key}</code>
              </td>
              <td className="py-1.5 pr-4 align-middle">
                <code className="font-mono text-static-400 text-xs">{String(value)}</code>
              </td>
              <td className="w-10 py-1.5 align-middle">
                {isColor(value) && (
                  <span
                    className="block size-5 rounded border border-border"
                    style={{ backgroundColor: value as string }}
                  />
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  </section>
);

const Tokens = () => {
  const groups = Object.entries(tokenArtifact) as [string, TokenGroup][];
  const total = groups.reduce((n, [, g]) => n + asRows(g).length, 0);

  return (
    <div className="flex flex-col gap-8 p-6">
      <header className="flex flex-col gap-1">
        <h1 className="font-semibold text-xl">Tokens</h1>
        <p className="max-w-2xl text-muted-foreground text-sm">
          {`All ${total} generated tokens, read from the same `}
          <code className="font-mono text-xs">tokens.json</code>
          {" the Tailwind preset and "}
          <code className="font-mono text-xs">theme.css</code>
          {" are built from. If it is not on this page, it does not exist at build time."}
        </p>
      </header>
      {groups.map(([name, group]) => (
        <GroupTable key={name} name={name} group={group} />
      ))}
    </div>
  );
};

export { Tokens };
