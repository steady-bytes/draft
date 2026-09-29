/* draft boot — restores the saved theme and primary before first paint.
 * Inline this (or load it synchronously in <head>). Preferences live in a cookie shared across
 * sibling subdomains (bench.draft.localhost ↔ blueprint.draft.localhost) with localStorage as the
 * fallback where the browser rejects a parent-domain cookie. */
(function () {
  var root = document.documentElement;
  var D = (window.draftPrefs = {
    get: function (k) {
      var m = document.cookie.match(new RegExp("(?:^|; )" + k.replace(/\./g, "\\.") + "=([^;]*)"));
      if (m) return decodeURIComponent(m[1]);
      try { return localStorage.getItem(k); } catch (e) { return null; }
    },
    set: function (k, v) {
      try { localStorage.setItem(k, v); } catch (e) {}
      var enc = k + "=" + encodeURIComponent(v);
      var base = enc + "; Path=/; Max-Age=31536000; SameSite=Lax";
      var parts = location.hostname.split(".");
      // An IP address (127.0.0.1) has no parent domain to share a cookie on.
      if (parts.length >= 3 && !/^[\d.]+$/.test(location.hostname)) {
        document.cookie = base + "; Domain=" + parts.slice(1).join(".");
        // Accepted only if this exact value reads back (an older cookie of the same name must not
        // pass for it), and then a host-only copy is retired so it cannot shadow the shared one.
        if (document.cookie.split("; ").indexOf(enc) !== -1) {
          document.cookie = k + "=; Path=/; Max-Age=0";
          return;
        }
      }
      document.cookie = base; // host-only fallback
    },
  });
  var theme = D.get("draft.theme");
  if (theme === "draft" || theme === "draft-light") root.setAttribute("data-theme", theme);
  var primary = D.get("draft.primary");
  if (primary && /^#[0-9a-f]{6}$/i.test(primary)) root.style.setProperty("--primary-base", primary);
})();
/* draft behaviours — progressive enhancement for server-rendered pages.
 *
 *   [data-theme-toggle]              flips draft ⇄ draft-light
 *   [data-primary="#rrggbb"]         primary swatch buttons
 *   .d-toggle                        flips aria-checked, fires `d:toggle` (bubbles; detail.checked)
 *   [data-copy="#id"]                copies that element's text
 *   form[data-confirm="msg"]         confirm() before submit
 *   textarea[data-editor]            line-number gutter, cursor readout, ⌘S / ⌘↵ (see below);
 *                                    data-highlight="#pre-id" colours the YAML in an overlay behind it
 *   .d-trace                         sets --fail-at from the failing step
 *   [data-shortcut="/"]              focuses this element when that key is pressed outside a field
 *   [data-insert-into="#id"]         appends its data-insert text to that textarea (a plugin snippet)
 *   [data-focus="#id"]               focuses that element when clicked
 *   input[data-yaml-field="metadata.name"][data-yaml-target="#id"]
 *                                    keeps a form field and one `metadata:` key of a YAML textarea in step
 *
 * Re-initialises after htmx swaps (`htmx:afterSwap`). No dependencies. */
(function () {
  var root = document.documentElement;
  var D = window.draftPrefs;

  // Theme ------------------------------------------------------------------------------------------
  function labelThemeButtons() {
    var light = root.getAttribute("data-theme") === "draft-light";
    document.querySelectorAll("[data-theme-toggle]").forEach(function (b) {
      b.textContent = light ? "Dark" : "Light";
      b.setAttribute("aria-label", light ? "Switch to dark theme" : "Switch to light theme");
    });
  }

  function markSwatches() {
    var cur = getComputedStyle(root).getPropertyValue("--primary-base").trim().toLowerCase();
    document.querySelectorAll("[data-primary]").forEach(function (x) {
      x.setAttribute("aria-pressed", String(x.getAttribute("data-primary").toLowerCase() === cur));
    });
  }

  // Click delegation -------------------------------------------------------------------------------
  document.addEventListener("click", function (e) {
    var t = e.target.closest("[data-theme-toggle]");
    if (t) {
      var next = root.getAttribute("data-theme") === "draft-light" ? "draft" : "draft-light";
      root.setAttribute("data-theme", next);
      D.set("draft.theme", next);
      labelThemeButtons();
      return;
    }
    var s = e.target.closest("[data-primary]");
    if (s) {
      var v = s.getAttribute("data-primary");
      root.style.setProperty("--primary-base", v);
      D.set("draft.primary", v);
      markSwatches();
      return;
    }
    var tg = e.target.closest(".d-toggle");
    if (tg && !tg.disabled) {
      var on = tg.getAttribute("aria-checked") !== "true";
      tg.setAttribute("aria-checked", String(on));
      tg.dispatchEvent(new CustomEvent("d:toggle", { bubbles: true, detail: { checked: on } }));
      return;
    }
    var fo = e.target.closest("[data-focus]");
    if (fo) {
      var to = document.querySelector(fo.getAttribute("data-focus"));
      if (to) to.focus();
      return;
    }
    var ins = e.target.closest("[data-insert-into]");
    if (ins) {
      var ta = document.querySelector(ins.getAttribute("data-insert-into"));
      if (!ta) return;
      var text = ins.getAttribute("data-insert") || "";
      // Appended at the end, not at the cursor: a snippet dropped mid-indentation would corrupt the
      // YAML around it. A blank line is not added; only a missing final newline is.
      ta.value += (ta.value === "" || ta.value.slice(-1) === "\n" ? "" : "\n") + text;
      ta.dispatchEvent(new Event("input", { bubbles: true }));
      ta.scrollTop = ta.scrollHeight;
      ta.focus();
      return;
    }
    var c = e.target.closest("[data-copy]");
    if (c) {
      var src = document.querySelector(c.getAttribute("data-copy"));
      if (!src) return;
      var original = c.getAttribute("data-label") || c.textContent;
      c.setAttribute("data-label", original);
      var done = function (msg) {
        c.textContent = msg;
        setTimeout(function () { c.textContent = original; }, 1400);
      };
      if (navigator.clipboard) {
        navigator.clipboard.writeText(src.innerText).then(function () { done("Copied"); }, function () { done("Select to copy"); });
      } else {
        done("Select to copy");
      }
    }
  });

  // Keyboard shortcuts: <input data-shortcut="/"> is focused by pressing "/" anywhere else.
  document.addEventListener("keydown", function (e) {
    if (e.metaKey || e.ctrlKey || e.altKey || !e.key || e.key.length !== 1) return;
    var t = e.target;
    if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)) return;
    var el = document.querySelector('[data-shortcut="' + e.key.replace(/["\\]/g, "") + '"]');
    if (el) {
      e.preventDefault();
      el.focus();
    }
  });

  document.addEventListener("submit", function (e) {
    var msg = e.target.getAttribute && e.target.getAttribute("data-confirm");
    if (msg && !window.confirm(msg)) e.preventDefault();
  });

  // Editor -----------------------------------------------------------------------------------------
  // <textarea data-editor data-gutter="gutter-id" data-cursor="#cursor" data-lines="#lines">
  // Error lines: an element `<div hidden data-error-lines-for="textarea-id">24,25</div>` anywhere on
  // the page (re-read after every htmx swap).
  // YAML colouring for the editor's overlay ---------------------------------------------------------
  // Deliberately small: keys, strings, URLs, literals and comments, line by line. It has no idea of
  // block scalars or anchors and does not need to; the text area stays the source of truth.
  function esc(s) { return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;"); }
  function span(cls, s) { return '<span class="' + cls + '">' + esc(s) + "</span>"; }

  function commentStart(v) {
    var q = "";
    for (var i = 0; i < v.length; i++) {
      var c = v[i];
      if (q) { if (c === q) q = ""; }
      else if (c === '"' || c === "'") q = c;
      else if (c === "#" && (i === 0 || /\s/.test(v[i - 1]))) return i;
    }
    return -1;
  }

  function hlScalar(v) {
    if (v === "") return "";
    var ci = commentStart(v), comment = "";
    if (ci >= 0) { comment = span("tk-com", v.slice(ci)); v = v.slice(0, ci); }
    var lead = /^\s*/.exec(v)[0], t = v.slice(lead.length), trail = /\s*$/.exec(t)[0];
    t = t.slice(0, t.length - trail.length);
    var out;
    if (t === "") out = "";
    else if (/^[a-z][a-z0-9+.\-]*:\/\//i.test(t)) out = span("tk-url", t);
    else if (/^(true|false|null|~|yes|no|ok|-?\d+(\.\d+)?|\$\{.*\})$/i.test(t)) out = span("tk-val", t);
    else if (t[0] === "{" || t[0] === "[") {
      // Flow style: colour the keys inside, leave the rest.
      out = esc(t).replace(/([A-Za-z0-9_.\-]+)(\s*:)(?=\s)/g, '<span class="tk-key">$1</span>$2');
    } else out = esc(t);
    return esc(lead) + out + esc(trail) + comment;
  }

  var YAML_KEY = /^(\s*(?:-\s+)*)("[^"]*"|'[^']*'|[^\s#:"'{\[\-][^:#]*?)(:)(?=\s|$)(.*)$/;

  function highlightYaml(text) {
    return text.split("\n").map(function (line) {
      var m = YAML_KEY.exec(line);
      if (m) return esc(m[1]) + span("tk-key", m[2]) + ":" + hlScalar(m[4]);
      if (/^\s*#/.test(line)) return span("tk-com", line);
      m = /^(\s*-\s+)(.*)$/.exec(line);
      if (m) return esc(m[1]) + hlScalar(m[2]);
      return esc(line);
    }).join("\n");
  }
  window.draftYaml = window.draftYaml || {};
  window.draftYaml.highlight = highlightYaml;

  function initEditor(ta) {
    if (ta._draft) return;
    ta._draft = true;
    var gutter = document.getElementById(ta.getAttribute("data-gutter"));
    var cursorEl = document.querySelector(ta.getAttribute("data-cursor") || "[data-none]");
    var linesEl = document.querySelector(ta.getAttribute("data-lines") || "[data-none]");
    var hlEl = ta.getAttribute("data-highlight") ? document.querySelector(ta.getAttribute("data-highlight")) : null;

    function errorLines() {
      var el = document.querySelector('[data-error-lines-for="' + ta.id + '"]');
      return el ? el.textContent.split(",").map(function (n) { return parseInt(n, 10); }) : [];
    }

    function render() {
      var n = ta.value.split("\n").length;
      var bad = errorLines();
      if (gutter) {
        var out = "";
        for (var i = 1; i <= n; i++) out += (bad.indexOf(i) >= 0 ? '<span class="is-err">' + i + "</span>" : i) + "\n";
        gutter.innerHTML = out;
        gutter.scrollTop = ta.scrollTop;
      }
      if (linesEl) linesEl.textContent = n + " lines";
      if (hlEl) {
        // The trailing space keeps the overlay as tall as the text area when the text ends in a newline.
        hlEl.innerHTML = highlightYaml(ta.value) + "\n ";
        hlEl.scrollTop = ta.scrollTop;
        hlEl.scrollLeft = ta.scrollLeft;
      }
    }

    function cursor() {
      if (!cursorEl) return;
      var before = ta.value.slice(0, ta.selectionStart).split("\n");
      cursorEl.textContent = before.length + ":" + (before[before.length - 1].length + 1);
    }

    ta.addEventListener("input", function () { render(); cursor(); });
    ta.addEventListener("scroll", function () {
      if (gutter) gutter.scrollTop = ta.scrollTop;
      if (hlEl) { hlEl.scrollTop = ta.scrollTop; hlEl.scrollLeft = ta.scrollLeft; }
    });
    ["keyup", "click", "focus"].forEach(function (ev) { ta.addEventListener(ev, cursor); });
    ta.addEventListener("d:errors", render);
    ta.addEventListener("keydown", function (e) {
      if (!(e.metaKey || e.ctrlKey)) return;
      var form = ta.form;
      if (!form) return;
      if (e.key === "s") {
        e.preventDefault();
        form.requestSubmit();
      } else if (e.key === "Enter") {
        e.preventDefault();
        var run = form.querySelector('input[name="run"]');
        if (run) run.value = "1";
        form.requestSubmit();
      }
    });
    if (hlEl && hlEl.parentNode) hlEl.parentNode.classList.add("is-hl");
    render();
    cursor();
  }

  // YAML fields -----------------------------------------------------------------------------------
  // A form field bound to `metadata.<key>` of a YAML textarea. The text is the source of truth (the
  // form posts only the textarea): typing in the field rewrites that one line, typing in the
  // textarea refreshes the field. Only single-line scalars are bound; a block scalar (`>`, `|`) is
  // left alone.
  function metaLine(text, key) {
    var lines = text.split("\n"), start = -1, i;
    for (i = 0; i < lines.length; i++) if (/^metadata:\s*(#.*)?$/.test(lines[i])) { start = i; break; }
    if (start < 0) return { lines: lines, start: -1 };
    var end = lines.length;
    for (i = start + 1; i < lines.length; i++) {
      if (lines[i].trim() !== "" && !/^\s/.test(lines[i])) { end = i; break; }
    }
    var re = new RegExp("^(\\s+)" + key + ":\\s*(.*)$");
    for (i = start + 1; i < end; i++) {
      var m = re.exec(lines[i]);
      if (m) return { lines: lines, start: start, end: end, at: i, indent: m[1], raw: m[2] };
    }
    return { lines: lines, start: start, end: end, at: -1 };
  }

  function unquote(raw) {
    var v = raw.replace(/\s+#.*$/, "").trim();
    if (/^"[\s\S]*"$/.test(v)) { try { return JSON.parse(v); } catch (e) { return v.slice(1, -1); } }
    if (/^'[\s\S]*'$/.test(v)) return v.slice(1, -1).replace(/''/g, "'");
    return v;
  }

  function quote(v) {
    var plain = /^[A-Za-z0-9_./@-][A-Za-z0-9_ ./@-]*$/.test(v) && !/^(true|false|null|yes|no|on|off|~)$/i.test(v) && !/\s$/.test(v);
    return plain ? v : JSON.stringify(v);
  }

  function readYamlField(text, key) {
    var m = metaLine(text, key);
    return m.at >= 0 && !/^[|>]/.test(m.raw) ? unquote(m.raw) : null;
  }

  function writeYamlField(text, key, value) {
    var m = metaLine(text, key);
    if (m.start < 0 || (m.at >= 0 && /^[|>]/.test(m.raw))) return text;
    var line = (m.at >= 0 ? m.indent : "  ") + key + ": " + quote(value);
    if (m.at >= 0) m.lines[m.at] = line;
    else m.lines.splice(m.end, 0, line);
    return m.lines.join("\n");
  }

  // Exposed for tests/yaml_field.test.mjs.
  window.draftYaml = window.draftYaml || {};
  window.draftYaml.read = readYamlField;
  window.draftYaml.write = writeYamlField;

  function initYamlField(field) {
    if (field._draft) return;
    field._draft = true;
    var ta = document.querySelector(field.getAttribute("data-yaml-target"));
    var key = (field.getAttribute("data-yaml-field") || "").replace(/^metadata\./, "");
    if (!ta || !key) return;
    field.addEventListener("input", function () {
      var next = writeYamlField(ta.value, key, field.value);
      if (next !== ta.value) {
        ta.value = next;
        ta.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });
    ta.addEventListener("input", function () {
      if (document.activeElement === field) return;
      var v = readYamlField(ta.value, key);
      if (v !== null) field.value = v;
    });
  }

  // Trace ------------------------------------------------------------------------------------------
  function placeTrace(scope) {
    (scope || document).querySelectorAll(".d-trace").forEach(function (tr) {
      var fail = tr.querySelector(".d-step.is-fail");
      tr.style.setProperty("--fail-at", fail ? fail.offsetTop + "px" : "100%");
    });
  }

  function init(scope) {
    (scope || document).querySelectorAll("textarea[data-editor]").forEach(initEditor);
    (scope || document).querySelectorAll("[data-yaml-field]").forEach(initYamlField);
    placeTrace(scope);
  }

  document.addEventListener("DOMContentLoaded", function () {
    labelThemeButtons();
    markSwatches();
    init();
  });
  // Out-of-band swaps (the editor's problems list) do not always come with an afterSwap on the
  // triggering element, so the gutter re-reads its error lines on either event.
  document.addEventListener("htmx:oobAfterSwap", function () {
    document.querySelectorAll("textarea[data-editor]").forEach(function (ta) { ta.dispatchEvent(new Event("d:errors")); });
  });
  document.addEventListener("htmx:afterSwap", function (e) {
    init(e.target && e.target.querySelectorAll ? e.target : document);
    document.querySelectorAll("textarea[data-editor]").forEach(function (ta) { ta.dispatchEvent(new Event("d:errors")); });
  });
  window.addEventListener("resize", function () { placeTrace(); });
  if (document.fonts) document.fonts.ready.then(function () { placeTrace(); });
})();
