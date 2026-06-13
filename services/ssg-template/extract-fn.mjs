// Shared test helper (FE-002 / FE-010): pull a `function name(...) { ... }`
// definition out of an HTML/source string by brace-matching, so the SSG inline
// scripts can be unit-tested in isolation. Not a *.test.mjs, so `node --test`
// imports it but doesn't treat it as a test file.
export function extractFunctionSource(src, name) {
  const start = src.indexOf(`function ${name}(`);
  if (start === -1) {
    throw new Error(`function ${name}() not found`);
  }
  const open = src.indexOf('{', start);
  let depth = 0;
  for (let i = open; i < src.length; i++) {
    if (src[i] === '{') depth++;
    else if (src[i] === '}') {
      depth--;
      if (depth === 0) return src.slice(start, i + 1);
    }
  }
  throw new Error(`unbalanced braces for ${name}`);
}
