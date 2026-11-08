// PROTOTYPE — throwaway (Direction mocks, 2026-11-08)
// Code-drawn art for variants E and F: a diner interior at night as the picture, a typographic
// wordmark standing in for the show's clear logo, and a poster stand-in for the next programme.
// No photos, no generated images, no remote URLs: every picture here is a few SVG shapes.
const toUri = (svg: string) => `data:image/svg+xml,${encodeURIComponent(svg)}`;

// ---------------------------------------------------------------------------------------------
// The picture: a diner after dark. Warm pendant lamps throw light pools on the counter, rain runs
// down a window onto a neon street, a kitchen pass glows at the back. Key shapes sit near the
// centre so a phone's portrait crop still reads as a room.

const pendant = (x: number) => `
  <path d="M${x} 0 V176" stroke="#0a0605" stroke-width="3"/>
  <polygon points="${x - 44},240 ${x + 44},240 ${x + 190},720 ${x - 190},720" fill="url(#cone)" filter="url(#b6)"/>
  <circle cx="${x}" cy="244" r="110" fill="url(#halo)"/>
  <path d="M${x - 50} 236 Q${x} 150 ${x + 50} 236 Z" fill="#150d09"/>
  <ellipse cx="${x}" cy="238" rx="50" ry="8" fill="#ffe6b8"/>
  <ellipse cx="${x}" cy="722" rx="250" ry="30" fill="url(#pool)"/>`;

const cups = (x: number, y: number) =>
  [0, 1, 2, 3, 4]
    .map(
      (i) =>
        `<rect x="${x + i * 34}" y="${y - 22}" width="24" height="22" rx="4" fill="#d9c7a6" opacity="${0.5 - i * 0.04}"/>`,
    )
    .join("");

const stool = (x: number) => `
  <rect x="${x - 7}" y="856" width="14" height="60" fill="#2a2a2e"/>
  <ellipse cx="${x}" cy="852" rx="62" ry="15" fill="#5c1a1d"/>
  <ellipse cx="${x}" cy="846" rx="62" ry="13" fill="#7d252a"/>
  <ellipse cx="${x - 14}" cy="842" rx="26" ry="4" fill="#ffd9a0" opacity="0.18"/>`;

const dinerSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="900" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice">
<defs>
  <linearGradient id="wall" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#150d09"/><stop offset="0.55" stop-color="#2a1a12"/><stop offset="1" stop-color="#140c08"/></linearGradient>
  <linearGradient id="win" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0b1626"/><stop offset="0.65" stop-color="#17304a"/><stop offset="1" stop-color="#1f4256"/></linearGradient>
  <linearGradient id="cone" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ffd9a0" stop-opacity="0.34"/><stop offset="1" stop-color="#ffb050" stop-opacity="0"/></linearGradient>
  <radialGradient id="halo"><stop offset="0" stop-color="#ffd28a" stop-opacity="0.6"/><stop offset="1" stop-color="#ffb050" stop-opacity="0"/></radialGradient>
  <radialGradient id="pool"><stop offset="0" stop-color="#ffc06a" stop-opacity="0.55"/><stop offset="1" stop-color="#ffb050" stop-opacity="0"/></radialGradient>
  <radialGradient id="pass" cx="0.5" cy="0.5" r="0.6"><stop offset="0" stop-color="#ffd698" stop-opacity="0.95"/><stop offset="1" stop-color="#e08a3a" stop-opacity="0.55"/></radialGradient>
  <radialGradient id="vig" cx="0.5" cy="0.42" r="0.78"><stop offset="0.5" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity="0.7"/></radialGradient>
  <filter id="b6"><feGaussianBlur stdDeviation="6"/></filter>
  <filter id="b14"><feGaussianBlur stdDeviation="14"/></filter>
  <filter id="b30"><feGaussianBlur stdDeviation="30"/></filter>
  <pattern id="tile" width="56" height="56" patternUnits="userSpaceOnUse"><rect width="56" height="56" fill="#2f231c"/><rect width="28" height="28" fill="#21170f"/><rect x="28" y="28" width="28" height="28" fill="#21170f"/></pattern>
</defs>
<rect width="1600" height="900" fill="url(#wall)"/>

