"use strict";
function pinMaterial(key, pinned) {
  const simulated = state?.Mode === "mock";
  act(() => api().SetMaterialPinned(key, pinned, simulated));
}
function materialPinIcon() {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  const path = document.createElementNS(ns, "path");
  path.setAttribute("d", "M8 3h8v2l-2 1v5l3 3v2H7v-2l3-3V6L8 5Z M12 16v6");
  svg.append(path);
  return svg;
}
function renderMaterialRows(data) {
  const list = document.getElementById("material-list");
  const rows = data.Rows || [];
  const signature = JSON.stringify([rows, data.PinsError, state?.Mode]);
  if (signature !== rowSignature) {
    rowSignature = signature;
    const existing = new Map([...list.children].map(node => [node.dataset.key, node]));
    const focused = list.contains(document.activeElement) ? document.activeElement : null;
    for (const [index, row] of rows.entries()) {
      let item = existing.get(row.Key);
      if (!item) {
        item = document.createElement("div");
        item.className = "material-row";
        item.dataset.key = row.Key;
        item.setAttribute("role", "listitem");
        const pin = document.createElement("button");
        pin.type = "button";
        pin.className = "material-pin";
        pin.append(materialPinIcon());
        pin.addEventListener("click", () => pinMaterial(row.Key, pin.getAttribute("aria-pressed") !== "true"));
        const select = document.createElement("button");
        select.type = "button";
        select.className = "material-select";
        select.setAttribute("aria-controls", "material-details");
        for (const name of ["name", "stock", "comment"]) {
          const cell = document.createElement("span");
          cell.className = "material-cell material-" + name;
          select.append(cell);
        }
        select.addEventListener("click", () => act(() => api().SelectMaterial(row.Key)));
        item.append(pin, select);
      }
      existing.delete(row.Key);
      const [pin, select] = item.children;
      pin.setAttribute("aria-pressed", String(row.Pinned));
      pin.title = (row.Pinned ? "Unpin " : "Pin ") + row.Display;
      pin.setAttribute("aria-label", pin.title);
      pin.disabled = Boolean(data.PinsError);
      select.setAttribute("aria-label", "View " + row.Display + " details");
      const [name, stock, comment] = select.children;
      name.textContent = row.Display;
      name.title = row.Display;
      stock.textContent = `${row.Known ? row.Count : "?"} / ${row.Target || "—"}`;
      comment.textContent = row.Note || "—";
      comment.title = row.Note || "No saved comment";
      if (list.children[index] !== item) list.insertBefore(item, list.children[index] || null);
    }
    for (const node of existing.values()) node.remove();
    if (focused?.isConnected && document.activeElement !== focused) focused.focus({ preventScroll: true });
  }
  for (const item of list.children) {
    const selected = item.dataset.key === data.Selected;
    item.classList.toggle("is-selected", selected);
    item.querySelector(".material-select").setAttribute("aria-expanded", String(selected));
  }
}
document.getElementById("pin-material").addEventListener("click", () => {
  if (!selectedMaterial) return;
  const row = state?.Engineering.Rows.find(row => row.Key === selectedMaterial);
  if (row) pinMaterial(row.Key, !row.Pinned);
});
document.getElementById("close-material-details").addEventListener("click", () => {
  act(() => api().SelectMaterial("")).then(() => document.getElementById("material-list").focus({ preventScroll: true }));
});
