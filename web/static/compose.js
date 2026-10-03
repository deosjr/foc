// Clicking a province on the map while a letter is being written inserts the
// province's name at the cursor. Everything else is htmx.
document.addEventListener("mousedown", function (e) {
  var prov = e.target.closest && e.target.closest("[data-province]");
  var ta = document.activeElement;
  if (!prov || !ta || ta.tagName !== "TEXTAREA") return;
  e.preventDefault(); // keep the focus in the letter
  var name = prov.getAttribute("data-province");
  var start = ta.selectionStart, end = ta.selectionEnd, text = ta.value;
  var before = text.slice(0, start);
  var pad = before && !/\s$/.test(before) ? " " : "";
  ta.value = before + pad + name + text.slice(end);
  var pos = start + pad.length + name.length;
  ta.setSelectionRange(pos, pos);
  ta.dispatchEvent(new Event("change", { bubbles: true })); // save the draft
});
