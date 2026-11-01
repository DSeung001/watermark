// LSB, DCT × RS만, 다수결만, 다수결 + RS 비교와 RS 복호 단계별 표시. T는 서버에서 36으로 고정한다.
const $ = (id) => document.getElementById(id);
const count = (v) => v.toLocaleString();
const dB = (v) => (v == null ? "∞" : `${v.toFixed(2)} dB`);
const hex = (b) => b.toString(16).padStart(2, "0");
const decode = (s) => (s ? Uint8Array.from(atob(s), (c) => c.charCodeAt(0)) : new Uint8Array());
const caseName = (c) => `${KINDS.find((k) => k.kind === c.attack).short} ${c.label}`;
const LABELS = { rsonly: "RS만", vote: "다수결만", rs: "다수결 + RS" };
const method = (sc, key) => sc.methods.find((m) => m.key === key);
let data = null, token = 0;

// RS 결과는 복호기의 판단과 실제 일치 여부를 나눠 표시한다.
function rsVerdict(c) {
  if (c.match) return { cls: "ok", text: "✓ 완전 복원" };
  if (c.failed) return { cls: "bad", text: "✗ 정정 실패" };
  if (c.fixed === 0) return { cls: "bad", text: "✗ 오류 미검출" };
  return { cls: "bad", text: "✗ 오정정" };
}

const copies = (v) => (Number.isInteger(v) ? count(v) : `약 ${v < 100 ? v.toFixed(2) : count(Math.round(v))}`);
const mark = (m, c) => (m.key === "vote" ? verdict(c) : rsVerdict(c));

function renderSettings(d) {
  const s = d.settings, [lsb, dct] = d.schemes;
  const rows = [
    ["공통", "숨긴 메시지", escHTML(d.message)],
    ["", "메시지 길이", `${d.bits / 8}바이트 (${d.bits}비트)`],
    ["", "원본 이미지", `${s.width} × ${s.height}`],
    ["", "훼손 조건", "JPEG 품질 90 / 70 / 50, Gaussian Blur 반경 1 / 2 / 3px, 축소 후 복원 75 / 50 / 25%"],
    ["", "복호 시 아는 값", "삽입 위치, 메시지 길이"],
    ["LSB", "삽입 자리", `불투명 픽셀(alpha=255)의 R, G, B 최하위 비트, ${count(lsb.slots)}자리`],
    ["", "프레임", `길이 헤더 ${s.headerBytes}바이트 + 데이터, 바이트마다 MSB부터`],
    ["DCT", "강도 T", `${s.strength} (비트 1: c₁−c₂ ≥ T, 비트 0: c₁−c₂ ≤ −T)`],
    ["", "블록과 계수쌍", `${s.blockSize}×${s.blockSize} 휘도 블록, (${s.pair.U1},${s.pair.V1}) / (${s.pair.U2},${s.pair.V2}), 블록당 1비트`],
    ["", "사용 블록", `전 픽셀 불투명, 평균 휘도 ${s.minLuma}~${s.maxLuma}, ${count(dct.slots)}블록`],
    ["다수결", "판정", "비트마다 복사본의 다수값을 채택, 동률이면 0"],
    ["RS", "부호", `RS(${s.n},${s.k}), RS(${s.baseN},${s.baseK})을 줄인 체계적 부호`],
    ["", "길이", `메시지 ${s.k}바이트 + 패리티 ${s.parity}바이트 = ${s.n}바이트 (${s.n * 8}비트)`],
    ["", "정정 한도", `t = ${s.parity} / 2 = ${s.correctable}바이트`],
    ["", "심볼과 체", "심볼 8비트, GF(2⁸)"],
    ["", "원시 다항식", `x⁸ + x⁴ + x³ + x² + 1 (0x${s.primitive.toString(16)})`],
    ["", "생성원", "α = 2"],
    ["", "생성 다항식의 근", `α⁰ … α^${s.parity - 1} (첫 근의 지수 0)`],
  ];
  $("setDesc").textContent = `코드와 같은 값을 서버에서 받아 표시합니다. 세 방식 모두 같은 자리 전체에 데이터를 반복 삽입합니다. RS만은 첫 복사본 ${s.n * 8}비트만 읽어 바로 복호하고, 다수결 + RS는 복사본을 투표로 합친 ${s.n}바이트를 복호합니다.`;
  $("settings").innerHTML = `<tr><th>구분</th><th>항목</th><th>값</th></tr>` +
    rows.map(([g, k, v]) => `<tr><td>${g}</td><td>${k}</td><td>${v}</td></tr>`).join("");
  const baseBits = s.baseN * 8, rsDct = method(dct, "rs");
  $("layout").innerHTML = `<div class="m" style="flex:${s.k}">메시지 ${s.k}바이트</div><div class="h" style="flex:${s.parity}">패리티 ${s.parity}바이트</div>`;
  const fit = baseBits > dct.slots ? `${count(baseBits)}비트라 DCT ${count(dct.slots)}블록에 들어가지 않습니다` : `${count(baseBits)}비트입니다`;
  $("setNote").textContent = `범용 값 RS(${s.baseN},${s.baseK})의 패리티 ${s.parity}바이트를 그대로 쓰고 코드워드만 줄였습니다. 메시지 앞에 0을 ${s.baseK - s.k}바이트 채워 부호화한 뒤 그 0을 보내지 않는 것과 같습니다. RS(${s.baseN},${s.baseK}) 코드워드는 ${fit}. ` +
    `RS(${s.n},${s.k})는 DCT에서 약 ${rsDct.repeat.toFixed(2)}회 반복되어 비트마다 복사본이 ${Math.floor(rsDct.repeat)}~${Math.ceil(rsDct.repeat)}개입니다.`;
}

