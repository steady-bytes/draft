// draft.js — tiny helpers shared by the design-system mockups.
// Theme toggle ([data-theme-toggle]) and primary swatches ([data-primary]).
// Choices persist per browser; everything still works if storage is blocked.
(function () {
  var root = document.documentElement;

  function load(key) { try { return localStorage.getItem(key); } catch (e) { return null; } }
  function save(key, v) { try { localStorage.setItem(key, v); } catch (e) {} }

  var theme = load("draft.theme");
  if (theme === "draft" || theme === "draft-light") root.setAttribute("data-theme", theme);
  var primary = load("draft.primary");
  if (primary) root.style.setProperty("--primary-base", primary);

  function label() {
    var light = root.getAttribute("data-theme") === "draft-light";
    document.querySelectorAll("[data-theme-toggle]").forEach(function (b) {
      b.textContent = light ? "Dark" : "Light";
      b.setAttribute("aria-label", light ? "Switch to dark theme" : "Switch to light theme");
    });
  }

  document.addEventListener("click", function (e) {
    var t = e.target.closest("[data-theme-toggle]");
    if (t) {
      var next = root.getAttribute("data-theme") === "draft-light" ? "draft" : "draft-light";
      root.setAttribute("data-theme", next);
      save("draft.theme", next);
      label();
      return;
    }
    var s = e.target.closest("[data-primary]");
    if (s) {
      var v = s.getAttribute("data-primary");
      root.style.setProperty("--primary-base", v);
      save("draft.primary", v);
      document.querySelectorAll("[data-primary]").forEach(function (x) {
        x.setAttribute("aria-pressed", String(x === s));
      });
    }
    var tg = e.target.closest(".d-toggle");
    if (tg) tg.setAttribute("aria-checked", String(tg.getAttribute("aria-checked") !== "true"));
  });

  document.addEventListener("DOMContentLoaded", function () {
    label();
    var current = getComputedStyle(root).getPropertyValue("--primary-base").trim().toLowerCase();
    document.querySelectorAll("[data-primary]").forEach(function (x) {
      x.setAttribute("aria-pressed", String(x.getAttribute("data-primary").toLowerCase() === current));
    });
  });
})();
