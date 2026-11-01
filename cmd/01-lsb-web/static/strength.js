// DCT 강도별 비교. 같은 메시지를 T만 바꿔 /api/attack으로 측정한다.
const STRENGTHS = [4, 12, 24, 36, 60, 96];
const LEVELS = [
  { name: "약한 훼손", color: "var(--q1)", shape: "circle" },
  { name: "중간 훼손", color: "var(--q2)", shape: "square", dash: "5 3" },
  { name: "강한 훼손", color: "var(--q3)", shape: "diamond", dash: "2 3" },
];
const PSNR_REF = 40; // 눈으로 차이를 느끼기 어려운 기준
const ZOOM = { fx: 0.3, fy: 0.293, scale: 4, aspect: 4 / 3 }; // 확대 영역의 왼쪽 위(이미지 대비 비율)와 배율

const $ = (id) => document.getElementById(id);
const dB = (v) => (v == null ? "∞" : v.toFixed(1));
let results = [], message = "", token = 0;

function svgEl(tag, attrs, parent) {
  const e = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const k in attrs) e.setAttribute(k, attrs[k]);
  parent?.appendChild(e);
  return e;
}

function marker(svg, cx, cy, s) {
  const ring = { fill: s.color, stroke: "var(--surface)", "stroke-width": 2 };
  if (s.shape === "square") svgEl("rect", { x: cx - 5, y: cy - 5, width: 10, height: 10, rx: 2, ...ring }, svg);
  else if (s.shape === "diamond") svgEl("rect", { x: cx - 4.5, y: cy - 4.5, width: 9, height: 9, transform: `rotate(45 ${cx} ${cy})`, ...ring }, svg);
  else svgEl("circle", { cx, cy, r: 5, ...ring }, svg);
}