function renderSummary(d) {
  const n = d.schemes[0].methods[0].cases.length;
  const rows = d.schemes.flatMap((sc) => sc.methods.map((m) => {
    const avg = m.cases.reduce((a, c) => a + c.ber, 0) / n;
    const label = LABELS[m.key];
    return `<tr><td>${sc.name} ${escHTML(label)}</td><td>${count(m.codedBits)}비트</td><td>${copies(m.repeat)}</td>` +
      `<td>${dB(m.psnr)}</td><td>${num(m.ssim, 4)}</td><td>${m.cases.filter((c) => c.match).length} / ${n}</td><td>${num(avg, 3)}</td></tr>`;
  }));
  $("sumTable").innerHTML = `<tr><th>방식</th><th>복사본 길이</th><th>읽는 복사본 수</th><th>PSNR</th><th>SSIM</th><th>완전 복원</th><th>평균 복구 후 BER</th></tr>` + rows.join("");
  $("sumNote").textContent = "복사본 길이는 부호화한 데이터 길이이고, LSB는 여기에 길이 헤더가 더 붙습니다. 자리 수가 같으므로 코드워드가 길수록 복사본 수는 줄어듭니다. RS만과 다수결 + RS는 같은 이미지를 쓰고 읽는 방법만 다릅니다.";
}

const BER_MAX = 0.6; // 막대 눈금의 끝. 0.5(무작위 추측)가 83.333%에 온다

// 복구 전(빈 원)에서 복구 후(채운 원)로 이어지는 한 줄
function berRow(name, c, v) {
  const before = Math.min(c.codedBer / BER_MAX, 1) * 100, after = Math.min(c.ber / BER_MAX, 1) * 100;
  return `<div class="rs-db" title="${escHTML(name)}: 복구 전 ${num(c.codedBer, 3)}, 복구 후 ${num(c.ber, 3)}, ${escHTML(v.text)}">
    <span class="rs-db-name">${escHTML(name)}</span>
    <span class="rs-db-track"><i class="vs-chance"></i>
      <i class="rs-db-line" style="left:${Math.min(before, after)}%;width:${Math.abs(before - after)}%"></i>
      <i class="rs-db-dot before" style="left:${before}%"></i><i class="rs-db-dot after" style="left:${after}%"></i></span>
    <span class="rs-db-val">${num(c.codedBer, 3)} → <b>${num(c.ber, 3)}</b> <span class="st ${v.cls}">${v.text.slice(0, 1)}</span></span>
  </div>`;
}

