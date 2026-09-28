package fillerreview

// temporalHumanReviewerTemplate stays isolated from preparation and lock logic:
// it is a complete offline application embedded in the review package.
const temporalHumanReviewerTemplate = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src 'self' file: data:; media-src 'self' file:; style-src 'unsafe-inline'; script-src 'unsafe-inline'">
  <title>Temporal filler review</title>
  <style>
    :root { color-scheme: dark; font-family: ui-sans-serif, system-ui, sans-serif; background: #101315; color: #f4f5f5; }
    * { box-sizing: border-box; }
    body { margin: 0; }
    button, input, select { font: inherit; }
    button, select, input { border: 1px solid #495057; border-radius: .45rem; background: #202529; color: inherit; padding: .55rem .7rem; }
    button { cursor: pointer; text-align: left; }
    button:hover, button:focus-visible, select:focus-visible, input:focus-visible { border-color: #76c7b7; outline: 2px solid #76c7b755; outline-offset: 1px; }
    button[aria-pressed="true"] { border-color: #76c7b7; background: #184940; }
    button:disabled { cursor: not-allowed; opacity: .45; }
    header { position: sticky; top: 0; z-index: 5; display: grid; gap: .65rem; grid-template-columns: minmax(14rem,1fr) auto auto; align-items: end; padding: .8rem 1rem; border-bottom: 1px solid #343a40; background: #101315f2; backdrop-filter: blur(8px); }
    header label { display: grid; gap: .25rem; font-size: .82rem; color: #b8c0c4; }
    main { display: grid; grid-template-columns: minmax(0, 1.35fr) minmax(19rem, .65fr); min-height: calc(100vh - 5rem); }
    .evidence, .questions { padding: 1rem; min-width: 0; }
    .questions { border-left: 1px solid #343a40; }
    video { display: block; width: 100%; max-height: 58vh; background: #000; border-radius: .6rem; }
    h1 { margin: 0; font-size: 1.05rem; }
    h2 { font-size: 1rem; margin: 1.2rem 0 .55rem; }
    .muted { color: #aeb7bc; }
    .controls, .navigation, .timestamp { display: flex; flex-wrap: wrap; gap: .55rem; align-items: center; }
    .options { display: grid; gap: .45rem; }
    .option { display: flex; justify-content: space-between; gap: 1rem; }
    kbd { color: #9de3d5; font: .8rem ui-monospace, monospace; }
    .frames { display: grid; grid-template-columns: repeat(auto-fill,minmax(9rem,1fr)); gap: .55rem; }
    .frame { padding: .3rem; }
    .frame img { display: block; width: 100%; aspect-ratio: 16/9; object-fit: contain; background: #000; border-radius: .25rem; }
    .frame span { display: block; padding: .3rem .15rem 0; color: #bdc5c9; font-size: .78rem; }
    .ocr { font-size: .75rem; color: #e4d997; }
    .transcript { display: grid; gap: .35rem; max-height: 15rem; overflow: auto; }
    .transcript button { display: grid; grid-template-columns: 6rem 1fr; gap: .5rem; }
    .status { padding: .65rem; border-radius: .45rem; background: #202529; }
    .complete { color: #9de3d5; }
    .warning { color: #f1c97b; }
    #download { width: 100%; justify-content: center; text-align: center; margin-top: 1rem; }
    @media (max-width: 860px) { header { grid-template-columns: 1fr 1fr; } header h1 { grid-column: 1/-1; } main { grid-template-columns: 1fr; } .questions { border-left: 0; border-top: 1px solid #343a40; } }
  </style>
</head>
<body>
  <header>
    <h1>Identity-blind temporal review</h1>
    <label>Reviewer identity<input id="reviewer" autocomplete="off" spellcheck="false" placeholder="reviewer id"></label>
    <div class="controls">
      <label>Speed <select id="speed"><option>0.5</option><option>0.75</option><option selected>1</option><option>1.25</option><option>1.5</option><option>2</option></select></label>
      <label><input id="autoplay" type="checkbox" checked> Autoplay</label>
    </div>
  </header>
  <main>
    <section class="evidence" aria-label="Evidence">
      <video id="video" controls playsinline preload="metadata"></video>
      <h2>Ordered frames</h2>
      <div id="frames" class="frames"></div>
      <h2>Transcript</h2>
      <div id="transcript" class="transcript"></div>
    </section>
    <section class="questions" aria-label="Review questions">
      <div id="progress" class="status" aria-live="polite"></div>
      <h2>1. Unit structure</h2>
      <div id="units" class="options"></div>
      <div id="role-section" hidden>
        <h2>2. Standalone filler role</h2>
        <div id="roles" class="options"></div>
      </div>
      <h2>Decisive timestamp</h2>
      <p class="muted">Seek to the moment that most strongly supports the answer, then record it.</p>
      <div class="timestamp">
        <input id="timestamp" type="number" min="0" step="1" aria-label="Decisive timestamp in milliseconds">
        <button id="use-time" type="button">Use current video time <kbd>T</kbd></button>
      </div>
      <h2>Navigation</h2>
      <div class="navigation">
        <button id="previous" type="button">Previous <kbd>[</kbd></button>
        <button id="next-incomplete" type="button">Next incomplete <kbd>]</kbd></button>
      </div>
      <p id="case-status" class="muted" aria-live="polite"></p>
      <button id="download" type="button" disabled>Export completed submission</button>
    </section>
  </main>
  <script>
    "use strict";
    const reviewPackage = __LOOMARR_TEMPORAL_REVIEW_PACKAGE_JSON__;
    const packageSHA256 = "__LOOMARR_TEMPORAL_REVIEW_PACKAGE_SHA256__";
    const storageKey = "loomarr-temporal-review:" + packageSHA256;
    const units = [
      ["standalone", "Standalone", "S"], ["compilation", "Compilation", "C"],
      ["programme_excerpt", "Programme excerpt", "P"], ["unusable", "Unusable", "U"], ["unclear", "Unclear", "X"],
    ];
    const roles = [
      ["commercial", "Commercial", "1"], ["promo", "Promo", "2"], ["bumper", "Bumper", "3"], ["psa", "PSA", "4"],
      ["station_id", "Station ID", "5"], ["trailer", "Trailer", "6"], ["interstitial", "Interstitial", "7"], ["unclear", "Unclear", "8"],
    ];
    const byId = (id) => document.getElementById(id);
    const video = byId("video");
    const reviewer = byId("reviewer");
    const speed = byId("speed");
    const autoplay = byId("autoplay");
    let state = { current: 0, reviewerId: "", speed: 1, autoplay: true, answers: {} };

    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) || "null");
      if (saved && typeof saved === "object") state = { ...state, ...saved, answers: saved.answers || {} };
    } catch (_) { /* corrupted browser state is ignored; it is never submission truth */ }
    if (!Number.isInteger(state.current) || state.current < 0 || state.current >= reviewPackage.cases.length) state.current = 0;
    reviewer.value = typeof state.reviewerId === "string" ? state.reviewerId : "";
    speed.value = String(state.speed || 1);
    autoplay.checked = state.autoplay !== false;

    const persist = () => {
      state.reviewerId = reviewer.value.trim();
      state.speed = Number(speed.value);
      state.autoplay = autoplay.checked;
      try { localStorage.setItem(storageKey, JSON.stringify(state)); } catch (_) { /* export remains available without resume */ }
    };
    const answerFor = (item) => state.answers[item.alias] || (state.answers[item.alias] = {});
    const isComplete = (item) => {
      const answer = answerFor(item);
      return !!answer.unit && (answer.unit !== "standalone" || !!answer.role) && Number.isInteger(answer.decisiveAtMs) && answer.decisiveAtMs >= 0 && answer.decisiveAtMs < item.durationMs;
    };
    const timestamp = (ms) => Math.floor(ms / 60000) + ":" + String(Math.floor((ms % 60000) / 1000)).padStart(2, "0") + "." + String(ms % 1000).padStart(3, "0");
    const optionButton = ([value, label, key], kind) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "option";
      button.dataset.value = value;
      button.innerHTML = "<span>" + label + "</span><kbd>" + key + "</kbd>";
      button.addEventListener("click", () => choose(kind, value));
      return button;
    };
    units.forEach((option) => byId("units").append(optionButton(option, "unit")));
    roles.forEach((option) => byId("roles").append(optionButton(option, "role")));

    function choose(kind, value) {
      const item = reviewPackage.cases[state.current];
      const answer = answerFor(item);
      answer[kind] = value;
      if (kind === "unit" && value !== "standalone") delete answer.role;
      persist();
      renderQuestions();
    }

    function renderEvidence(item) {
      video.src = item.video.path;
      video.playbackRate = Number(speed.value);
      if (autoplay.checked) video.play().catch(() => {});
      const frames = byId("frames");
      frames.replaceChildren();
      item.frames.forEach((frame) => {
        const button = document.createElement("button");
        button.type = "button";
        button.className = "frame";
        button.innerHTML = "<img alt=\"Frame at " + timestamp(frame.atMs) + "\" src=\"" + frame.path + "\"><span>" + timestamp(frame.atMs) + "</span>";
        if (frame.ocr && frame.ocr.length) {
          const ocr = document.createElement("span");
          ocr.className = "ocr";
          ocr.textContent = frame.ocr.map((item) => item.text).join(" · ");
          button.append(ocr);
        }
        button.addEventListener("click", () => { video.currentTime = frame.atMs / 1000; });
        frames.append(button);
      });
      const transcript = byId("transcript");
      transcript.replaceChildren();
      if (!item.transcriptSegments || !item.transcriptSegments.length) {
        transcript.textContent = "No transcript is available for this case.";
      } else item.transcriptSegments.forEach((segment) => {
        const button = document.createElement("button");
        button.type = "button";
        button.innerHTML = "<span>" + timestamp(segment.startMs) + "</span><span></span>";
        button.lastElementChild.textContent = segment.text;
        button.addEventListener("click", () => { video.currentTime = segment.startMs / 1000; });
        transcript.append(button);
      });
    }

    function renderQuestions() {
      const item = reviewPackage.cases[state.current];
      const answer = answerFor(item);
      byId("units").querySelectorAll("button").forEach((button) => button.setAttribute("aria-pressed", String(button.dataset.value === answer.unit)));
      byId("role-section").hidden = answer.unit !== "standalone";
      byId("roles").querySelectorAll("button").forEach((button) => button.setAttribute("aria-pressed", String(button.dataset.value === answer.role)));
      byId("timestamp").value = Number.isInteger(answer.decisiveAtMs) ? String(answer.decisiveAtMs) : "";
      const complete = reviewPackage.cases.filter(isComplete).length;
      byId("progress").innerHTML = "<strong>Case " + (state.current + 1) + " of " + reviewPackage.cases.length + "</strong><br><span class=\"" + (complete === reviewPackage.cases.length ? "complete" : "muted") + "\">" + complete + " complete · " + (reviewPackage.cases.length - complete) + " remaining</span>";
      byId("case-status").textContent = isComplete(item) ? "This case is complete." : "This case still needs a valid unit, conditional role, and timestamp.";
      byId("previous").disabled = state.current === 0;
      byId("download").disabled = complete !== reviewPackage.cases.length || !reviewer.value.trim();
    }

    function show(index) {
      state.current = Math.max(0, Math.min(reviewPackage.cases.length - 1, index));
      persist();
      renderEvidence(reviewPackage.cases[state.current]);
      renderQuestions();
    }
    function nextIncomplete() {
      for (let offset = 1; offset <= reviewPackage.cases.length; offset++) {
        const index = (state.current + offset) % reviewPackage.cases.length;
        if (!isComplete(reviewPackage.cases[index])) return show(index);
      }
      show((state.current + 1) % reviewPackage.cases.length);
    }
    function recordTime() {
      const item = reviewPackage.cases[state.current];
      const value = Math.min(item.durationMs - 1, Math.max(0, Math.round(video.currentTime * 1000)));
      answerFor(item).decisiveAtMs = value;
      persist();
      renderQuestions();
    }

    reviewer.addEventListener("input", () => { persist(); renderQuestions(); });
    speed.addEventListener("change", () => { video.playbackRate = Number(speed.value); persist(); });
    autoplay.addEventListener("change", persist);
    byId("timestamp").addEventListener("change", (event) => {
      const value = Number(event.target.value);
      const item = reviewPackage.cases[state.current];
      if (Number.isInteger(value) && value >= 0 && value < item.durationMs) answerFor(item).decisiveAtMs = value;
      else delete answerFor(item).decisiveAtMs;
      persist(); renderQuestions();
    });
    byId("use-time").addEventListener("click", recordTime);
    byId("previous").addEventListener("click", () => show(state.current - 1));
    byId("next-incomplete").addEventListener("click", nextIncomplete);
    byId("download").addEventListener("click", () => {
      persist();
      const reviewerId = reviewer.value.trim();
      const completedAt = new Date().toISOString();
      const submission = {
        schemaVersion: 1, contractVersion: reviewPackage.contractVersion, batchId: reviewPackage.batchId,
        packageSha256: packageSHA256, reviewerId, preparedAt: reviewPackage.preparedAt, completedAt,
        answers: reviewPackage.cases.map((item) => ({ alias: item.alias, reviewerId, unit: answerFor(item).unit,
          ...(answerFor(item).unit === "standalone" ? { role: answerFor(item).role } : {}), decisiveAtMs: answerFor(item).decisiveAtMs })),
      };
      const blob = new Blob([JSON.stringify(submission, null, 2) + "\n"], { type: "application/json" });
      const link = document.createElement("a");
      link.href = URL.createObjectURL(blob);
      link.download = "temporal-review-" + reviewPackage.batchId + ".json";
      link.click();
      setTimeout(() => URL.revokeObjectURL(link.href), 0);
    });
    document.addEventListener("keydown", (event) => {
      if (event.target instanceof HTMLInputElement || event.target instanceof HTMLSelectElement || event.target instanceof HTMLTextAreaElement || event.metaKey || event.ctrlKey || event.altKey) return;
      const key = event.key.toLowerCase();
      const unit = units.find((item) => item[2].toLowerCase() === key);
      const role = roles.find((item) => item[2] === event.key);
      if (unit) choose("unit", unit[0]);
      else if (role && answerFor(reviewPackage.cases[state.current]).unit === "standalone") choose("role", role[0]);
      else if (key === "t") recordTime();
      else if (event.key === "[") show(state.current - 1);
      else if (event.key === "]") nextIncomplete();
      else return;
      event.preventDefault();
    });
    show(state.current);
  </script>
</body>
</html>
`
