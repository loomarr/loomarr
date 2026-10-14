import { useMemo } from "react";
import type { QrCodeProps } from "../qr-code";
import { matrixPath } from "../qr-code-matrix";
import { brandChroma, brandContract, semanticColors } from "../tokens";

/** Browser renderer for Loomarr's shared QR matrix and protected centre mark. */
const QrCode = ({
  accessibilityLabel = "Pair Loomarr with this QR code",
  size = 180,
  value,
}: QrCodeProps) => {
  const path = useMemo(() => matrixPath(value), [value]);
  const markSize = Math.round(size * 0.14);
  const markPadding = Math.max(3, Math.round(size * 0.018));
  const plate = markSize + markPadding * 2;
  const inset = (size - plate) / 2;

  return (
    <div style={{ height: size, position: "relative", width: size }}>
      <svg
        aria-label={accessibilityLabel}
        height={size}
        role="img"
        viewBox={`0 0 ${path.dimension} ${path.dimension}`}
        width={size}
      >
        <rect fill={semanticColors.brand.foreground} height="100%" width="100%" />
        <path d={path.commands} fill={semanticColors.brand.ground} />
      </svg>
      <svg
        aria-hidden="true"
        height={plate}
        style={{ left: inset, position: "absolute", top: inset }}
        viewBox="0 0 32 32"
        width={plate}
      >
        <defs>
          <clipPath id="loomarr-qr-web-mark">
            <rect height="28" rx="6.5" width="28" x="2" y="2" />
          </clipPath>
        </defs>
        <rect fill={semanticColors.brand.foreground} height="32" rx="7" width="32" />
        <g clipPath="url(#loomarr-qr-web-mark)">
          {brandChroma.map((color, index) => (
            <rect fill={color} height="28" key={color} width="4" x={2 + index * 4} y="2" />
          ))}
        </g>
        <rect
          fill="none"
          height="27"
          rx="6"
          stroke={brandContract.outline}
          strokeWidth="1"
          width="27"
          x="2.5"
          y="2.5"
        />
      </svg>
    </div>
  );
};

export type { QrCodeProps };
export { QrCode };