function berChart(sc) {
  const rows = sc.methods[0].cases.map((c0, idx) =>
    `<div class="rs-dbt-r"><span class="rs-dbt-c">${escHTML(caseName(c0))}</span>` +
    sc.methods.map((m) => berRow(LABELS[m.key], m.cases[idx], mark(m, m.cases[idx]))).join("") + `</div>`).join("");
  return `<h3>복구 전후 BER</h3>
    <p class="muted">복구 전은 자리 하나하나를 따로 읽은 오류율이고, 복구 후는 최종 메시지의 오류율입니다. 왼쪽 끝이 0이며 복구 후 값이 0에 가까울수록 잘 복구한 것입니다.</p>
    <div class="legend rs-db-legend">
      <span><i class="rs-db-dot before"></i>복구 전</span>
      <span><i class="rs-db-dot after"></i>복구 후</span>
      <span><i class="rs-db-chance"></i>0.5 무작위 추측</span>
      <span><b class="st ok">✓</b> 완전 복원</span>
      <span><b class="st part">△</b> 일부 오류</span>
      <span><b class="st bad">✗</b> 복원 실패</span>
    </div>
    <div class="rs-dbt">
      <div class="rs-dbt-r rs-dbt-h"><span></span>${sc.methods.map((m) => `<span>${LABELS[m.key]}</span>`).join("")}</div>
      ${rows}
    </div>`;
}

function detailRow(m, c, d, head) {
  const v = mark(m, c), voted = m.key !== "rsonly", coded = m.key !== "vote";
  const over = coded && c.byteErrors > d.settings.correctable ? " class=\"chg\"" : "";
  return `<tr>${head}<td>${LABELS[m.key]}</td><td>${num(c.codedBer, 3)}</td>` +
    `<td>${voted ? `${c.votedErrors} / ${m.codedBits}` : "투표 없음"}</td>` +
    `<td${over}>${coded ? `${c.byteErrors} / ${d.settings.n}` : "-"}</td>` +
    `<td>${!coded ? "-" : c.failed ? "실패" : `${c.fixed}바이트`}</td>` +
    `<td>${c.errors} / ${c.bits}</td><td>${num(c.ber, 3)}</td><td><b class="st ${v.cls}">${escHTML(v.text)}</b></td></tr>`;
}

function renderScheme(sc, i, d) {
  const first = sc.methods[0], vote = method(sc, "vote"), rs = method(sc, "rs");
  const rows = first.cases.map((c0, idx) => sc.methods.map((m, n) => detailRow(m, m.cases[idx], d,
    n === 0 ? `<th scope="row" rowspan="${sc.methods.length}">${escHTML(caseName(c0))}</th>` : "")).join("")).join("");
  const strs = first.cases.map((c0, idx) =>
    `<tr><th scope="row">${escHTML(caseName(c0))}</th>${sc.methods.map((m) => `<td>${escHTML(showText(m.cases[idx].extracted))}</td>`).join("")}</tr>`).join("");
  return `<section>
    <h2><span class="num">${i + 3}</span>${sc.name} 훼손 조건별 결과</h2>
    <p class="muted">${sc.name}에 메시지는 약 ${count(+vote.repeat.toFixed(2))}회, ${escHTML(rs.name)} 코드워드는 약 ${count(+rs.repeat.toFixed(2))}회 반복됩니다. RS만은 첫 복사본 하나만 읽습니다. RS 정정 한도는 ${d.settings.correctable}바이트입니다.</p>
    ${berChart(sc)}
    <h3>상세 수치</h3>
    <div class="tbl-wrap"><table class="rs-cmp">
      <tr><th>훼손 조건</th><th>방식</th><th>복구 전 BER</th><th>투표 후 오류 비트</th><th>RS 입력 틀린 바이트</th><th>RS 정정</th><th>복구 후 오류 비트</th><th>복구 후 BER</th><th>결과</th></tr>
      ${rows}
    </table></div>
    <h3>추출한 문자열</h3>
    <div class="tbl-wrap"><table class="rs-strs"><tr><th>훼손 조건</th>${sc.methods.map((m) => `<th>${LABELS[m.key]}</th>`).join("")}</tr>${strs}</table></div>
    <div class="legend">
      <span><b class="st bad">정정 실패</b> 복호기가 고칠 수 없다고 판단</span>
      <span><b class="st bad">오정정</b> 다른 유효 코드워드로 잘못 고침</span>
      <span><b class="st bad">오류 미검출</b> 신드롬이 0이라 오류를 알아채지 못함</span>
    </div>
  </section>`;
}

