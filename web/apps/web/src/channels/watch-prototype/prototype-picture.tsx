// PROTOTYPE — throwaway (#2088 step 0). A stand-in "frame of video" drawn from design tokens (no
// photos, no generated art): a dusk seascape with a lighthouse, so overlay legibility is judged
// against a picture with real light and dark areas rather than a flat field.
const PrototypePicture = () => (
  <div aria-hidden className="absolute inset-0 overflow-hidden">
    <div className="absolute inset-0 bg-[linear-gradient(180deg,#1b2a4a_0%,#4a3b6b_38%,#c8743f_58%,#f2b36b_62%,#20304a_63%,#0d1626_100%)]" />
    <div className="absolute top-[30%] left-[62%] aspect-square w-[7%] rounded-full bg-[#ffd9a0] opacity-80 blur-[2px]" />
    <div className="absolute top-[36%] left-[18%] h-[27%] w-[2.2%] bg-[linear-gradient(180deg,#e9e2d0,#8a8478)]" />
    <div className="absolute top-[33%] left-[17.4%] h-[4%] w-[3.4%] rounded-t-sm bg-[#3a3f4a]" />
    <div className="absolute top-[34%] left-[19.1%] h-[1.5%] w-[24%] origin-left -rotate-6 bg-[linear-gradient(90deg,rgba(255,236,170,.75),transparent)] blur-[3px]" />
    <div className="absolute top-[63%] right-0 left-0 h-[37%] bg-[repeating-linear-gradient(180deg,rgba(255,255,255,.05)_0_2px,transparent_2px_9px)]" />
  </div>
);

export { PrototypePicture };
