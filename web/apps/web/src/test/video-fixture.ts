// A two-second, 160×90 VP8 clip as an inline data URI — the story fixture for anything that
// renders a <video>.
//
// ⚠ **Inline, never a URL.** Stories must render offline and deterministically: the visual suite
// runs against `storybook-static` with no server behind it, so a story fetching `/v1/filler/media`
// would race the snapshot and flake. This repo has already been bitten by remote images in visual
// stories for exactly that reason.
//
// VP8/WebM is deliberate: the pinned Linux Chromium used by the visual suite does not ship the
// proprietary H.264 decoder. The flat `#1A222C` field has no motion, so a paused-player snapshot is
// stable whichever frame the browser happens to have decoded. Generated with:
//
//   ffmpeg -f lavfi -i "color=c=0x1A222C:s=160x90:d=2:r=8" -c:v libvpx -deadline realtime \
//     -cpu-used 8 -b:v 35k -an tiny.webm
const TINY_WEBM =
  "data:video/webm;base64," +
  "GkXfo59ChoEBQveBAULygQRC84EIQoKEd2VibUKHgQJChYECGFOAZwH/////////EU2bdKtNu4tTq4QVSalmU6yBoU27" +
  "i1OrhBZUrmtTrIHLTbuMU6uEElTDZ1OsggEY7AEAAAAAAABoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" +
  "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAVSalmpSrXsYMPQkBN" +
  "gIxMYXZmNjMuMS4xMDJXQYxMYXZmNjMuMS4xMDIWVK5ryK4BAAAAAAAAP9eBAXPFiIw5ZTJliji4nIEAIrWcg3VuZIiB" +
  "AIaFVl9WUDiDgQEj44OEB3NZQOCQsIGguoFamoECVbCEVbmBARJUw2fWc3OfY8CAZ8iZRaOHRU5DT0RFUkSHjExhdmY2" +
  "My4xLjEwMnNzsWPAi2PFiIw5ZTJliji4Z8igRaOHRU5DT0RFUkSHk0xhdmM2My4xLjEwMiBsaWJ2cHgfQ7Z1Qc3ngQCj" +
  "yIEAAIBwBQCdASqgAFoAAIcIhYWImYSIA4ICdPJA6IrIIaqk13EOqpNdxDqqTXcQ6qk13EOqpNdtAP7+wvvgSVqzdOW" +
  "Zwrz4AKOugQB9ANEDAAUQvAAYDFFJU1uUcATEQq0BWAthWsBbCtYC2FatAP7w1oZE7kl4AKOpgQD6AHEDAAUQjAAYABi" +
  "39AwABBoACwAFgALAAWAAsABX+P71V3S9j4CjrYEBdwBxAwAFEGQAGAAYt/QMAAQaAAsABYACwAFgALAAV/j++ajqooo" +
  "/uT9EYKOtgQH0AHEDAAUQSAAYABi39AwABBoACwAFgALAAWAAsABX+P767+qiij+5P0Rgo62BAnEAcQMABRA0ABgAGL" +
  "f0DAAEGgALAAWAAsABYACwAFf4/vzek7J3peIwzrCjr4EC7gBxAwAFECQAGAAYt/QMAAQaAAsABYACwAFgALAAV/j+/" +
  "kgKwZbbFRvctFAIo7GBA2sAcQMABRAYABgAGLf0DAAEGgALAAWAAsABYACwAFf4/v7C+/dJ+p/ovXFzZDeAo7KBA+gA" +
  "sQMABRAQABgHYBAz/uaZgAg0ABYACwAFgALAAWAAr/D+/y6p+tC0IHPw2qsloB9DtnVBYueCBGWjsIEAAABxAwAFEBAA" +
  "GAAYt/QMAAQaAAsABYACwAFgALAAV/j+/y6p+tC0IHPw2qsloKOwgQB9AHEDAAUQEAAYABi39AwABBoACwAFgALAAWAA" +
  "sABX+P7/Lqn60LQgc/DaqyWgo7CBAPoAcQMABRAQABgAGLf0DAAEGgALAAWAAsABYACwAFf4/v8uqfrQtCBz8NqrJaC" +
  "jsIEBdwBxAwAFEBAAGAAYt/QMAAQaAAsABYACwAFgALAAV/j+/y6p+tC0IHPw2qsloKOwgQH0AHEDAAUQEAAYABi39Aw" +
  "ABBoACwAFgALAAWAAsABX+P7/Lqn60LQgc/DaqyWgo7CBAnEAcQMABRAQABgAGLf0DAAEGgALAAWAAsABYACwAFf4/v" +
  "8uqfrQtCBz8NqrJaCjsIEC7gBxAwAFEBAAGAAYt/QMAAQaAAsABYACwAFgALAAV/j+/y6p+tC0IHPw2qsloA==";

export { TINY_WEBM };