// 바이트 격자. cls(i)가 칸의 추가 클래스를 정한다.
function grid(bytes, k, cls = () => "") {
  return `<div class="rs-grid">` + Array.from(bytes, (b, i) =>
    `<span class="hx ${i < k ? "m" : "p"} ${cls(i)}"><small>${i}</small>${hex(b)}</span>`).join("") + `</div>`;
}

// 번호 없는 값 목록. S₀… 같은 머리글을 붙인다.
function vals(bytes, name) {
  return `<div class="rs-grid vals">` + Array.from(bytes, (b, i) =>
    `<span class="hx ${b ? "nz" : "zero"}"><small>${name}${i}</small>${hex(b)}</span>`).join("") + `</div>`;
}

const pre = (code) => `<pre class="rs-code"><code>${escHTML(code)}</code></pre>`;
const skipped = (why) => `<p class="rs-skip">${why}</p>`;
const stop = (why) => `<p class="rs-stop">${why}</p>`;

function trimZeros(b) {
  let n = b.length;
  while (n > 0 && b[n - 1] === 0) n--;
  return b.slice(0, n);
}

const FAIL = {
  degree: (s, deg) => `복호 중단: Λ의 차수가 ${deg}로 정정 한도 ${s.correctable}를 넘었습니다.`,
  chien: (s, deg, found) => `복호 중단: Λ의 차수는 ${deg}인데 코드워드 ${s.n}바이트 안에서 찾은 근은 ${found}개입니다. 차수와 근의 수가 다르면 오류 위치를 믿을 수 없습니다.`,
  forney: () => "복호 중단: Forney 식의 분모 Λ'(X⁻¹)이 0입니다.",
  verify: () => "복호 중단: 정정한 뒤 다시 계산한 신드롬이 0이 아닙니다.",
};

