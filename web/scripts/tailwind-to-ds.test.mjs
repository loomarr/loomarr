import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { transform } from "./codemods/tailwind-to-ds.mjs";

const IMPORT = 'import { Stack } from "@loomarr/design-system";\n';

// Each case is a source before and after. `after: null` means the source must come back unchanged.
const run = (before) => transform(before, "case.tsx").output;
const expectChange = (before, after) => assert.equal(run(before), after);
const expectUntouched = (before) => assert.equal(run(before), before);

describe("a div that is a flex container becomes a Stack", () => {
  it("moves a column with a gap, and adds the import", () => {
    expectChange(
      `const A = () => <div className="flex flex-col gap-2">x</div>;\n`,
      `${IMPORT}const A = () => <Stack gap="2">x</Stack>;\n`,
    );
  });

  it("adds the Stack to an existing design-system import", () => {
    expectChange(
      `import { Text } from "@loomarr/design-system";\nconst A = () => <div className="flex flex-col gap-3">x</div>;\n`,
      `import { Text, Stack } from "@loomarr/design-system";\nconst A = () => <Stack gap="3">x</Stack>;\n`,
    );
  });

  it("a former row says direction=row and keeps what it cannot map", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex items-center gap-1.5 rounded-md border">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack align="center" className="rounded-md border" direction="row" gap="1.5">x</Stack>;\n`,
    );
  });

  it("keeps the element through render, and closes the same tag", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <ul className="flex flex-col gap-2"><li className="flex gap-2">a</li></ul>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack gap="2" render="ul"><Stack direction="row" gap="2" render="li">a</Stack></Stack>;\n`,
    );
  });

  it("moves padding and margin to pixel props, collapsing equal sides", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex flex-col p-4 px-2 mt-3 -mb-1 ml-auto">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack marginBottom={-4} marginLeft="auto" marginTop={12} paddingHorizontal={8} paddingVertical={16}>x</Stack>;\n`,
    );
  });

  it("turns flex-1 into basis, grow and shrink", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex min-w-0 flex-1 flex-col">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack flexBasis="0%" flexGrow={1} flexShrink={1} minWidth={0}>x</Stack>;\n`,
    );
  });

  it("gives a min-w-0 container the shrink it had as a div", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex min-w-0 flex-col">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack flexShrink={1} minWidth={0}>x</Stack>;\n`,
    );
  });

  it("maps shrink-0, w-full and h-full", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex flex-col shrink-0 w-full h-full">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack flexShrink={0} height="100%" width="100%">x</Stack>;\n`,
    );
  });

  it("maps sm/md/lg forms of direction and gap", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex flex-col gap-2 sm:flex-row sm:gap-4">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack direction={{ base: "column", sm: "row" }} gap={{ base: "2", sm: "4" }}>x</Stack>;\n`,
    );
  });

  it("moves min-height to a prop, because the View base rule beats the class", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex min-h-screen flex-col">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack minHeight="100vh">x</Stack>;\n`,
    );
  });

  it("keeps a comment inside the opening tag", () => {
    const source = `import { Stack } from "@loomarr/design-system";\nconst A = () => (\n  <div\n    // the list scrolls\n    className="flex flex-col gap-2"\n    id="x"\n  >\n    x\n  </div>\n);\n`;
    const output = run(source);
    assert.match(output, /\/\/ the list scrolls/);
    assert.match(output, /<Stack/);
    assert.match(output, /gap="2"/);
    assert.doesNotMatch(output, /className/);
  });
});

describe("what a div conversion refuses", () => {
  it("leaves a grid alone", () => {
    expectUntouched(`const A = () => <div className="grid grid-cols-2 gap-2">x</div>;\n`);
  });

  it("leaves a div that is not a flex container", () => {
    expectUntouched(`const A = () => <div className="min-w-0 flex-1">x</div>;\n`);
  });

  it("leaves an element whose display changes with the width", () => {
    expectUntouched(`const A = () => <div className="hidden sm:flex flex-col">x</div>;\n`);
  });

  it("leaves a hard class it cannot map, since the Stack would override it", () => {
    expectUntouched(`const A = () => <div className="flex flex-col gap-10">x</div>;\n`);
    expectUntouched(`const A = () => <div className="flex justify-around">x</div>;\n`);
    expectUntouched(`const A = () => <div className="flex min-h-[200px] flex-col">x</div>;\n`);
  });

  it("leaves a class with a state variant or an unknown responsive prefix", () => {
    expectUntouched(`const A = () => <div className="flex flex-col @md:flex-row">x</div>;\n`);
    expectUntouched(`const A = () => <div className="flex flex-col hover:gap-4">x</div>;\n`);
  });

  it("leaves a dynamic className, an event handler and a spread", () => {
    expectUntouched(`const A = ({ c }) => <div className={cn("flex flex-col", c)}>x</div>;\n`);
    expectUntouched(`const A = () => <div className="flex flex-col" onClick={go}>x</div>;\n`);
    expectUntouched(`const A = (p) => <div className="flex flex-col" {...p}>x</div>;\n`);
  });

  it("does not take a name another module already owns", () => {
    expectUntouched(
      `import { Stack } from "other";\nconst A = () => <div className="flex flex-col">x</div>;\n`,
    );
  });

  it("keeps a margin it cannot express", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="flex flex-col mt-2 sm:mt-4">x</div>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack className="mt-2 sm:mt-4">x</Stack>;\n`,
    );
  });
});

describe("a class on an existing Stack", () => {
  it("moves padding and margin", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack className="border p-6 mt-2" gap="2">x</Stack>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack className="border" gap="2" marginTop={8} padding={24}>x</Stack>;\n`,
    );
  });

  it("moves the three classes the Stack already agrees with", () => {
    expectChange(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack className="min-w-0 min-h-0 shrink-0">x</Stack>;\n`,
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack flexShrink={0} minHeight={0} minWidth={0}>x</Stack>;\n`,
    );
  });

  it("leaves a layout class the Stack's own props would have beaten", () => {
    expectUntouched(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack className="flex-1 items-center">x</Stack>;\n`,
    );
  });

  it("leaves a class whose prop is already set", () => {
    expectUntouched(
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack className="p-4" padding={8}>x</Stack>;\n`,
    );
  });
});

