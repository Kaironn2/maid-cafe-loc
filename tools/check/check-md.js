// Confere encoding de um arquivo de texto: UTF-8 válido, sem BOM, sem mojibake.
const fs = require('fs');
const f = process.argv[2], b = fs.readFileSync(f), t = b.toString('utf8');
const errs = [];
if (b[0] === 0xef && b[1] === 0xbb && b[2] === 0xbf) errs.push('tem BOM');
if (!Buffer.from(t, 'utf8').equals(b)) errs.push('não é UTF-8 válido');
const m = t.match(/\u00c3[\u0080-\u00bf]|\u00c2[\u0080-\u00bf]|\u00e2\u20ac|\ufffd/g);
if (m) errs.push('mojibake: ' + [...new Set(m)].join(' '));
console.log(errs.length ? 'ERRO ' + f + ': ' + errs.join('; ') : 'OK: ' + f);
process.exit(errs.length ? 1 : 0);
