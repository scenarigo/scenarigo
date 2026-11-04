// Tokenizes YAML documents with a YAML grammar and the injection grammar,
// registered from package.json as VS Code registers them, using the TextMate
// engine and the Oniguruma regular expressions VS Code uses. The injection
// selector, injectTo and the patterns are all exercised.
//
// The YAML grammar is the TextMate one from the tm-grammars package. VS Code
// ships another one (extensions/yaml/syntaxes in the VS Code repository); set
// YAML_GRAMMAR_DIR to a directory holding its files to run the test with it.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const oniguruma = require('vscode-oniguruma');
const textmate = require('vscode-textmate');

const root = path.join(__dirname, '..');
const manifest = JSON.parse(fs.readFileSync(path.join(root, 'package.json'), 'utf8'));
const TEMPLATE = 'meta.embedded.template.scenarigo';

// grammarFiles maps scope names to grammar files: the YAML grammars and the
// grammars of the extension.
function grammarFiles() {
  const files = {};
  const dir = process.env.YAML_GRAMMAR_DIR;
  if (dir) {
    for (const name of fs.readdirSync(dir).filter((n) => n.endsWith('.json'))) {
      const file = path.join(dir, name);
      const { scopeName } = JSON.parse(fs.readFileSync(file, 'utf8'));
      if (scopeName) {
        files[scopeName] = file;
      }
    }
  } else {
    files['source.yaml'] = require.resolve('tm-grammars/grammars/yaml.json');
  }
  for (const g of manifest.contributes.grammars) {
    files[g.scopeName] = path.join(root, g.path);
  }
  return files;
}

async function main() {
  const wasm = fs.readFileSync(require.resolve('vscode-oniguruma/release/onig.wasm')).buffer;
  await oniguruma.loadWASM(wasm);
  const files = grammarFiles();
  const registry = new textmate.Registry({
    onigLib: Promise.resolve({
      createOnigScanner: (sources) => new oniguruma.OnigScanner(sources),
      createOnigString: (s) => new oniguruma.OnigString(s),
    }),
    loadGrammar: async (scopeName) =>
      files[scopeName] ? textmate.parseRawGrammar(fs.readFileSync(files[scopeName], 'utf8'), files[scopeName]) : null,
    getInjections: (scopeName) =>
      manifest.contributes.grammars.filter((g) => (g.injectTo || []).includes(scopeName)).map((g) => g.scopeName),
  });
  const yaml = await registry.loadGrammar('source.yaml');
  assert.ok(yaml, 'no YAML grammar');

  // tokens returns the tokens of a YAML document as [line, text, scopes].
  const tokens = (doc) => {
    const out = [];
    let state = textmate.INITIAL;
    doc.split('\n').forEach((line, i) => {
      const r = yaml.tokenizeLine(line, state);
      for (const t of r.tokens) {
        out.push([i, line.slice(t.startIndex, t.endIndex), t.scopes]);
      }
      state = r.ruleStack;
    });
    return out;
  };
  // scopes returns the scopes of the n-th token of a document whose text is
  // text, on the given line when there is one.
  const scopes = (doc, text, { line, n = 0 } = {}) => {
    const found = tokens(doc).filter(([l, t]) => t === text && (line === undefined || l === line));
    assert.ok(found.length > n, `no token ${JSON.stringify(text)} in ${JSON.stringify(tokens(doc).map(([l, t, s]) => [l, t, s.at(-1)]))}`);
    return found[n][2];
  };
  const innermost = (doc, text, opts) => scopes(doc, text, opts).at(-1);
  const inTemplate = (doc, text, opts) => scopes(doc, text, opts).includes(TEMPLATE);

  // Templates in plain, quoted and block scalars.
  const doc = [
    'url: http://{{vars.host}}/{{size(steps.login.result) ?? 0}}',
    'q: \'{{ "a}}b" }}\'',
    'u: "{{vars.名前 && assert.contains <-}}"',
    'block: |',
    '  {{myvars.x}}',
  ].join('\n');
  assert.equal(innermost(doc, '{{'), 'punctuation.section.embedded.begin.scenarigo');
  assert.equal(innermost(doc, 'vars'), 'variable.language.scenarigo');
  assert.equal(innermost(doc, 'host'), 'variable.other.property.scenarigo');
  assert.equal(innermost(doc, 'size'), 'support.function.builtin.scenarigo');
  assert.equal(innermost(doc, '??'), 'keyword.operator.scenarigo');
  assert.equal(innermost(doc, '0'), 'constant.numeric.scenarigo');
  assert.equal(innermost(doc, 'a}}b'), 'string.quoted.double.scenarigo');
  assert.equal(innermost(doc, '名前'), 'variable.other.property.scenarigo');
  assert.equal(innermost(doc, '<-'), 'keyword.operator.scenarigo');
  assert.ok(inTemplate(doc, '{{', { line: 4 }), 'no template in a block scalar');
  // A name that only ends like a root is not a root.
  assert.ok(
    !tokens(doc).some(([l, , s]) => l === 4 && s.at(-1) === 'variable.language.scenarigo'),
    'myvars is not a root name',
  );

  // Template strings have no escapes: "C:\" ends at the second quote.
  const escapes = ["p: '{{ \"C:\\\" + vars.dir }}'", 'block: |', '  {{ "C:\\" + vars.dir }} more', '  next line', 'after: x'].join('\n');
  assert.equal(innermost(escapes, 'dir', { line: 0 }), 'variable.other.property.scenarigo');
  assert.ok(!inTemplate(escapes, "'", { line: 0, n: 1 }), 'the closing quote of the YAML string is in the template');
  assert.equal(innermost(escapes, 'dir', { line: 2 }), 'variable.other.property.scenarigo');
  assert.ok(!tokens(escapes).some(([l, , s]) => l >= 3 && s.includes(TEMPLATE)), 'the template goes on after its line');

  // In a double-quoted YAML string, a template string is written \"x\".
  const doubleQuoted = 'd: "{{ \\"x\\" }}" # comment';
  const comment = (d) => tokens(d).filter(([, t]) => t.includes('comment'));
  assert.ok(comment(doubleQuoted).length > 0 && !comment(doubleQuoted).some(([, , s]) => s.includes(TEMPLATE)), JSON.stringify(tokens(doubleQuoted)));

  // A template string holding {{ does not open a template in the template.
  const nested = "c: '{{ \"{{\" }}' # comment";
  assert.ok(comment(nested).length > 0 && !comment(nested).some(([, , s]) => s.includes(TEMPLATE)), JSON.stringify(tokens(nested)));
  assert.equal(scopes(nested, '{{').filter((s) => s === TEMPLATE).length, 1);

  // Comments are not templates.
  assert.ok(!tokens('# {{vars.x}}').some(([, , s]) => s.includes(TEMPLATE)));

  console.log(`grammar ok (${process.env.YAML_GRAMMAR_DIR ? 'YAML grammar of ' + process.env.YAML_GRAMMAR_DIR : 'tm-grammars YAML grammar'})`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
