const $ = id => document.getElementById(id);
const levels = {
  jpeg: [[90, "품질 90"], [70, "품질 70"], [50, "품질 50"]],
  blur: [[1, "반경 1px"], [2, "반경 2px"], [3, "반경 3px"]],
  resize: [[75, "75%"], [50, "50%"], [25, "25%"]],
};
let current = null;
let generation = 0;
function invalidate() {
  generation++;
  current = null;
  $("damaged").hidden = true;
  $("recovered").hidden = true;
  $("error").hidden = true;
  $("status").textContent = "조건이 바뀌었습니다. 삽입 후 훼손하기를 눌러 다시 실험하세요."; // replaced below
  $("run").disabled = false;
}
function updateLevels() {
  $("level").replaceChildren(...levels[$("attack").value].map(([value, label]) => new Option(label, value)));
}
updateLevels();
$("attack").addEventListener("change", updateLevels);
$("experiment").addEventListener("input", () => {
  $("strengthValue").value = $("strength").value;
  $("strength").disabled = $("method").value !== "dct";
  invalidate();
});
$("experiment").addEventListener("submit", async event => {
  event.preventDefault();
  invalidate();
  const version = generation;
  const config = {method: $("method").value, message: $("message").value, strength: Number($("strength").value), attack: $("attack").value, level: Number($("level").value)};
  $("run").disabled = true;
  $("status").textContent = "워터마크를 삽입하고 훼손 결과를 계산하고 있습니다…"; // replaced below
  try {
    const data = await postAttack(config.method, config.message, config.strength);
    if (version !== generation) return;
    const result = data.cases.find(c => c.attack === config.attack && c.level === config.level);
    if (!result) throw new Error("훼손 결과를 찾을 수 없습니다.");
    current = {data, result};
    const params = new URLSearchParams({...config, view: "attacked"});
    $("attacked").src = `/img/attacked.png?${params}`;
    params.set("attack", "none");
    $("watermarked").src = `/img/attacked.png?${params}`;
    $("condition").textContent = `${config.method.toUpperCase()} · ${$("attack").selectedOptions[0].textContent} · ${result.label} · 메시지 전체 ${data.repeat}회 반복 + 나머지 자리`;
    $("raw").textContent = `훼손 후 개별 복사본의 오류율: ${(result.rawBer * 100).toFixed(2)}% (${result.rawErrors.toLocaleString()} / ${result.rawTotal.toLocaleString()}비트). 아래 버튼으로 반복 비트를 모아 복구합니다.`;
    $("damaged").hidden = false;
    $("recover").disabled = false;
    $("status").textContent = "훼손 완료. 이미지를 확인한 뒤 메시지를 복구하세요.";
  } catch (error) {
    if (version !== generation) return;
    $("error").textContent = error.message;
    $("error").hidden = false;
    $("status").textContent = "실험을 완료하지 못했습니다. 조건을 확인하고 다시 시도하세요.";
  } finally {
    if (version === generation) $("run").disabled = false;
  }
});
for (const id of ["watermarked", "attacked"]) {
  $(id).addEventListener("error", () => {
    if (!current) return;
    $("error").textContent = "이미지를 불러오지 못했습니다. 실험을 다시 실행해 주세요.";
    $("error").hidden = false;
  });
}
$("recover").addEventListener("click", () => {
  if (!current) return;
  const {data, result: c} = current;
  $("expected").textContent = data.message;
  $("actual").textContent = showText(c.extracted);
  $("verdict").textContent = c.match ? "완전 복구 성공 · 원래 메시지와 일치합니다." : `복구 후에도 ${c.errors}비트 오류가 남았습니다.`;
  $("verdict").style.color = c.match ? "var(--ok)" : "var(--mark)";
  $("comparison").textContent = `복사본 오류율 ${(c.rawBer * 100).toFixed(2)}% → 다수결 후 BER ${(c.ber * 100).toFixed(2)}% (${c.errors} / ${c.bits}비트)`;
  $("limit").textContent = `전체 ${c.bits}비트 중 앞 ${c.gotBits.length}비트 표시. 동률은 0으로 결정하며, 다수의 복사본이 함께 훼손되면 복구가 실패할 수 있습니다.`;
  $("votes").replaceChildren(...Array.from(c.gotBits, (bit, i) => {
    const row = document.createElement("tr");
    for (const value of [i + 1, c.copies[i] - c.ones[i], c.ones[i], bit, c.wantBits[i], bit === c.wantBits[i] ? "일치" : "오류"]) {
      const cell = document.createElement("td");
      cell.textContent = value;
      row.appendChild(cell);
    }
    if (bit !== c.wantBits[i]) row.className = "mark-bad";
    return row;
  }));
  $("recover").disabled = true;
  $("recovered").hidden = false;
  $("status").textContent = "복구 완료. 아래에서 메시지와 투표 과정을 확인하세요.";
  $("recovered").scrollIntoView({behavior: "smooth", block: "start"});
});
