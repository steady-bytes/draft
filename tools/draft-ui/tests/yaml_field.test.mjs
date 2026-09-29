// Tests for the workflow editor's Name / Description fields, which behaviors.js keeps in step with
// the `metadata:` block of the YAML text. Run with `node --test tests/*.test.mjs`.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "..", "js", "behaviors.js"), "utf8");

// behaviors.js needs a page around it only to register listeners; the helpers under test are pure.
const noop = () => {};
const document = { documentElement: { getAttribute: noop, setAttribute: noop }, addEventListener: noop, querySelectorAll: () => [], fonts: null };
const window = { draftPrefs: { get: noop, set: noop }, addEventListener: noop };
new Function("document", "window", "getComputedStyle", src)(document, window, () => ({ getPropertyValue: () => "" }));
const { read, write } = window.draftYaml;

const doc = 'apiVersion: bench/v1\nkind: Workflow\nmetadata:\n  name: crud-e2e\n  description: "Create a Name: via Crud"\ntrigger:\n  webhook:\n    slug: x\nsteps:\n  - name: a\n';

test("reads plain and quoted values", () => {
  assert.equal(read(doc, "name"), "crud-e2e");
  assert.equal(read(doc, "description"), "Create a Name: via Crud");
  assert.equal(read("metadata:\n  name: 'it''s'\n", "name"), "it's");
  assert.equal(read("metadata:\n  name: true\n", "name"), "true");
});

test("does not mistake the step's name for the workflow's", () => {
  assert.equal(read("steps:\n  - name: a\n", "name"), null);
  assert.equal(read("metadata:\n  description: d\nsteps:\n  - name: a\n", "name"), null);
});

test("rewrites only the metadata line", () => {
  const next = write(doc, "name", "new-name").split("\n");
  assert.equal(next[3], "  name: new-name");
  assert.equal(next[9], "  - name: a");
  assert.equal(next.length, doc.split("\n").length);
});

test("quotes what YAML would misread", () => {
  assert.equal(write(doc, "description", "has: colon").split("\n")[4], '  description: "has: colon"');
  assert.equal(write(doc, "description", "").split("\n")[4], '  description: ""');
  assert.equal(write("metadata:\n  name: a\n", "name", "true").split("\n")[1], '  name: "true"');
  assert.equal(write("metadata:\n  name: a\n", "name", "a #b").split("\n")[1], '  name: "a #b"');
});

test("adds a missing key at the end of the metadata block", () => {
  assert.equal(
    write("metadata:\n  name: a\nsteps:\n  - name: s\n", "description", "hi"),
    "metadata:\n  name: a\n  description: hi\nsteps:\n  - name: s\n",
  );
});

test("leaves a document without metadata, and block scalars, alone", () => {
  const noMeta = "steps:\n  - name: s\n";
  assert.equal(write(noMeta, "name", "x"), noMeta);
  const block = "metadata:\n  name: a\n  description: >\n    long\n    text\nsteps: []\n";
  assert.equal(write(block, "description", "x"), block);
  assert.equal(read(block, "description"), null);
});

// --- the editor's colouring -------------------------------------------------------------------------

const hl = (text) => window.draftYaml.highlight(text);

test("colours keys, urls, literals and comments", () => {
  assert.equal(hl("kind: Workflow"), '<span class="tk-key">kind</span>: Workflow');
  assert.equal(hl("  - name: a"), '  - <span class="tk-key">name</span>: a');
  assert.equal(hl("    uses: foundry://http-call@v1"), '    <span class="tk-key">uses</span>: <span class="tk-url">foundry://http-call@v1</span>');
  assert.equal(hl("      status: OK"), '      <span class="tk-key">status</span>: <span class="tk-val">OK</span>');
  assert.equal(hl("      retries: 3"), '      <span class="tk-key">retries</span>: <span class="tk-val">3</span>');
  assert.equal(hl("# a note"), '<span class="tk-com"># a note</span>');
  assert.equal(hl("key: value # trailing"), '<span class="tk-key">key</span>: value <span class="tk-com"># trailing</span>');
});

test("a colon inside a value is not a second key, and # inside quotes is not a comment", () => {
  assert.equal(hl("url: http://x:80/y"), '<span class="tk-key">url</span>: <span class="tk-url">http://x:80/y</span>');
  assert.equal(hl('title: "a # b"'), '<span class="tk-key">title</span>: "a # b"');
});

test("flow mappings get their keys coloured", () => {
  assert.equal(hl("request: { id: 1 }"), '<span class="tk-key">request</span>: { <span class="tk-key">id</span>: 1 }');
});

test("markup in the text is escaped", () => {
  assert.equal(hl("a: <script>&"), '<span class="tk-key">a</span>: &lt;script&gt;&amp;');
  assert.equal(hl("- <b>"), "- &lt;b&gt;");
});

test("list items without a key and blank lines survive", () => {
  assert.equal(hl("  - plain"), "  - plain");
  assert.equal(hl(""), "");
  assert.equal(hl("a: 1\n\nb: 2"), '<span class="tk-key">a</span>: <span class="tk-val">1</span>\n\n<span class="tk-key">b</span>: <span class="tk-val">2</span>');
});
