"use strict";

const cockpitControlDetails = {
  t1: { stencil: "FLT / 01", toggle: true },
  t2: { stencil: "UTL / 02", toggle: true },
  t3: { stencil: "VIS / 03", toggle: true },
  t4: { stencil: "CBT / 04", toggle: true },
  b1: { stencil: "VIS / 05", toggle: true },
  b2: { stencil: "NAV / 06", toggle: false },
  b3: { stencil: "NAV / 07", toggle: false },
  b4: { stencil: "CBT / 08", toggle: false },
  b5: { stencil: "UTL / 09", toggle: false },
  b6: { stencil: "CBT / 00", toggle: false },
};
let renderedControlOrder = "";
let controlGridObserver = null;

function controlKind(control) {
  return control.Kind || (cockpitControlDetails[control.ID]?.toggle ? "toggle" : "trigger");
}
function permanentControl(control) {
  return control.Permanent ?? ["t1","t2","t3","t4","b1","b2","b3","b4"].includes(control.ID);
}
function configuredControlIDs(data) {
  const permanent = (data.Controls || []).filter(permanentControl).map(control => control.ID);
  return [...new Set([...permanent,...(Array.isArray(data.ControlLayout) ? data.ControlLayout : [])])];
}
function updateControlColumns() {
  for (const container of document.querySelectorAll("#controls .control-grid")) {
    if (!container.clientWidth) continue;
    const style = getComputedStyle(container);
    const width = container.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight);
    const gap = parseFloat(style.columnGap) || 0;
    const unit = parseFloat(getComputedStyle(document.documentElement).fontSize);
    const tileWidth = Math.max(container.querySelector(".cockpit-switch")?.getBoundingClientRect().width || 7 * unit,8 * unit);
    const capacity = Math.max(1, Math.floor((width + gap) / (tileWidth + gap)));
    const requested = Number(preferences?.controlColumns || 0);
    const desired = Number.isInteger(requested) && requested >= 1 && requested <= 10 ? requested : 5;
    const columns = Math.min(desired, capacity, container.children.length || 1);
    if (container.dataset.columns !== String(columns)) {
      container.dataset.columns = String(columns);
      container.style.setProperty("--control-columns", String(columns));
    }
  }
}
window.addEventListener("pagehide", () => {
  controlGridObserver?.disconnect();
  controlGridObserver = null;
});
function createCockpitIcon(id) {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  for (const [key, value] of Object.entries({
    viewBox: "0 0 24 24", width: "48", height: "48", fill: "none",
    stroke: "currentColor", "stroke-width": "1.6",
    "stroke-linecap": "round", "stroke-linejoin": "round",
    "aria-hidden": "true", focusable: "false",
  })) svg.setAttribute(key, value);
  svg.classList.add("control-icon");
  for (const [tag, attributes] of cockpitIconNodes[id] || []) {
    const node = document.createElementNS(ns, tag);
    for (const [key, value] of Object.entries(attributes)) node.setAttribute(key, value);
    svg.append(node);
  }
  return svg;
}
function makeCockpitControl(control) {
  const details = cockpitControlDetails[control.ID];
  const group = document.createElement("div");
  group.className = "cockpit-control";
  group.dataset.action = control.ID;
  const name = document.createElement("h3");
  name.className = "control-name";
  name.id = "control-name-" + control.ID;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "cockpit-switch";
  button.dataset.action = control.ID;
  button.dataset.stencil = details.stencil;
  button.dataset.kind = controlKind(control);
  const lens = document.createElement("span");
  lens.className = "control-lens";
  const key = document.createElement("span");
  key.id = "control-key-" + control.ID;
  key.className = "control-key";
  lens.append(createCockpitIcon(control.ID), key);
  button.append(lens);
  button.addEventListener("click", () => act(() => api().PressControl(control.ID)));
  const caption = document.createElement("span");
  caption.className = "control-caption";
  caption.setAttribute("aria-hidden", "true");
  const stencil = document.createElement("span");
  stencil.className = "control-stencil";
  stencil.textContent = details.stencil;
  const note = document.createElement("span");
  note.className = "control-note";
  caption.append(stencil, note);
  group.append(name, button, caption);
  return group;
}
function renderControls(controls, layout) {
  const catalog = new Map(controls.map(control => [control.ID, control]));
  const ids = configuredControlIDs({Controls:controls,ControlLayout:layout});
  const selected = ids.map(id => catalog.get(id)).filter(control => control && cockpitControlDetails[control.ID]);
  const order = JSON.stringify(selected.map(control => [control.ID, controlKind(control)]));
  if (order !== renderedControlOrder) {
    renderedControlOrder = order;
    document.getElementById("toggle-controls").replaceChildren();
    document.getElementById("trigger-controls").replaceChildren();
    for (const control of selected) {
      const container = document.getElementById(controlKind(control) === "toggle" ? "toggle-controls" : "trigger-controls");
      container.append(makeCockpitControl(control));
    }
  }
  for (const kind of ["toggle", "trigger"]) {
    const count = selected.filter(control => controlKind(control) === kind).length;
    text(kind + "-control-count", String(count).padStart(2, "0"));
    document.getElementById(kind + "-controls-empty").hidden = count > 0;
  }
  updateControlColumns();
  if (!controlGridObserver && globalThis.ResizeObserver) {
    controlGridObserver = new ResizeObserver(updateControlColumns);
    document.querySelectorAll("#controls .control-grid").forEach(grid => controlGridObserver.observe(grid));
  }
  for (const control of selected) {
    const group = [...document.querySelectorAll("#controls .cockpit-control")].find(element => element.dataset.action === control.ID);
    const toggle = controlKind(control) === "toggle";
    const missing = control.Availability === "NOT IN LAST LOADOUT";
    const available = ["BUILT IN","IN LAST LOADOUT","SIMULATED"].includes(control.Availability);
    const button = group.querySelector("button");
    const name = group.querySelector(".control-name");
    const key = button.querySelector(".control-key");
    const note = group.querySelector(".control-note");
    if (name.textContent !== control.Label) name.textContent = control.Label;
    if (key.textContent !== control.Key) key.textContent = control.Key;
    button.dataset.state = control.State;
    button.dataset.availability = missing ? "unavailable" : available ? "available" : "unknown";
    button.disabled = !control.Enabled || !available;
    button.setAttribute("aria-label", control.Label + ", " + control.Key + ": " + control.State + (missing ? ", not in last reported loadout" : ""));
    if (available && toggle && ["ACTIVE", "INACTIVE"].includes(control.State))
      button.setAttribute("aria-pressed", String(control.State === "ACTIVE"));
    else button.removeAttribute("aria-pressed");
    const status = missing ? "UNAVAILABLE" : !available || control.State === "UNKNOWN" ? "UNKNOWN" : toggle ? "" : "ACTION";
    if (note.textContent !== status) note.textContent = status;
    button.title = control.Label + ": " + control.State + ". " + (
      missing ? "Launcher not in the last reported loadout; check Ship details."
        : control.Enabled ? toggle ? "Request this control; the glass follows reported game state."
          : "Momentary action; the glass does not indicate confirmation."
        : "Focus Elite and press Ctrl+Alt+F9 to arm first."
    );
  }
}
