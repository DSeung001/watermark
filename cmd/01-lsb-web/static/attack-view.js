// 훼손 실험 결과 렌더링. LSB와 DCT 페이지가 method만 바꿔 함께 쓴다.
const KINDS = [
  { kind: "jpeg", name: "JPEG 압축", short: "JPEG" },
  { kind: "blur", name: "Gaussian Blur", short: "Blur" },
  { kind: "resize", name: "해상도 변경", short: "축소" },
];
const NOISE_BER = 0.35; // 이 이상이면 무작위 추측과 구분하기 어렵다고 표시

const escHTML = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
const showText = (s) => s.replace(/[\u0000-\u001f\u007f]/g, "·");
const num = (v, d = 4) => v.toFixed(d);

function verdict(c) {
  if (c.match) return { cls: "ok", text: "✓ 완전 복원" };
  if (c.ber < NOISE_BER) return { cls: "part", text: "△ 일부 오류" };
  return { cls: "bad", text: "✗ 무작위 수준" };
}

async function postAttack(method, message, strength) {
  const res = await fetch("/api/attack", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message, method, strength }),
  });
  const d = await res.json();
  if (!res.ok) throw new Error(d.error);
  return d;
}

// 공격 강도별 BER 작은 차트 3개. series: [{name, color, dash, square, values(9개)}]
function drawBerCharts(root, series, labels, onPick) {
  const W = 280, H = 190, L = 34, R = 12, T = 24, B = 26, MAX = 0.6;
  const NS = "http://www.w3.org/2000/svg";
  const el = (tag, attrs, parent) => {
    const e = document.createElementNS(NS, tag);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    parent?.appendChild(e);
    return e;
  };
  const y = (v) => T + (1 - Math.min(v, MAX) / MAX) * (H - T - B);

  root.textContent = "";
  const legend = document.createElement("div");
  legend.className = "chart-legend";
  for (const s of series) {
    const item = document.createElement("span");
    const key = el("svg", { width: 26, height: 12 }, item);
    el("line", { x1: 0, x2: 26, y1: 6, y2: 6, stroke: s.color, "stroke-width": 2, "stroke-dasharray": s.dash || "" }, key);
    marker(key, 13, 6, s);
    item.append(s.name);
    legend.appendChild(item);
  }
  const chance = document.createElement("span");
  const chanceKey = el("svg", { width: 26, height: 12 }, chance);
  el("line", { x1: 0, x2: 26, y1: 6, y2: 6, stroke: "var(--muted)", "stroke-dasharray": "4 3" }, chanceKey);
  chance.append("0.5 = 무작위 추측");
  legend.appendChild(chance);
  root.appendChild(legend);

  const wrap = document.createElement("div");
  wrap.className = "charts";
  root.appendChild(wrap);
  const tip = document.createElement("div");
  tip.className = "tip";
  tip.hidden = true;
  wrap.appendChild(tip);

  function marker(svg, cx, cy, s) {
    const ring = { fill: s.color, stroke: "var(--surface)", "stroke-width": 2 };
    if (s.square) el("rect", { x: cx - 5, y: cy - 5, width: 10, height: 10, rx: 2, ...ring }, svg);
    else el("circle", { cx, cy, r: 5, ...ring }, svg);
  }

  KINDS.forEach((k, panel) => {
    const svg = el("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `${k.name} 강도별 BER` }, wrap);
    const x = (i) => L + 22 + i * ((W - L - R - 44) / 2);
    el("text", { x: L, y: 12, class: "ttl" }, svg).textContent = k.name;

    for (const v of [0, 0.25]) {
      el("line", { x1: L, x2: W - R, y1: y(v), y2: y(v), stroke: "var(--line)" }, svg);
      el("text", { x: L - 6, y: y(v) + 4, "text-anchor": "end" }, svg).textContent = v === 0 ? "0" : ".25";
    }
    el("line", { x1: L, x2: W - R, y1: y(0.5), y2: y(0.5), stroke: "var(--muted)", "stroke-dasharray": "4 3" }, svg);
    el("text", { x: L - 6, y: y(0.5) + 4, "text-anchor": "end" }, svg).textContent = ".5";

    for (const s of series) {
      const pts = [0, 1, 2].map((i) => `${x(i)},${y(s.values[panel * 3 + i])}`).join(" ");
      el("polyline", { points: pts, fill: "none", stroke: s.color, "stroke-width": 2, "stroke-linejoin": "round", "stroke-dasharray": s.dash || "" }, svg);
    }
    [0, 1, 2].forEach((i) => {
      const idx = panel * 3 + i;
      el("text", { x: x(i), y: H - 8, "text-anchor": "middle" }, svg).textContent = labels[idx];
      const col = el("g", { class: "col" }, svg);
      el("line", { class: "hair", x1: x(i), x2: x(i), y1: T, y2: H - B }, col);
      for (const s of series) marker(col, x(i), y(s.values[idx]), s);
      const hit = el("rect", {
        class: "hit", x: x(i) - 44, y: 0, width: 88, height: H, tabindex: 0, role: "button",
        "aria-label": `${k.name} ${labels[idx]}: ` + series.map((s) => `${s.name} ${num(s.values[idx])}`).join(", "),
      }, col);
      const show = () => {
        col.classList.add("on");
        tip.textContent = "";
        const head = document.createElement("div");
        head.className = "h";
        head.textContent = `${k.short} ${labels[idx]}`;
        tip.appendChild(head);
        for (const s of series) {
          const row = document.createElement("div");
          const key = document.createElement("i");
          key.style.background = s.color;
          const val = document.createElement("b");
          val.textContent = num(s.values[idx]);
          row.append(key, val, s.name);
          tip.appendChild(row);
        }
        tip.hidden = false;
        const box = hit.getBoundingClientRect(), host = wrap.getBoundingClientRect();
        const left = box.left - host.left + box.width / 2 - tip.offsetWidth / 2;
        tip.style.left = Math.max(0, Math.min(left, host.width - tip.offsetWidth)) + "px";
        tip.style.top = box.top - host.top + 26 + "px";
      };
      const hide = () => { col.classList.remove("on"); tip.hidden = true; };
      hit.addEventListener("pointerenter", show);
      hit.addEventListener("pointerleave", hide);
      hit.addEventListener("focus", show);
      hit.addEventListener("blur", hide);
      if (onPick) {
        hit.addEventListener("click", () => onPick(idx));
        hit.addEventListener("keydown", (e) => {
          if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onPick(idx); }
        });
      }
    });
  });
}