function stageData(d, c) {
  const s = d.settings, tr = c.trace, sent = decode(d.sent);
  const rec = decode(tr.received), syn = decode(tr.syndromes), loc = decode(tr.locator);
  const omega = trimZeros(decode(tr.omega)), mags = decode(tr.magnitudes);
  const corrected = decode(tr.corrected), recheck = decode(tr.recheck), msg = decode(tr.message);
  const pos = tr.positions || [], deg = loc.length - 1;
  const diff = new Set(Array.from(rec.keys()).filter((i) => rec[i] !== sent[i]));
  const found = new Set(pos), k = s.k, at = tr.failedAt;
  const sameSet = diff.size === found.size && [...diff].every((i) => found.has(i));
  const allZero = rec.every((b) => b === 0);
  const nonzero = syn.filter((b) => b).length;
  const failed = !!at;

  const st1 = `<p>메시지 구역 ${[...diff].filter((i) => i < k).length}바이트, 패리티 구역 ${[...diff].filter((i) => i >= k).length}바이트가 송신값과 다릅니다. 빨간 칸은 정답과 비교해 표시한 것으로, 복호기는 이 정보를 모릅니다.</p>` +
    grid(rec, k, (i) => (diff.has(i) ? "err" : ""));

  let st2 = `<p>32개 중 ${nonzero}개가 0이 아닙니다.</p>` + vals(syn, "S");
  if (tr.clean) {
    st2 += diff.size === 0
      ? `<p class="rs-ok">모든 신드롬이 0입니다. 코드워드에 오류가 없습니다.</p>`
      : stop(`모든 신드롬이 0이라 복호기는 오류가 없다고 판단합니다. 하지만 실제로는 ${diff.size}바이트가 틀렸습니다. 수신값이 다른 유효 코드워드이면 신드롬으로 구분할 수 없습니다.${allZero ? " 수신값이 전부 0이고, 전부 0인 값은 RS의 유효 코드워드입니다." : ""}`);
  }

  let st3, st4, st5;
  if (tr.clean) {
    st3 = st4 = st5 = skipped("건너뜀: 신드롬이 모두 0입니다.");
  } else {
    const poly = Array.from(loc, (b, i) => (i === 0 ? hex(b) : `${hex(b)}·x${i === 1 ? "" : `^${i}`}`)).join(" + ");
    st3 = `<p>Berlekamp-Massey가 구한 Λ의 차수는 ${deg}입니다.</p><div class="rule">Λ(x) = ${poly}</div>` +
      `<p>Chien Search로 찾은 위치 ${pos.length}곳: <span class="mono">${pos.join(", ") || "없음"}</span></p>` +
      grid(rec, k, (i) => (found.has(i) ? "err" : ""));
    if (at === "degree" || at === "chien") {
      st3 += stop(FAIL[at](s, deg, pos.length));
      st4 = st5 = skipped("실행되지 않음");
    } else {
      st3 += sameSet ? `<p class="rs-ok">찾은 위치가 실제 틀린 바이트와 일치합니다.</p>`
        : stop(`찾은 위치가 실제 틀린 ${diff.size}바이트와 다릅니다. 오류가 정정 한도를 넘어 다른 코드워드 쪽으로 풀렸습니다.`);
      const rows = pos.map((i, n) => {
        const p = s.n - 1 - i, e = mags[n] ?? 0;
        return `<tr><td>${i}</td><td>${i < k ? "메시지" : "패리티"}</td><td>α^${p}</td><td>${hex(rec[i])}</td><td class="chg">${hex(e)}</td><td>${hex(rec[i] ^ e)}</td><td>${hex(sent[i])}</td></tr>`;
      }).join("");
      st4 = (omega.length ? `<p>Ω 계수는 다음과 같습니다.</p>` + vals(omega, "Ω") : "") +
        `<div class="tbl-wrap"><table class="rs-mag"><tr><th>인덱스 i</th><th>구역</th><th>X</th><th>수신값 r</th><th>오류 크기 e</th><th>r ⊕ e</th><th>송신값</th></tr>${rows}</table></div>`;
      if (at === "forney") {
        st4 += stop(FAIL.forney());
        st5 = skipped("실행되지 않음");
      } else {
        const fixed = corrected.length ? corrected : rec;
        st5 = at === "verify"
          ? stop(FAIL.verify()) + vals(recheck, "S")
          : grid(fixed, k, (i) => (found.has(i) ? "fix" : "")) +
            `<p class="rs-ok">다시 계산한 신드롬 ${recheck.length}개가 모두 0입니다.</p>`;
      }
    }
  }

  const text = escHTML(showText(c.extracted));
  let st6 = grid(msg, k, () => "") + `<dl class="strs"><dt>문자열</dt><dd>${text}</dd><dt>원문</dt><dd>${escHTML(d.message)}</dd></dl>`;
  if (failed) st6 = `<p>정정하지 못했으므로 수신값 앞 ${k}바이트를 그대로 씁니다.</p>` + st6 + stop(`메시지 ${c.errors} / ${c.bits}비트가 틀렸습니다.`);
  else if (c.match) st6 += `<p class="rs-ok">원문과 정확히 일치합니다.</p>`;
  else st6 += stop(`복호기는 성공으로 판단했지만 메시지 ${c.errors} / ${c.bits}비트가 틀렸습니다.`);
  return [st1, st2, st3, st4, st5, st6];
}

