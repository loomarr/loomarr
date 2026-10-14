import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { Action, ChoiceGroup, Field, FieldHint, LoomarrProvider, semanticColors, Toggle } from "../index";

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

  it("can announce a checkbox's label without printing it beside content that already shows it", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <Toggle checked={false} label="Select Noir for bulk approve" labelHidden onCheckedChange={vi.fn()} />
      </LoomarrProvider>,
    );

    expect(markup).toContain('role="checkbox"');
    expect(markup).toContain('aria-label="Select Noir for bulk approve"');
    expect(markup).not.toContain(">Select Noir for bulk approve<");
  });

  // `metadata` is a data (mono) role; option and toggle descriptions are prose, so they take the
  // sans `hint` measure (12px/16px regular), never metadata's 500-weight 18px line.
  it("sets option and toggle descriptions in the sans hint measure, not the mono metadata role", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <ChoiceGroup
          label="Strength"
          onValueChange={vi.fn()}
          options={[{ description: "One episode a day.", label: "Light", value: "light" }]}
          value="light"
        />
        <Toggle checked description="Spread across the day." label="Evenings" onCheckedChange={vi.fn()} />
      </LoomarrProvider>,
    );

    for (const text of ["One episode a day.", "Spread across the day."]) {
      const tag = markup.match(new RegExp(`<[^>]*>(?=${text.replace(".", "\\.")})`))?.[0] ?? "";
      expect(tag).toContain("font_body");
      expect(tag).not.toContain("font_data");
      expect(tag).toContain("_fs-12px");
      expect(tag).toContain("_fw-400");
      expect(tag).toContain("_lh-16px");
    }
  });

  // A hint sits under a browser <input>, which points at it by id; it must be a real paragraph
  // carrying that id, or the field's accessible description silently resolves to nothing.
  it("renders a field hint as a describable paragraph in the Web forms' help measure", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <FieldHint className="mt-1" id="dsn-doc">
          Use require across the internet.
        </FieldHint>
      </LoomarrProvider>,
    );

    const hint = markup.match(/<p [^>]*>/)?.[0] ?? "";
    expect(hint).toContain('id="dsn-doc"');
    expect(hint).toContain('class="mt-1"');
    expect(hint).not.toContain("role=");
    expect(hint).toContain(`color:${semanticColors.content.secondary}`);
    expect(hint).toContain("font-size:12px");
    expect(hint).toContain("font-weight:400");
    expect(hint).toContain("line-height:16px");
  });

  it("announces a validation hint in the danger colour", () => {
    const markup = renderToStaticMarkup(
      <LoomarrProvider>
        <FieldHint id="url-error" role="alert" tone="error">
          Enter a full URL.
        </FieldHint>
      </LoomarrProvider>,
    );

    const hint = markup.match(/<p [^>]*>/)?.[0] ?? "";
    expect(hint).toContain('role="alert"');
    expect(hint).toContain('id="url-error"');
    expect(hint).toContain(`color:${semanticColors.state.danger}`);
  });
});