// T를 가로축으로 쓰는 선 차트 한 칸. wrap은 툴팁의 기준이 되는 .charts 요소다.
function lineChart(wrap, tip, { title, series, yMin, yMax, ticks, ref, fmt }) {
  const W = 280, H = 190, L = 34, R = 14, T = 24, B = 26;
  const span = STRENGTHS.at(-1) - STRENGTHS[0];
  const x = (t) => L + 6 + ((t - STRENGTHS[0]) / span) * (W - L - R - 12);
  const y = (v) => T + (1 - (Math.min(v, yMax) - yMin) / (yMax - yMin)) * (H - T - B);
  const svg = svgEl("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `T별 ${title}` }, wrap);
  svgEl("text", { x: L, y: 12, class: "ttl" }, svg).textContent = title;

  for (const v of ticks) {
    const isRef = v === ref;
    svgEl("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), stroke: isRef ? "var(--muted)" : "var(--line)", "stroke-dasharray": isRef ? "4 3" : "" }, svg);
    svgEl("text", { x: L - 6, y: y(v) + 4, "text-anchor": "end" }, svg).textContent = String(v).replace(/^0\./, ".");
  }
  for (const s of series) {
    const pts = STRENGTHS.map((t, i) => `${x(t)},${y(s.values[i])}`).join(" ");
    svgEl("polyline", { points: pts, fill: "none", stroke: s.color, "stroke-width": 2, "stroke-linejoin": "round", "stroke-dasharray": s.dash || "" }, svg);
  }

  STRENGTHS.forEach((t, i) => {
    svgEl("text", { x: x(t), y: H - 8, "text-anchor": "middle" }, svg).textContent = t;
    const col = svgEl("g", { class: "col" }, svg);
    svgEl("line", { class: "hair", x1: x(t), x2: x(t), y1: T, y2: H - B }, col);
    for (const s of series) marker(col, x(t), y(s.values[i]), s);
    // 칸 경계는 이웃 점과의 중간으로 잡아 간격이 달라도 빈틈이 없게 한다.
    const left = i ? (x(STRENGTHS[i - 1]) + x(t)) / 2 : x(t) - 12;
    const right = i < STRENGTHS.length - 1 ? (x(t) + x(STRENGTHS[i + 1])) / 2 : x(t) + 12;
    const hit = svgEl("rect", {
      class: "hit", x: left, y: 0, width: right - left, height: H, tabindex: 0,
      "aria-label": `T ${t}: ` + series.map((s) => `${s.name} ${fmt(s.values[i])}`).join(", "),
    }, col);
    const show = () => {
      col.classList.add("on");
      tip.textContent = "";
      const head = document.createElement("div");
      head.className = "h";
      head.textContent = `${title} · T=${t}`;
      tip.appendChild(head);
      for (const s of series) {
        const row = document.createElement("div");
        const key = document.createElement("i");
        key.style.background = s.color;
        const val = document.createElement("b");
        val.textContent = fmt(s.values[i]);
        row.append(key, val, s.name);
        tip.appendChild(row);
      }
      tip.hidden = false;
      const box = hit.getBoundingClientRect(), host = wrap.getBoundingClientRect();
      const pos = box.left - host.left + box.width / 2 - tip.offsetWidth / 2;
      tip.style.left = Math.max(0, Math.min(pos, host.width - tip.offsetWidth)) + "px";
      tip.style.top = box.top - host.top + 26 + "px";
    };
    const hide = () => { col.classList.remove("on"); tip.hidden = true; };
    hit.addEventListener("pointerenter", show);
    hit.addEventListener("pointerleave", hide);
    hit.addEventListener("focus", show);
    hit.addEventListener("blur", hide);
  });
}

function chartHost(root) {
  root.textContent = "";
  const wrap = document.createElement("div");
  wrap.className = "charts";
  const tip = document.createElement("div");
  tip.className = "tip";
  tip.hidden = true;
  wrap.appendChild(tip);
  root.appendChild(wrap);
  return { wrap, tip };
}

function renderQuality() {
  const psnrs = results.map((d) => d.wmPsnr).filter((v) => v != null);
  const yMin = Math.floor(Math.min(...psnrs, PSNR_REF) / 10) * 10;
  const yMax = Math.ceil(Math.max(...psnrs, PSNR_REF) / 10) * 10;
  const ticks = [];
  for (let v = yMin; v <= yMax; v += 10) ticks.push(v);
  const { wrap, tip } = chartHost($("psnrChart"));
  lineChart(wrap, tip, {
    title: "PSNR (dB), 점선은 40dB", ref: PSNR_REF, yMin, yMax, ticks, fmt: (v) => `${v.toFixed(1)} dB`,
    series: [{ name: "PSNR", color: "var(--s1)", values: results.map((d) => d.wmPsnr ?? yMax) }],
  });
  $("quality").innerHTML = `<tr><th>T</th><th>PSNR</th><th>SSIM</th></tr>` +
    results.map((d, i) => `<tr><td>${STRENGTHS[i]}</td><td>${dB(d.wmPsnr)} dB</td><td>${num(d.wmSsim)}</td></tr>`).join("");
}

function renderBer() {
  const head = STRENGTHS.map((t) => `<th>T=${t}</th>`).join("");
  $("ber").innerHTML = `<tr><th></th>${head}</tr>` +
    results[0].cases.map((c, idx) => {
      const name = `${KINDS.find((k) => k.kind === c.attack).short} ${c.label.replace("품질 ", "Q").replace("반경 ", "")}`;
      const cells = results.map((d) => `<td><div class="st-cell"><b>${num(d.cases[idx].rawBer, 3)}</b></div></td>`).join("");
      return `<tr><th scope="row">${escHTML(name)}</th>${cells}</tr>`;
    }).join("");

  const root = $("berCharts");
  const { wrap, tip } = chartHost(root);
  const legend = document.createElement("div");
  legend.className = "chart-legend";
  for (const s of LEVELS) {
    const item = document.createElement("span");
    const key = svgEl("svg", { width: 26, height: 12 }, item);
    svgEl("line", { x1: 0, x2: 26, y1: 6, y2: 6, stroke: s.color, "stroke-width": 2, "stroke-dasharray": s.dash || "" }, key);
    marker(key, 13, 6, s);
    item.append(s.name);
    legend.appendChild(item);
  }
  const chance = document.createElement("span");
  chance.textContent = "점선 0.5 = 무작위 추측";
  legend.appendChild(chance);
  root.prepend(legend);

  KINDS.forEach((k, panel) => {
    lineChart(wrap, tip, {
      title: k.name, ref: 0.5, yMin: 0, yMax: 0.6, ticks: [0, 0.25, 0.5], fmt: (v) => num(v, 3),
      series: LEVELS.map((lv, i) => ({
        ...lv,
        name: results[0].cases[panel * 3 + i].label,
        values: results.map((d) => d.cases[panel * 3 + i].rawBer),
      })),
    });
  });
}

function renderViews() {
  const view = $("view").value, usesAttack = view === "attacked" || view === "errors";
  $("attack").disabled = !usesAttack;
  const c = results[0].cases[+$("attack").value];
  const gain = results[0].embedGain;
  $("viewDesc").textContent = {
    embed: `원본과 삽입 결과의 픽셀 차이를 ${gain}배 증폭했습니다. 모든 T에 같은 배율을 씁니다.`,
    attacked: "선택한 조건으로 훼손한 이미지입니다.",
    errors: "밝은 칸은 비트 유지, 빨간 칸은 비트 파손, 어두운 곳은 삽입 안 함입니다. 칸 하나가 8×8 블록입니다.",
  }[view];

  $("views").innerHTML = results.map((d, i) => {
    const src = "/img/attacked.png?" + new URLSearchParams({
      method: "dct", message, strength: STRENGTHS[i], attack: usesAttack ? c.attack : "none",
      level: usesAttack ? c.level : 0, view,
    });
    const cap = usesAttack ? `T=${STRENGTHS[i]} · BER ${num(d.cases[+$("attack").value].rawBer, 3)}` : `T=${STRENGTHS[i]} · PSNR ${dB(d.wmPsnr)} dB`;
    return `<figure><div class="frame dark"><img src="${src}" alt="${escHTML(cap)}" loading="lazy"></div><figcaption>${cap}</figcaption></figure>`;
  }).join("");
}

function renderActual() {
  const items = [
    { src: "/img/original.png", cap: "원본" },
    ...results.map((d, i) => ({
      src: "/img/attacked.png?" + new URLSearchParams({ method: "dct", message, strength: STRENGTHS[i], attack: "none", level: 0, view: "raw" }),
      cap: `T=${STRENGTHS[i]} · PSNR ${dB(d.wmPsnr)} dB`,
    })),
  ];
  $("actual").innerHTML = items.map((it) =>
    `<figure><div class="frame"><img src="${it.src}" alt="${escHTML(it.cap)}" loading="lazy"></div><figcaption>${escHTML(it.cap)}</figcaption></figure>`).join("");

  const { fx, fy, scale, aspect } = ZOOM, d = results[0];
  const pos = `width:${scale * 100}%;left:${-scale * fx * 100}%;top:${-scale * fy * (d.height / d.width) * aspect * 100}%`;
  $("zoomDesc").textContent = `같은 영역을 ${scale}배 확대했습니다. 픽셀을 보간하지 않습니다.`;
  $("actualZoom").innerHTML = items.map((it) =>
    `<figure><div class="zoom" style="aspect-ratio:${aspect}"><img src="${it.src}" alt="${escHTML(it.cap)} 확대" style="${pos}"></div><figcaption>${escHTML(it.cap)}</figcaption></figure>`).join("");
}

async function run() {
  const mine = ++token, msg = $("msg").value;
  $("summary").innerHTML = `<span class="loading">측정 중…</span>`;
  try {
    const data = await Promise.all(STRENGTHS.map((t) => postAttack("dct", msg, t)));
    if (mine !== token) return; // 더 최신 요청이 있으면 버린다
    results = data;
    message = msg;
    const d = data[0];
    $("summary").textContent = `${d.slots.toLocaleString()}개 블록에 1비트씩 넣습니다.`;
    const picked = +$("attack").value;
    $("attack").innerHTML = d.cases.map((c, i) =>
      `<option value="${i}"${i === picked ? " selected" : ""}>${escHTML(KINDS.find((k) => k.kind === c.attack).short)} ${escHTML(c.label)}</option>`).join("");
    renderQuality();
    renderBer();
    renderViews();
    renderActual();
    $("out").hidden = false;
  } catch (e) {
    if (mine === token) $("summary").innerHTML = `<span class="err">${escHTML(e.message)}</span>`;
  }
}

$("form").addEventListener("submit", (e) => { e.preventDefault(); run(); });
$("view").addEventListener("change", renderViews);
$("attack").addEventListener("change", renderViews);
run();