// 수식과 코드는 internal/ecc/rs.go의 구현과 같은 표기를 쓴다.
function stageDocs(s, key) {
  const source = key === "rsonly" ? `첫 복사본에서 읽은 ${s.n}바이트` : `다수결로 만든 ${s.n}바이트`;
  const last = s.parity - 1;
  return [
    {
      title: "수신한 코드워드",
      body: `<div class="rule">r(x) = c(x) + e(x)<br>c(x) = m(x)·x<sup>${s.parity}</sup> + (m(x)·x<sup>${s.parity}</sup> mod g(x))<br>g(x) = (x − α<sup>0</sup>)(x − α<sup>1</sup>) … (x − α<sup>${last}</sup>)</div>
        <p>${source}가 수신어 r입니다. c는 보낸 코드워드, e는 오류입니다. GF(2⁸)에서 덧셈은 XOR입니다.</p>`,
    },
    {
      title: "신드롬 계산",
      body: `<div class="rule">S<sub>j</sub> = r(α<sup>j</sup>) = c(α<sup>j</sup>) + e(α<sup>j</sup>) = e(α<sup>j</sup>)  (j = 0 … ${last})</div>
        <p>α<sup>j</sup>는 g(x)의 근이라 c(α<sup>j</sup>) = 0입니다. 그래서 신드롬에는 오류 성분만 남습니다.</p>` +
        pre(`for j := range nsym {
	var y byte
	for _, c := range code {
		y = gfMul(y, gfExp[j]) ^ c // Horner
	}
	syn[j] = y
}`),
    },
    {
      title: "오류 탐색",
      body: `<div class="rule">Λ(x) = (1 − X<sub>1</sub>x)(1 − X<sub>2</sub>x) … (1 − X<sub>ν</sub>x)<br>Δ<sub>n</sub> = S<sub>n</sub> + Λ<sub>1</sub>S<sub>n−1</sub> + … + Λ<sub>L</sub>S<sub>n−L</sub><br>Λ(X<sub>k</sub><sup>−1</sup>) = 0,  X<sub>k</sub> = α<sup>p</sup>,  i = n − 1 − p</div>
        <p>Berlekamp-Massey는 신드롬을 만족하는 가장 짧은 Λ를 구합니다. Chien Search는 모든 위치를 대입해 근을 찾습니다.</p>` +
        pre(`// Berlekamp-Massey: 어긋남 delta를 이전 다항식으로 지운다
delta := syn[n]
for i := 1; i <= errs && i < len(loc); i++ {
	delta ^= gfMul(loc[i], syn[n-i])
}
scale := gfDiv(delta, prevDelta)
for i, c := range prev {
	next[i+shift] ^= gfMul(scale, c)
}

// Chien Search: 인덱스 i의 차수는 p = n-1-i
for i := range n {
	if polyEval(loc, gfExp[(255-(n-1-i))%255]) == 0 {
		positions = append(positions, i)
	}
}`),
    },
    {
      title: "오류 크기 계산",
      body: `<div class="rule">Ω(x) = S(x)·Λ(x) mod x<sup>${s.parity}</sup><br>e<sub>k</sub> = X<sub>k</sub> · Ω(X<sub>k</sub><sup>−1</sup>) / Λ′(X<sub>k</sub><sup>−1</sup>)</div>
        <p>Forney 식입니다. 위치를 알면 각 위치의 오류 크기가 바로 나옵니다. Λ′은 형식 미분이고, 표수 2에서는 홀수 차수 항만 남습니다.</p>` +
        pre(`inv := gfExp[(255-p)%255] // X^-1
d := polyEval(deriv, inv)  // Λ'(X^-1)
e := gfDiv(gfMul(gfExp[p], polyEval(omega, inv)), d)`),
    },
    {
      title: "손실된 심볼 정정",
      body: `<div class="rule">c<sub>i</sub> = r<sub>i</sub> ⊕ e<sub>i</sub></div>` +
        pre(`out[i] ^= e
recheck, ok := rsSyndromes(out, nsym) // 모두 0이어야 성공`),
    },
    {
      title: "원본데이터 추출",
      body: `<div class="rule">m = c[0 : ${s.k}]</div><p>체계적 부호라 코드워드 앞 ${s.k}바이트가 메시지입니다. 패리티 ${s.parity}바이트는 버립니다.</p>`,
    },
  ];
}

