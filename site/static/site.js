// Copy buttons, and the language menu. The page works without this file:
// the copy buttons stay hidden and the menu still opens.
(function () {
  "use strict";

  if (navigator.clipboard) {
    document.querySelectorAll("button[data-copy]").forEach(function (btn) {
      var label = btn.textContent;
      btn.hidden = false;
      btn.setAttribute("aria-live", "polite");
      btn.addEventListener("click", function () {
        var box = btn.closest(".code, .install");
        var pre = box && box.querySelector("pre");
        if (!pre) return;
        navigator.clipboard.writeText(pre.textContent.trim()).then(function () {
          btn.textContent = btn.getAttribute("data-done");
          setTimeout(function () { btn.textContent = label; }, 1600);
        }, function () {});
      });
    });
  }

  // The root page sends visitors to the language they picked last.
  document.querySelectorAll("a[data-lang]").forEach(function (a) {
    a.addEventListener("click", function () {
      try { localStorage.setItem("lang", a.getAttribute("data-lang")); } catch (e) {}
    });
  });

  var menu = document.querySelector("details.lang");
  if (menu) {
    document.addEventListener("click", function (e) {
      if (menu.open && !menu.contains(e.target)) menu.open = false;
    });
    document.addEventListener("keydown", function (e) {
      if (e.key === "Escape" && menu.open) {
        menu.open = false;
        menu.querySelector("summary").focus();
      }
    });
  }
})();
