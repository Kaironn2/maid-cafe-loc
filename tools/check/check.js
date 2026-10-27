// Valida um arquivo de tradução {chave: {PT, EN}}: UTF-8 sem BOM, sem mojibake,
// marcação ({n}, <tags>, \n) igual ao EN. Uso: node tools/check/check.js <arquivo> [...]
const fs = require('fs');
let bad = 0;
for (const file of process.argv.slice(2)) {
  const buf = fs.readFileSync(file);
  const err = (m) => { bad++; console.log(`ERRO ${file}: ${m}`); };
  if (buf[0] === 0xef && buf[1] === 0xbb && buf[2] === 0xbf) err('tem BOM (salve como UTF-8 sem BOM)');
  const text = buf.toString('utf8');
  if (!Buffer.from(text, 'utf8').equals(buf)) err('não é UTF-8 válido');
  let data;
  try { data = JSON.parse(text.replace(/^﻿/, '')); } catch (e) { err('JSON inválido: ' + e.message); continue; }
  let empty = 0, done = 0;
  for (const [k, v] of Object.entries(data)) {
    if (typeof v !== 'object' || typeof v.EN !== 'string' || typeof v.PT !== 'string') { err(`[${k}] precisa ter "PT" e "EN" como texto`); continue; }
    if (!v.PT.trim()) { empty++; continue; }
    done++;
    const pt = v.PT, en = v.EN;
    if (/Ã[\u0080-¿]|Â[\u0080-¿ ]|â€|�/.test(pt)) err(`[${k}] mojibake/encoding quebrado: ${JSON.stringify(pt)}`);
    if (/\\u[0-9a-f]{4}/i.test(pt)) err(`[${k}] escape \\uXXXX literal no texto: ${JSON.stringify(pt)}`);
    const same = (re) => { const a = (en.match(re) || []).sort().join(' '), b = (pt.match(re) || []).sort().join(' '); return a === b ? null : `EN[${a}] PT[${b}]`; };
    let d;
    if ((d = same(/\{\d+\}/g))) err(`[${k}] marcadores {n} diferentes: ${d}`);
    if ((d = same(/<\/?[a-zA-Z][^<>]*>/g))) err(`[${k}] tags <...> diferentes: ${d}`);
    const nl = (s) => (s.match(/\n/g) || []).length;
    if (Math.abs(nl(en) - nl(pt)) > 1) err(`[${k}] quebras de linha muito diferentes: EN ${nl(en)} / PT ${nl(pt)}`);
  }
  console.log(`${file}: ${done} traduzidos, ${empty} vazios`);
}
console.log(bad ? `\n${bad} problema(s).` : '\nOK: nenhum problema.');
process.exit(bad ? 1 : 0);
