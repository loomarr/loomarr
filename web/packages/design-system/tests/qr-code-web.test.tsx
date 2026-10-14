import { QrCode } from "@loomarr/design-system/qr-code-web";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

describe("Loomarr web QR code", () => {
  it("shares the accessible high-contrast matrix and protected brand treatment", () => {
    const markup = renderToStaticMarkup(
      <QrCode
        accessibilityLabel="Scan this Loomarr invitation"
        size={196}
        value="https://loomarr.example/join#grant=fake-test-grant"
      />,
    );

    expect(markup).toContain('aria-label="Scan this Loomarr invitation"');
    expect(markup).toContain('role="img"');
    expect(markup).toContain('<path d="');
    expect(markup).toContain("#0B0C0E");
    expect(markup).toContain("#F7F8FA");
    expect(markup).toContain('aria-hidden="true"');
    expect(markup).toContain("#FFB020");
    expect(markup).toContain("#8B93A3");
  });
});