<rect x="64" y="96" width="520" height="380" rx="6" fill="#0a0605"/>
<rect x="76" y="108" width="496" height="356" fill="url(#win)"/>
<g opacity="0.9">
  <rect x="76" y="360" width="60" height="104" fill="#0b1019"/><rect x="146" y="320" width="84" height="144" fill="#0e1520"/>
  <rect x="240" y="372" width="52" height="92" fill="#0b1019"/><rect x="394" y="338" width="76" height="126" fill="#0d141e"/><rect x="480" y="380" width="92" height="84" fill="#0b1019"/>
  <rect x="160" y="340" width="8" height="8" fill="#ffd28a" opacity="0.7"/><rect x="182" y="372" width="8" height="8" fill="#ffd28a" opacity="0.5"/><rect x="412" y="360" width="8" height="8" fill="#ffd28a" opacity="0.6"/><rect x="432" y="392" width="8" height="8" fill="#ffd28a" opacity="0.4"/>
</g>
<ellipse cx="296" cy="260" rx="150" ry="46" fill="#ff4d5a" opacity="0.55" filter="url(#b30)"/>
<rect x="206" y="236" width="180" height="48" rx="24" fill="none" stroke="#ff7b86" stroke-width="5" filter="url(#b6)"/>
<rect x="206" y="236" width="180" height="48" rx="24" fill="none" stroke="#ffb3b9" stroke-width="2"/>
<ellipse cx="470" cy="430" rx="120" ry="14" fill="#3fd0c9" opacity="0.28" filter="url(#b14)"/>
<g stroke="#a9c7e0" stroke-opacity="0.16" stroke-width="1.5">${Array.from({ length: 26 }, (_, i) => `<path d="M${88 + i * 19} ${112 + ((i * 37) % 90)} l-6 46"/>`).join("")}</g>
<rect x="320" y="108" width="8" height="356" fill="#0a0605"/>
<rect x="56" y="470" width="536" height="14" rx="3" fill="#2b1c13"/>

<rect x="940" y="124" width="340" height="206" rx="8" fill="#0a0605"/>
<rect x="954" y="138" width="312" height="178" fill="url(#pass)"/>
<rect x="954" y="138" width="312" height="178" fill="#ffd698" opacity="0.2" filter="url(#b14)"/>
<g fill="#2a160c" opacity="0.8"><rect x="990" y="214" width="46" height="102"/><rect x="1060" y="244" width="30" height="72"/><rect x="1150" y="196" width="54" height="120"/><circle cx="1013" cy="196" r="14"/><circle cx="1165" cy="178" r="15"/></g>
<rect x="944" y="322" width="332" height="14" fill="#3b2a1d"/>
<rect x="640" y="190" width="240" height="6" fill="#3b2a1d"/>${cups(660, 190)}
<rect x="640" y="268" width="240" height="6" fill="#3b2a1d"/>${cups(660, 268)}
<rect x="1330" y="160" width="190" height="250" rx="4" fill="#120b08" stroke="#3b2a1d" stroke-width="3"/>
<g stroke="#e8dcc2" stroke-opacity="0.35" stroke-width="3" stroke-linecap="round"><path d="M1356 200 h120"/><path d="M1356 236 h92"/><path d="M1356 272 h110"/><path d="M1356 308 h76"/><path d="M1356 344 h104"/></g>

<rect x="0" y="476" width="1600" height="230" fill="url(#tile)"/>
<rect x="0" y="470" width="1600" height="8" fill="#3d2b20"/>
<rect x="0" y="476" width="1600" height="230" fill="#000" opacity="0.28"/>

${pendant(480)}${pendant(800)}${pendant(1120)}

<rect x="0" y="706" width="1600" height="30" fill="#7a5238"/>
<rect x="0" y="706" width="1600" height="4" fill="#e6b583" opacity="0.7"/>
<rect x="0" y="736" width="1600" height="164" fill="#4f1a1a"/>
<rect x="0" y="788" width="1600" height="8" fill="#b9bfc7" opacity="0.5"/>
${stool(360)}${stool(800)}${stool(1240)}