function renderTrace() {
  const d = data, sc = d.schemes[+$("traceScheme").value], key = $("traceMethod").value;
  const c = method(sc, key).cases[+$("traceCase").value];
  const v = rsVerdict(c), s = stageData(d, c);
  $("traceSum").innerHTML = `${sc.name}, ${LABELS[key]}, ${escHTML(caseName(c))}: RS 입력 <b>${c.byteErrors}바이트</b> 틀림, 정정 한도 <b>${d.settings.correctable}바이트</b>, 결과 <b class="st ${v.cls}">${escHTML(v.text)}</b>`;
  $("stages").innerHTML = stageDocs(d.settings, key).map((doc, i) =>
    `<li><h3>${doc.title}</h3><div class="rs-stage"><div class="rs-doc">${doc.body}</div><div class="rs-data">${s[i]}</div></div></li>`).join("");
}

function fillCases(keep) {
  const cases = method(data.schemes[+$("traceScheme").value], $("traceMethod").value).cases;
  $("traceCase").innerHTML = cases.map((c, i) => `<option value="${i}">${escHTML(caseName(c))} (${escHTML(rsVerdict(c).text)})</option>`).join("");
  if (keep != null) $("traceCase").value = keep;
}

// 기본 표시는 실제로 정정에 성공한 DCT 조건 중 가장 많이 고친 조건이다.
function defaultCase(d) {
  const i = d.schemes.length - 1, cases = method(d.schemes[i], "rs").cases;
  let best = 0, fixed = -1;
  cases.forEach((c, n) => { if (c.match && c.fixed > fixed) { best = n; fixed = c.fixed; } });
  return [i, best];
}

function render(d) {
  data = d;
  renderSettings(d);
  renderSummary(d);
  $("schemes").innerHTML = d.schemes.map((sc, i) => renderScheme(sc, i, d)).join("");
  $("traceNum").textContent = d.schemes.length + 3;
  $("traceScheme").innerHTML = d.schemes.map((sc, i) => `<option value="${i}">${sc.name}</option>`).join("");
  $("traceMethod").innerHTML = ["rs", "rsonly"].map((k) => `<option value="${k}">${LABELS[k]}</option>`).join("");
  const [si, ci] = defaultCase(d);
  $("traceScheme").value = si;
  fillCases(ci);
  renderTrace();
  $("out").hidden = false;
}

async function run() {
  const mine = ++token;
  $("summary").innerHTML = `<span class="loading">측정 중…</span>`;
  try {
    const res = await fetch("/api/rs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message: $("msg").value }),
    });
    const d = await res.json();
    if (!res.ok) throw new Error(d.error);
    if (mine !== token) return; // 더 최신 요청이 있으면 버린다
    $("summary").textContent = "";
    render(d);
  } catch (e) {
    if (mine === token) $("summary").innerHTML = `<span class="err">${escHTML(e.message)}</span>`;
  }
}

$("form").addEventListener("submit", (e) => { e.preventDefault(); run(); });
$("traceScheme").addEventListener("change", () => { fillCases(); renderTrace(); });
$("traceMethod").addEventListener("change", () => { const keep = $("traceCase").value; fillCases(keep); renderTrace(); });
$("traceCase").addEventListener("change", renderTrace);
run();