describe("typography", () => {
  it("moves size, weight and colour onto a Text role and keeps the element", () => {
    expectChange(
      `const A = () => <p className="mt-2 text-sm font-medium text-muted-foreground">x</p>;\n`,
      `import { Text } from "@loomarr/design-system";\nconst A = () => <Text as="p" className="mt-2" textRole="cardLabel" tone="secondary">x</Text>;\n`,
    );
  });

  it("maps the doc table's rows", () => {
    const row = (classes, props) =>
      assert.match(run(`const A = () => <span className="${classes}">x</span>;\n`), new RegExp(props));
    row("text-xs", 'textRole="hint"');
    row("text-xs font-medium", 'textRole="label"');
    row("font-mono text-xs", 'textRole="mono"');
    row("text-sm", 'textRole="compact"');
    row("text-sm font-semibold", 'strong textRole="cardLabel"');
    row("text-base", 'textRole="body"');
    row("text-md font-semibold", 'textRole="heading"');
    row("text-lg", 'textRole="headline"');
    row("text-xl font-semibold", 'textRole="title"');
    row("text-2xl", 'textRole="display"');
    row("text-xs uppercase tracking-wide", "shout");
  });

  it("names a tone only where the role's own differs", () => {
    assert.doesNotMatch(run(`const A = () => <p className="text-sm text-foreground">x</p>;\n`), /tone/);
    assert.match(
      run(`const A = () => <p className="text-xs font-medium text-foreground">x</p>;\n`),
      /tone="primary"/,
    );
    assert.match(run(`const A = () => <p className="text-sm text-destructive">x</p>;\n`), /tone="danger"/);
  });

  it("leaves a size and weight pair the table has no row for", () => {
    expectUntouched(`const A = () => <p className="text-lg font-semibold">x</p>;\n`);
    expectUntouched(`const A = () => <p className="text-sm font-mono">x</p>;\n`);
  });

  it("leaves leading, tracking, a state variant and an unknown colour", () => {
    expectUntouched(`const A = () => <p className="text-sm leading-none">x</p>;\n`);
    expectUntouched(`const A = () => <p className="text-sm tracking-tight">x</p>;\n`);
    expectUntouched(`const A = () => <p className="text-sm hover:text-foreground">x</p>;\n`);
    expectUntouched(`const A = () => <p className="text-sm text-primary-foreground">x</p>;\n`);
  });

  it("never puts a layout prop on a Text", () => {
    const output = run(`const A = () => <p className="text-sm mt-2 w-full">x</p>;\n`);
    assert.match(output, /className="mt-2 w-full"/);
    assert.doesNotMatch(output, /marginTop|width=/);
  });

  it("drops the classes an existing Text's inline style already overrides", () => {
    expectChange(
      `import { Text } from "@loomarr/design-system";\nconst A = () => <Text as="p" className="mt-2 text-sm font-medium text-muted-foreground" textRole="compact">x</Text>;\n`,
      `import { Text } from "@loomarr/design-system";\nconst A = () => <Text as="p" className="mt-2" textRole="compact">x</Text>;\n`,
    );
  });
});

describe("block containers, which are opt-in", () => {
  const blocks = (source) => transform(source, "case.tsx", { blocks: true }).output;

  it("changes nothing without the option", () => {
    expectUntouched(`const A = () => <div className="mt-2 p-4"><div>a</div></div>;\n`);
  });

  it("moves a block div of block children onto a column Stack", () => {
    assert.equal(
      blocks(
        `import { Stack } from "@loomarr/design-system";\nconst A = () => <div className="mt-2 min-w-0 flex-1"><div>a</div><p>b</p></div>;\n`,
      ),
      `import { Stack } from "@loomarr/design-system";\nconst A = () => <Stack flexBasis="0%" flexGrow={1} flexShrink={1} marginTop={8} minWidth={0}><div>a</div><p>b</p></Stack>;\n`,
    );
  });

  it("keeps a div with text, an inline child or a text class", () => {
    const keep = (source) => assert.equal(blocks(source), source);
    keep(`const A = () => <div className="mt-2">hello<div>a</div></div>;\n`);
    keep(`const A = () => <div className="mt-2"><span>a</span></div>;\n`);
    keep(`const A = () => <div className="mt-2 text-center"><div>a</div></div>;\n`);
    keep(`const A = () => <div className="mt-2 space-y-2"><div>a</div></div>;\n`);
  });
});

describe("the whole run", () => {
  it("is idempotent", () => {
    const source = `import { Text } from "@loomarr/design-system";
const A = () => (
  <div className="flex items-center gap-2 p-3 min-w-0">
    <p className="text-sm text-muted-foreground">a</p>
    <div className="flex flex-col gap-1">b</div>
  </div>
);
`;
    const once = run(source);
    assert.notEqual(once, source);
    assert.equal(run(once), once);
  });

  it("reports what it changed and what it left", () => {
    const { converted, left, removed } = transform(
      `const A = () => <div className="flex flex-col gap-2"><div className="grid flex">x</div><p className="text-sm leading-none">y</p></div>;\n`,
    );
    assert.equal(removed, 3);
    assert.equal(converted.length, 1);
    assert.match(left.join("\n"), /<p> type class|unmapped type class/);
  });
});
