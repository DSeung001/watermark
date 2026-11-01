// 복구 방식 비교. T별로 /api/recovery를 한 번씩 측정해 두고 선택만 바꿔 보여준다.
const STRENGTHS = [4, 12, 24, 36, 60, 96];
const DEFAULT_STRENGTH = 12;

const $ = (id) => document.getElementById(id);
const count = (v) => v.toLocaleString();
const dB = (v) => (v == null ? "∞" : `${v.toFixed(1)} dB`);
let results = new Map(), token = 0;

async function postRecovery(message, strength) {
  const res = await fetch("/api/recovery", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ message, strength }),
  });
  const d = await res.json();
  if (!res.ok) throw new Error(d.error);
  return d;
}

const caseName = (c) => `${KINDS.find((k) => k.kind === c.attack).short} ${c.label}`;

function encodeCell(m, d) {
  if (m.key === "vote") return `<b>${count(m.codedBits)}비트</b><small>부호화 없음</small>`;
  if (m.key === "hamming") return `<b>${count(m.codedBits)}비트</b><small>4비트마다 패리티 3비트 추가</small>`;
  return `<b>${count(m.codedBits)}비트</b><small>패리티 ${m.parity}바이트, 최대 ${m.correctable}바이트 정정</small>`;
}

function voteCell(m, c) {
  if (m.repeat < 2) return `<b>${count(c.votedErrors)}비트 틀림</b><small>반복이 없어 투표 없음</small>`;
  return `<b>${count(c.votedErrors)}비트 틀림</b><small>전체 ${count(m.codedBits)}비트 중</small>`;
}

function fixCell(m, c) {
  if (m.key === "vote") return `<b>없음</b><small>정정 부호 없음</small>`;
  if (m.key === "hamming") return `<b>${count(c.fixed)}곳 수정</b><small>코드워드당 1비트까지 정정</small>`;
  if (c.failed) return `<b>정정 실패</b><small>한도 ${m.correctable}바이트 초과</small>`;
  return `<b>${count(c.fixed)}바이트 수정</b><small>한도 ${m.correctable}바이트</small>`;
}

function renderFlow(d, idx) {
  const first = d.methods[0].cases[idx];
  $("flowDesc").textContent = `T=${d.strength}, ${caseName(first)} 조건입니다. 메시지 ${d.bits}비트를 세 방식 모두 ${count(d.blocks)}개 블록에 넣습니다.`;
  $("flow").innerHTML = `<tr><th>방식</th><th>부호화</th><th>DCT 삽입</th><th>훼손 후 읽기</th><th>다수결</th><th>정정</th><th>복구 결과</th></tr>` +
    d.methods.map((m) => {
      const c = m.cases[idx], mark = verdict(c);
      return `<tr><td>${escHTML(m.name)}</td><td>${encodeCell(m, d)}</td>` +
        `<td><b>${m.repeat < 2 ? "반복 없음" : `약 ${m.repeat.toFixed(1)}회 반복`}</b><small>PSNR ${dB(m.psnr)}</small></td>` +
        `<td><b>BER ${num(c.codedBer, 3)}</b><small>${count(c.codedErrors)} / ${count(c.codedTotal)}자리 틀림</small></td>` +
        `<td>${voteCell(m, c)}</td><td>${fixCell(m, c)}</td>` +
        `<td><b class="st ${mark.cls}">${escHTML(mark.text)}</b><small>메시지 ${c.errors} / ${c.bits}비트 틀림</small><span class="msg">${escHTML(showText(c.extracted))}</span></td></tr>`;
    }).join("");
  const rs = d.methods.find((m) => m.key === "rs");
  $("flowNote").textContent = `${rs.name}은 RS(255,223)을 블록 수에 맞춰 줄인 코드입니다. 코드워드 ${rs.codedBits / 8}바이트에 메시지 ${d.bits / 8}바이트와 패리티 ${rs.parity}바이트가 들어갑니다. 원래 메시지는 정답 비교에만 쓰고, 복호에는 삽입 위치와 메시지 길이를 안다고 가정합니다.`;
}

function renderBer(d) {
  $("berDesc").textContent = `T=${d.strength}에서 복구까지 마친 메시지의 BER입니다. 작은 값은 복구 전 복사본 하나하나의 오류율입니다.`;
  const head = d.methods.map((m) => `<th>${escHTML(m.name)}</th>`).join("");
  $("ber").innerHTML = `<tr><th></th>${head}</tr>` +
    d.methods[0].cases.map((c0, idx) => {
      const cells = d.methods.map((m) => {
        const c = m.cases[idx], mark = verdict(c);
        return `<td><div class="st-cell"><b>${num(c.ber, 3)}<span class="st ${mark.cls}" title="${escHTML(mark.text)}">${mark.text.slice(0, 1)}</span></b>` +
          `<small>복구 전 ${num(c.codedBer, 3)}${c.failed ? ", 정정 실패" : ""}</small></div></td>`;
      }).join("");
      return `<tr><th scope="row">${escHTML(caseName(c0))}</th>${cells}</tr>`;
    }).join("");
}

function renderByT() {
  const all = STRENGTHS.map((t) => results.get(t));
  const row = (name, cell) => `<tr><td>${escHTML(name)}</td>${all.map(cell).join("")}</tr>`;
  $("byT").innerHTML = `<tr><th>방식</th>${STRENGTHS.map((t) => `<th>T=${t}</th>`).join("")}</tr>` +
    all[0].methods.map((m, i) => row(m.name, (d) =>
      `<td>${d.methods[i].cases.filter((c) => c.match).length} / ${d.methods[i].cases.length}</td>`)).join("") +
    row("PSNR", (d) => `<td>${dB(d.methods[0].psnr)}</td>`);
}

function render() {
  const d = results.get(+$("strength").value), idx = +$("attack").value;
  renderFlow(d, idx);
  renderBer(d);
  renderByT();
  $("out").hidden = false;
}

async function run() {
  const mine = ++token, msg = $("msg").value;
  $("summary").innerHTML = `<span class="loading">측정 중…</span>`;
  try {
    const data = await Promise.all(STRENGTHS.map((t) => postRecovery(msg, t)));
    if (mine !== token) return; // 더 최신 요청이 있으면 버린다
    results = new Map(STRENGTHS.map((t, i) => [t, data[i]]));
    if (!$("attack").options.length) {
      $("attack").innerHTML = data[0].methods[0].cases.map((c, i) => `<option value="${i}">${escHTML(caseName(c))}</option>`).join("");
    }
    $("summary").textContent = "";
    render();
  } catch (e) {
    if (mine === token) $("summary").innerHTML = `<span class="err">${escHTML(e.message)}</span>`;
  }
}

$("strength").innerHTML = STRENGTHS.map((t) => `<option value="${t}"${t === DEFAULT_STRENGTH ? " selected" : ""}>${t}</option>`).join("");
$("form").addEventListener("submit", (e) => { e.preventDefault(); run(); });
$("strength").addEventListener("change", render);
$("attack").addEventListener("change", render);
run();