<g filter="url(#b14)"><circle cx="1480" cy="90" r="34" fill="#ffb050" opacity="0.28"/><circle cx="1380" cy="60" r="20" fill="#ffd28a" opacity="0.3"/><circle cx="640" cy="60" r="26" fill="#ff7b86" opacity="0.18"/></g>
<rect width="1600" height="900" fill="url(#vig)"/>
</svg>`;

const dinerFrameUri = toUri(dinerSvg);

// Cover-fits whatever frame it sits in. Used as the player's picture and, small, as scrub previews.
const DinerFrame = () => (
  <div
    aria-hidden
    style={{
      background: `#150d09 url("${dinerFrameUri}") center/cover no-repeat`,
      inset: 0,
      position: "absolute",
    }}
  />
);

// ---------------------------------------------------------------------------------------------
// The show's logo. A media server can hold a clear-logo image per show (Emby and Plex both do) and a
// real player would set it where this wordmark is. Loomarr does not store logos yet, so this stands
// in: a drawn display wordmark, cream on transparent. It is deliberately NOT the UI font.

const wordmarkSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="560" height="132" viewBox="0 0 560 132">
<g fill="#f3e4c4">
  <path d="M0 18 H206" stroke="#f3e4c4" stroke-width="2"/><path d="M354 18 H560" stroke="#f3e4c4" stroke-width="2"/>
  <text x="280" y="27" text-anchor="middle" font-family="'Liberation Serif','Times New Roman',Georgia,serif" font-size="22" font-weight="700" letter-spacing="9" textLength="104" lengthAdjust="spacing">THE</text>
  <text x="0" y="98" font-family="'Liberation Serif','Times New Roman',Georgia,serif" font-size="82" font-weight="700" font-style="italic" textLength="560" lengthAdjust="spacingAndGlyphs">Diner Years</text>
</g>
<path d="M0 118 H258" stroke="#f3e4c4" stroke-width="2.5"/><path d="M302 118 H560" stroke="#f3e4c4" stroke-width="2.5"/>
<polygon points="280,109 282.2,114.9 288.6,115.2 283.6,119.2 285.3,125.3 280,121.8 274.7,125.3 276.4,119.2 271.4,115.2 277.8,114.9" fill="#f3e4c4"/>
</svg>`;

const wordmarkUri = toUri(wordmarkSvg);
const WORDMARK_RATIO = 560 / 132;

// ---------------------------------------------------------------------------------------------
// The next programme's poster: a noir one-sheet, portrait, a few shapes (blinds of light, a hat and
// a street lamp). Same status as the wordmark: a stand-in the real app replaces with the library's
// poster ImageDTO.

const posterSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="300" viewBox="0 0 200 300">
<defs>
  <linearGradient id="p" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#2a2f4a"/><stop offset="1" stop-color="#0d0f1a"/></linearGradient>
  <linearGradient id="b" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#ffd7a0" stop-opacity="0.8"/><stop offset="1" stop-color="#ffd7a0" stop-opacity="0.15"/></linearGradient>
  <filter id="f"><feGaussianBlur stdDeviation="2"/></filter>
</defs>
<rect width="200" height="300" fill="url(#p)"/>
<g fill="url(#b)" filter="url(#f)" transform="skewY(-12)">${[0, 1, 2, 3, 4, 5].map((i) => `<rect x="-20" y="${50 + i * 26}" width="260" height="10"/>`).join("")}</g>
<path d="M0 300 V222 Q40 196 100 204 T200 214 V300Z" fill="#06070d"/>
<ellipse cx="100" cy="176" rx="46" ry="9" fill="#06070d"/><path d="M72 176 Q76 140 100 138 Q124 140 128 176Z" fill="#06070d"/>
<rect x="72" y="166" width="56" height="5" fill="#8c2a2e"/>
<rect x="24" y="40" width="152" height="1.5" fill="#f3e4c4" opacity="0.7"/><rect x="24" y="260" width="152" height="1.5" fill="#f3e4c4" opacity="0.7"/>
</svg>`;

const detectivePosterUri = toUri(posterSvg);

// ---------------------------------------------------------------------------------------------
// Fanart (a programme's backdrop) for "A heist film", CH 9: a night vault job drawn in a few shapes,
// a steel door behind laser lines and a skyline. Landscape, 16:9, dark enough to dim further. The
// real app takes backdrops from the library (#2567 imports them as RoleBackdrop with dominantHex).

