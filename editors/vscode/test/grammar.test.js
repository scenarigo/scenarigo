// Tokenizes template expressions with the injection grammar, using the
// TextMate engine and the Oniguruma regular expressions VS Code uses, so that
// a pattern that does not compile or that misses its scope fails here.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const oniguruma = require('vscode-oniguruma');
const textmate = require('vscode-textmate');

const wasm = fs.readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm')).buffer;
const grammarPath = path.join(__dirname, '..', 'syntaxes', 'scenarigo-template.injection.json');

async function main() {
  await oniguruma.loadWASM(wasm);
  const registry = new textmate.Registry({
    onigLib: Promise.resolve({
      createOnigScanner: (sources) => new oniguruma.OnigScanner(sources),
      createOnigString: (s) => new oniguruma.OnigString(s),
    }),
    // Load the injection grammar as the root grammar of the test lines.
    loadGrammar: async () => textmate.parseRawGrammar(fs.readFileSync(grammarPath, 'utf8'), grammarPath),
  });
  const grammar = await registry.loadGrammar('scenarigo.template.injection');

  const scopesOf = (line) => {
    const { tokens } = grammar.tokenizeLine(line, textmate.INITIAL);
    return tokens.map((t) => [line.slice(t.startIndex, t.endIndex), t.scopes[t.scopes.length - 1]]);
  };
  const scopeOf = (line, text) => {
    const found = scopesOf(line).find(([t]) => t === text);
    assert.ok(found, `no token ${JSON.stringify(text)} in ${JSON.stringify(scopesOf(line))}`);
    return found[1];
  };

  const line = 'url: http://{{vars.host}}/{{size(steps.login.result) ?? 0}} "{{ "a}}b" }}"';
  assert.equal(scopeOf(line, '{{'), 'punctuation.section.embedded.begin.scenarigo');
  assert.equal(scopeOf(line, 'vars'), 'variable.language.scenarigo');
  assert.equal(scopeOf(line, 'host'), 'variable.other.property.scenarigo');
  assert.equal(scopeOf(line, 'size'), 'support.function.builtin.scenarigo');
  assert.equal(scopeOf(line, '??'), 'keyword.operator.scenarigo');
  assert.equal(scopeOf(line, '0'), 'constant.numeric.scenarigo');
  assert.equal(scopeOf(line, 'a}}b'), 'string.quoted.double.scenarigo');
  assert.equal(scopeOf(line, 'url: http://'), 'scenarigo.template.injection');

  const unicode = '{{vars.名前 && assert.contains <-}}';
  assert.equal(scopeOf(unicode, '名前'), 'variable.other.property.scenarigo');
  assert.equal(scopeOf(unicode, '<-'), 'keyword.operator.scenarigo');
  // A name that only ends like a root is not a root.
  assert.ok(
    !scopesOf('{{myvars.x}}').some(([, scope]) => scope === 'variable.language.scenarigo'),
    JSON.stringify(scopesOf('{{myvars.x}}')),
  );

  console.log('grammar ok');
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
