// Tests for js/boot.js's preference storage (theme and primary). Run with `node --test tests/`.
//
// The script keeps preferences in a cookie shared by sibling subdomains, so the fake here models
// just enough of a browser's cookie jar to tell the cases apart: host-only vs Domain cookies, a
// Domain the browser rejects (an IP address), and reading back only the cookies a host can see.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const src = readFileSync(join(dirname(fileURLToPath(import.meta.url)), "..", "js", "boot.js"), "utf8");

// A jar shared by several "pages" (hosts), like one browser profile.
function browser() {
  const jar = []; // { name, value, domain, hostOnly }
  const storage = new Map();
  function page(hostname) {
    const attrs = { setAttribute() {}, style: { setProperty() {} } };
    const root = { ...attrs, attrs: {}, setAttribute(k, v) { this.attrs[k] = v; } };
    const document = {
      documentElement: root,
      get cookie() {
        return jar
          .filter((c) => (c.hostOnly ? c.domain === hostname : hostname === c.domain || hostname.endsWith("." + c.domain)))
          .map((c) => `${c.name}=${c.value}`)
          .join("; ");
      },
      set cookie(line) {
        const [pair, ...rest] = line.split(";").map((s) => s.trim());
        const eq = pair.indexOf("=");
        const name = pair.slice(0, eq), value = pair.slice(eq + 1);
        const opts = Object.fromEntries(rest.map((r) => { const i = r.indexOf("="); return i < 0 ? [r.toLowerCase(), true] : [r.slice(0, i).toLowerCase(), r.slice(i + 1)]; }));
        let domain = hostname, hostOnly = true;
        if (opts.domain) {
          const d = String(opts.domain);
          const isIp = /^[\d.]+$/.test(hostname);
          if (isIp || !(hostname === d || hostname.endsWith("." + d)) || !d.includes(".")) return; // rejected
          domain = d; hostOnly = false;
        }
        const at = jar.findIndex((c) => c.name === name && c.domain === domain && c.hostOnly === hostOnly);
        if (at >= 0) jar.splice(at, 1);
        if (opts["max-age"] === "0") return;
        jar.push({ name, value, domain, hostOnly });
      },
    };
    const localStorage = { getItem: (k) => storage.get(k) ?? null, setItem: (k, v) => storage.set(k, String(v)) };
    const window = {};
    new Function("document", "location", "localStorage", "window", src)(document, { hostname }, localStorage, window);
    return { prefs: window.draftPrefs, root };
  }
  return { page, jar, storage };
}

test("a saved theme is applied before first paint", () => {
  const b = browser();
  b.page("bench.draft.localhost").prefs.set("draft.theme", "draft-light");
  assert.equal(b.page("bench.draft.localhost").root.attrs["data-theme"], "draft-light");
});

test("sibling subdomains share the preference", () => {
  const b = browser();
  b.page("bench.draft.localhost").prefs.set("draft.theme", "draft-light");
  assert.equal(b.page("blueprint.draft.localhost").prefs.get("draft.theme"), "draft-light");
});

test("an IP host can change the preference more than once", () => {
  // Regression: the parent-domain write is rejected for 127.0.0.1, and the old check took the
  // existing host-only cookie as proof it had been accepted, so the second change never landed.
  const b = browser();
  const p = b.page("127.0.0.1").prefs;
  p.set("draft.theme", "draft-light");
  p.set("draft.theme", "draft");
  assert.equal(b.page("127.0.0.1").prefs.get("draft.theme"), "draft");
  p.set("draft.theme", "draft-light");
  assert.equal(b.page("127.0.0.1").prefs.get("draft.theme"), "draft-light");
});

test("a stale host-only cookie does not shadow a newer shared one", () => {
  const b = browser();
  b.page("localhost").prefs.set("draft.theme", "draft"); // host-only, from before subdomains were used
  b.jar.push({ name: "draft.theme", value: "draft", domain: "bench.draft.localhost", hostOnly: true });
  const p = b.page("bench.draft.localhost").prefs;
  p.set("draft.theme", "draft-light");
  assert.equal(b.page("bench.draft.localhost").prefs.get("draft.theme"), "draft-light");
  assert.equal(b.page("blueprint.draft.localhost").prefs.get("draft.theme"), "draft-light");
});

test("with no cookie the value falls back to localStorage", () => {
  const b = browser();
  b.storage.set("draft.primary", "#3ec8a0");
  assert.equal(b.page("127.0.0.1").prefs.get("draft.primary"), "#3ec8a0");
});

test("a malformed primary is ignored", () => {
  const b = browser();
  b.storage.set("draft.primary", "red; drop table");
  const { root } = b.page("127.0.0.1");
  assert.equal(root.attrs["data-theme"], undefined);
});