const heistFanartSvg = `<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="900" viewBox="0 0 1600 900" preserveAspectRatio="xMidYMid slice">
<defs>
  <linearGradient id="h" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="#0b1a2c"/><stop offset="1" stop-color="#05080f"/></linearGradient>
  <radialGradient id="g" cx="0.62" cy="0.5" r="0.5"><stop offset="0" stop-color="#4fa3b5" stop-opacity="0.55"/><stop offset="1" stop-color="#4fa3b5" stop-opacity="0"/></radialGradient>
  <filter id="b"><feGaussianBlur stdDeviation="3"/></filter>
</defs>
<rect width="1600" height="900" fill="url(#h)"/>
<rect width="1600" height="900" fill="url(#g)"/>
<g fill="#07101b">${[0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((i) => `<rect x="${i * 170 - 20}" y="${560 - ((i * 53) % 150)}" width="${120 + ((i * 29) % 50)}" height="400"/>`).join("")}</g>
<g transform="translate(990 440)">
  <circle r="300" fill="#16293b" stroke="#0a1420" stroke-width="14"/>
  <circle r="236" fill="#1d3448" stroke="#0a1420" stroke-width="8"/>
  <circle r="150" fill="#16293b" stroke="#6fb5c4" stroke-opacity="0.45" stroke-width="3"/>
  ${[0, 45, 90, 135].map((d) => `<rect x="-6" y="-236" width="12" height="472" fill="#0a1420" transform="rotate(${d})"/>`).join("")}
  <circle r="34" fill="#0a1420"/><circle r="14" fill="#6fb5c4" opacity="0.8"/>
</g>
<g stroke="#ff5a5f" stroke-width="3" filter="url(#b)" opacity="0.7"><path d="M0 250 L1600 470"/><path d="M0 700 L1600 310"/><path d="M300 0 L900 900"/></g>
<g stroke="#ffb3b6" stroke-width="1.2" opacity="0.85"><path d="M0 250 L1600 470"/><path d="M0 700 L1600 310"/><path d="M300 0 L900 900"/></g>
<rect width="1600" height="900" fill="#000" opacity="0.18"/>
</svg>`;

const heistFanartUri = toUri(heistFanartSvg);

// ---------------------------------------------------------------------------------------------
// The artwork's dominant colour, and what the chrome takes from it. The real ImageDTO carries
// `dominantHex`; a hue and a few fixed saturations/lightnesses turn it into a scrim, a set of solid
// surfaces and an accent, so each programme's chrome is its own and never louder than the picture.

const hexToHsl = (hex: string): [number, number, number] => {
  const n = Number.parseInt(hex.slice(1), 16);
  const r = ((n >> 16) & 255) / 255;
  const g = ((n >> 8) & 255) / 255;
  const b = (n & 255) / 255;
  const max = Math.max(r, g, b);
  const min = Math.min(r, g, b);
  const l = (max + min) / 2;
  const d = max - min;
  if (d === 0) return [0, 0, l * 100];
  const s = d / (1 - Math.abs(2 * l - 1));
  const h = max === r ? ((g - b) / d) % 6 : max === g ? (b - r) / d + 2 : (r - g) / d + 4;
  return [Math.round(((h * 60 + 360) % 360) * 10) / 10, s * 100, l * 100];
};

interface Tint {
  accent: string;
  hairline: string;
  raised: string;
  raisedHi: string;
  raisedTop: string;
  scrimFoot: string;
  scrimMid: string;
}

const tintOf = (hex: string): Tint => {
  const [h] = hexToHsl(hex);
  return {
    accent: `hsl(${h} 72% 80%)`,
    hairline: `hsl(${h} 16% 26%)`,
    raised: `hsl(${h} 22% 10%)`,
    raisedHi: `hsl(${h} 20% 16%)`,
    raisedTop: `hsl(${h} 18% 24%)`,
    scrimFoot: `hsl(${h} 40% 4% / 0.94)`,
    scrimMid: `hsl(${h} 36% 5% / 0.82)`,
  };
};

export type { Tint };
export { DinerFrame, detectivePosterUri, dinerFrameUri, heistFanartUri, tintOf, WORDMARK_RATIO, wordmarkUri };
