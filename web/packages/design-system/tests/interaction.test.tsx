import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { Action, ChoiceGroup, Field, LoomarrProvider, Toggle } from "../index";

describe("shared interaction controls", () => {
  it("publishes selected and disabled action state without corrupting its label", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Action disabled selected tone="secondary">
          Current destination
        </Action>
      </LoomarrProvider>,
    );

    expect(markup).toContain("Current destination");
    expect(markup).not.toContain("&gt;Current destination");
    expect(markup).toContain('aria-disabled="true"');
    expect(markup).toContain('aria-pressed="true"');
  });

  it("owns accessible label, help, invalid, and disabled field presentation", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider theme="light">
        <Field disabled error="Enter a secure Loomarr URL." label="Server address" value="http://" />
      </LoomarrProvider>,
    );

    expect(markup).toContain("Server address");
    expect(markup).toContain('aria-label="Server address"');
    expect(markup).toContain('aria-invalid="true"');
    expect(markup).toContain('aria-disabled="true"');
    expect(markup).toContain("Enter a secure Loomarr URL.");
  });

  it("exposes checkbox, switch, and radio semantics through one universal interface", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Toggle checked label="Show episode artwork" onCheckedChange={vi.fn()} />
        <Toggle checked={false} kind="switch" label="Follow system theme" onCheckedChange={vi.fn()} />
        <ChoiceGroup
          label="Guide density"
          onValueChange={vi.fn()}
          options={[
            { label: "Comfortable", value: "comfortable" },
            { disabled: true, label: "Compact", value: "compact" },
          ]}
          value="comfortable"
        />
      </LoomarrProvider>,
    );

    expect(markup).toContain('role="checkbox"');
    expect(markup).toContain('role="switch"');
    expect(markup).toContain('role="radiogroup"');
    expect(markup.match(/role="radio"/g)).toHaveLength(2);
    expect(markup).toContain('aria-checked="true"');
    expect(markup).toContain('aria-disabled="true"');
  });
});