// root 안에 실험 결과 섹션을 만든다. first: 섹션 번호 시작값
function createAttackView(root, method, first = 1) {
  const isLSB = method === "lsb";
  const n = (i) => `<span class="num">${first + i}</span>`;
  root.innerHTML = `
  <section>
    <h2>${n(0)}훼손 후 BER</h2>
    <p class="muted" data-el="intro"></p>
    <div class="ber-grid" data-el="grid"></div>
    <p class="note" style="margin-top:12px">BER은 틀린 비트의 비율입니다. 0.5는 "절반이 살아남았다"가 아니라 동전 던지기와 같다는 뜻이라, BER ${NOISE_BER} 이상은 무작위 수준으로 표시합니다.
      큰 값은 복사본 다수결 BER, 작은 값은 ${isLSB ? "자리" : "블록"} 하나하나를 따로 본 단일 위치 BER입니다. 칸을 누르면 아래 내용이 바뀝니다.</p>
    <div data-el="chart"></div>
  </section>
  <section>
    <h2>${n(1)}값이 어떻게 훼손되나: <span data-el="selName"></span></h2>
    <p class="muted" data-el="selDesc"></p>
    <div class="views" data-el="views"></div>
    <div data-el="zoomWrap"></div>
    <h3>삽입 자리의 값 변화</h3>
    <p class="muted" data-el="sampleDesc"></p>
    <div class="tbl-wrap"><table data-el="samples"></table></div>
  </section>
  <section>
    <h2>${n(2)}워터마크 추출과 BER 측정</h2>
    <p class="calc" data-el="calc"></p>
    <dl class="strs" data-el="strs"></dl>
    <div class="bitgrid" data-el="bits"></div>
    <div class="legend">
      <span><span class="b" style="display:inline-grid;margin-right:4px">0</span>넣은 비트와 같음</span>
      <span><span class="b bad" style="display:inline-grid;margin-right:4px">0</span>틀린 비트 (읽은 값 표시)</span>
    </div>
    <h3>다수결 집계 (앞 8비트)</h3>
    <div class="tbl-wrap"><table data-el="votes"></table></div>
  </section>`;
  const $el = (name) => root.querySelector(`[data-el=${name}]`);

  let data = null, selected = 0, message = "", strength = 0;

  const imgURL = (c, view) => "/img/attacked.png?" + new URLSearchParams({
    method, message, strength: strength || "", attack: c ? c.attack : "none", level: c ? c.level : 0, view,
  });

  function renderGrid() {
    const grid = $el("grid");
    grid.textContent = "";
    KINDS.forEach((k, r) => {
      const h = document.createElement("div");
      h.className = "rowh";
      h.textContent = k.name;
      grid.appendChild(h);
      [0, 1, 2].forEach((i) => {
        const idx = r * 3 + i, c = data.cases[idx], v = verdict(c);
        const btn = document.createElement("button");
        btn.type = "button";
        btn.className = "cell";
        btn.setAttribute("aria-pressed", idx === selected);
        btn.innerHTML = `<span class="lv">${escHTML(c.label)}</span><span class="v">${num(c.ber, 3)}</span>` +
          `<span class="raw">단일 ${num(c.rawBer, 3)}</span><span class="st ${v.cls}">${v.text}</span>`;
        btn.addEventListener("click", () => select(idx));
        grid.appendChild(btn);
      });
    });
  }

  function select(idx) {
    selected = idx;
    renderGrid();
    renderDetail();
  }

  function describe(c) {
    if (c.attack === "jpeg") return `품질 ${c.level}로 압축했다가 다시 풀었습니다. 압축 크기 ${(c.jpegBytes / 1024).toFixed(1)}KB. JPEG는 8×8 블록의 고주파를 버립니다. ` +
      (isLSB ? "픽셀마다 독립적인 ±1 변화는 고주파 성분이라 양자화에서 사라집니다." : "삽입에 쓴 계수쌍은 저주파라 양자화 뒤에도 대소 관계가 남기 쉽습니다.");
    if (c.attack === "blur") return `σ=${c.level}px Gaussian 필터를 적용했습니다. 각 값이 주변 값과 섞입니다. ` +
      (isLSB ? "픽셀 하나에 담긴 비트가 이웃 비트와 뒤섞입니다." : "두 계수가 함께 줄어들어 부호는 유지되지만, 반경이 커지면 차이가 반올림 오차에 묻힙니다.");
    return `${c.smallW}×${c.smallH}로 줄인 뒤(박스 평균) 원본 크기 ${data.width}×${data.height}로 되돌려(바이리니어) 추출했습니다. 복원에는 원본 크기를 안다는 가정이 들어갑니다.`;
  }

  function figure(src, alt, cap, dark) {
    return `<figure><div class="frame${dark ? " dark" : ""}"><img src="${src}" alt="${escHTML(alt)}" loading="lazy"></div><figcaption>${cap}</figcaption></figure>`;
  }

  function renderDetail() {
    const c = data.cases[selected];
    const name = KINDS.find((k) => k.kind === c.attack).name;
    $el("selName").textContent = `${name} ${c.label}`;
    $el("selDesc").textContent = describe(c) + ` 공격 전 대비 PSNR ${c.psnr == null ? "∞" : c.psnr.toFixed(1)} dB, SSIM ${num(c.ssim)}.`;

    const views = [
      figure(imgURL(null, "attacked"), "공격 전 이미지", "공격 전. 공격은 RGB만 다루므로 투명 배경이 검게 보입니다"),
      figure(imgURL(c, "attacked"), "공격 후 이미지", "공격 후"),
      figure(imgURL(c, "diff"), "차이 이미지", `공격 전후 차이 ×${data.diffGain} 증폭`),
      figure(imgURL(c, "errors"), "오류 지도", `오류 지도: 밝은 곳은 비트 유지, 빨간 곳은 비트 파손, 어두운 곳은 삽입 안 함${isLSB ? "" : " (칸 하나가 8×8 블록)"}`, true),
    ];
    if (isLSB) {
      views.push(
        figure(imgURL(null, "plane"), "공격 전 LSB 평면", "공격 전 LSB 평면: 보이는 영역 전체에 메시지 패턴이 깔려 있습니다"),
        figure(imgURL(c, "plane"), "공격 후 LSB 평면", "공격 후 LSB 평면"),
      );
    }
    $el("views").innerHTML = views.join("");

    // LSB 평면 확대: 얼굴처럼 평탄한 영역을 8배로 본다
    if (isLSB) {
      const zoom = (src, alt) => `<div class="zoom"><img src="${src}" alt="${escHTML(alt)}" style="width:800%;left:-260%;top:-300%"></div>`;
      $el("zoomWrap").innerHTML = `<h3>LSB 평면 일부 8배 확대</h3><div class="views">` +
        `<figure>${zoom(imgURL(null, "plane"), "공격 전 확대")}<figcaption>공격 전</figcaption></figure>` +
        `<figure>${zoom(imgURL(c, "plane"), "공격 후 확대")}<figcaption>공격 후</figcaption></figure></div>`;
    }

    renderSamples(c);
    renderExtract(c);
  }

  function renderSamples(c) {
    const fmt = (v) => {
      if (!isLSB) return v.toFixed(1);
      const b = v.toString(2).padStart(8, "0");
      return `${b.slice(0, 7)}<span class="lsb">${b[7]}</span> <span class="muted">(${v})</span>`;
    };
    $el("sampleDesc").textContent = isLSB
      ? `이미지 곳곳에서 고른 ${c.samples.length}자리입니다. 마지막 1비트가 워터마크입니다.`
      : `이미지 곳곳에서 고른 ${c.samples.length}개 블록입니다. 계수 차이의 부호(양수=1, 음수=0)가 워터마크입니다.`;
    $el("samples").innerHTML =
      `<tr><th>위치</th><th>${isLSB ? "공격 전" : "공격 전 c₁−c₂"}</th><th>${isLSB ? "공격 후" : "공격 후 c₁−c₂"}</th><th>넣은 비트</th><th>읽은 비트</th><th>결과</th></tr>` +
      c.samples.map((s) => {
        const ok = s.want === s.got;
        return `<tr><td>${escHTML(s.loc)}</td><td>${fmt(s.before)}</td><td>${fmt(s.after)}</td><td>${s.want}</td><td>${s.got}</td>` +
          `<td>${ok ? `<span class="mark-ok">✓ 유지</span>` : `<span class="mark-bad">✗ 틀림</span>`}</td></tr>`;
      }).join("");
  }

  function renderExtract(c) {
    const raw = `단일 위치 BER = ${c.rawErrors.toLocaleString()} ÷ ${c.rawTotal.toLocaleString()} = ${num(c.rawBer)}`;
    $el("calc").innerHTML = `다수결 BER = ${c.errors} ÷ ${c.bits} = ${num(c.ber)}<br><span class="muted" style="font-weight:400;font-size:14px">${raw}</span>`;

    const strs = $el("strs");
    strs.textContent = "";
    const add = (dt, dd) => {
      const a = document.createElement("dt"), b = document.createElement("dd");
      a.textContent = dt;
      b.textContent = dd;
      strs.append(a, b);
    };
    add("넣은 메시지", data.message);
    add("추출 결과", showText(c.extracted));
    add("판정", verdict(c).text);
    if (c.headerOk != null) {
      add("길이 헤더", c.headerOk
        ? `정상 (${c.header}바이트)`
        : `파손: ${c.header.toLocaleString()}바이트로 읽힘. 메시지 길이를 모르면 추출 자체가 불가능합니다`);
    }

    const bits = $el("bits");
    let html = "";
    for (let i = 0; i < c.gotBits.length; i += 8) {
      html += `<span class="byte">`;
      for (let j = i; j < i + 8; j++) {
        html += `<span class="b${c.gotBits[j] !== c.wantBits[j] ? " bad" : ""}">${c.gotBits[j]}</span>`;
      }
      html += `</span>`;
    }
    bits.innerHTML = html;

    $el("votes").innerHTML =
      `<tr><th>메시지 비트</th><th>넣은 값</th><th>복사본 중 맞은 수</th><th>다수결</th></tr>` +
      Array.from({ length: Math.min(8, c.gotBits.length) }, (_, i) => {
        const want = +c.wantBits[i], copies = c.copies[i];
        const right = want ? c.ones[i] : copies - c.ones[i];
        const ok = c.wantBits[i] === c.gotBits[i];
        return `<tr><td>${i}</td><td>${want}</td><td><span class="meter"><i style="width:${(right / copies) * 100}%"></i></span>${right.toLocaleString()} / ${copies.toLocaleString()}</td>` +
          `<td>${c.gotBits[i]} ${ok ? `<span class="mark-ok">✓</span>` : `<span class="mark-bad">✗</span>`}</td></tr>`;
      }).join("");
  }

  return {
    render(d) {
      data = d;
      message = d.message;
      strength = d.strength;
      $el("intro").textContent =
        `메시지 전체가 ${d.slots.toLocaleString()}${d.unit}에 약 ${d.repeat.toLocaleString()}번 반복해 들어갑니다. ` +
        `삽입 후 원본 대비 PSNR ${d.wmPsnr == null ? "∞" : d.wmPsnr.toFixed(1)} dB입니다.`;
      const labels = d.cases.map((c) => c.label.replace("품질 ", "Q").replace("반경 ", ""));
      drawBerCharts($el("chart"), [
        { name: "다수결 BER", color: "var(--s1)", values: d.cases.map((c) => c.ber) },
        { name: "단일 위치 BER", color: "var(--s2)", dash: "5 3", square: true, values: d.cases.map((c) => c.rawBer) },
      ], labels, select);
      select(Math.min(selected, d.cases.length - 1));
    },
  };
}
