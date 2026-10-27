"use strict";
let layoutEditorSignature = "";
let savingCockpitLayout = false;
function saveCockpitLayout(ids) {
  if (savingCockpitLayout) return Promise.resolve(false);
  let saved = false;
  savingCockpitLayout = true;
  updatePickerSelection();
  document.getElementById("close-control-picker").disabled = true;
  document.querySelectorAll("#ship-control-picker input").forEach(input => { input.disabled = true; });
  document.querySelectorAll("#control-layout-editor button, #control-layout-editor select").forEach(control => { control.disabled = true; });
  return act(async () => {
    await api().SaveControlLayout(ids);
    saved = true;
    text("control-layout-message", "Cockpit layout saved.");
  }).finally(() => {
    savingCockpitLayout = false;
    layoutEditorSignature = "";
    if (state) renderControlLayoutEditor(state);
    updatePickerSelection();
    document.getElementById("close-control-picker").disabled = false;
    pickerSignature = "";
    if (state) renderShipControlPicker(state);
  }).then(() => saved);
}
function layoutAssignmentLabel(control) {
  const suffix = control.Availability === "NOT IN LAST LOADOUT" ? " · not in last loadout"
    : control.Availability === "LOADOUT UNKNOWN" ? " · loadout unknown" : "";
  return control.Label + " · " + control.Key + suffix;
}
function assignmentOptions(select, controls, used, current = "") {
  for (const [kind, label] of [["toggle", "Toggles"], ["trigger", "Activatable buttons"]]) {
    const optgroup = document.createElement("optgroup");
    optgroup.label = label;
    for (const control of controls.filter(control => controlKind(control) === kind)) {
      const option = document.createElement("option");
      option.value = control.ID;
      option.textContent = layoutAssignmentLabel(control);
      if (used.has(control.ID) && current !== control.ID) option.textContent += " · swap";
      optgroup.append(option);
    }
    select.append(optgroup);
  }
  if (current) select.value = current;
}
function renderControlLayoutEditor(data) {
  if (data.Page !== 3) return;
  const controls = data.Controls || [];
  const ids = configuredControlIDs(data);
  const signature = JSON.stringify([ids, Boolean(data.Preferences.autoAddShipControls), controls.map(c => [c.ID, c.Label, c.Key, c.Kind, c.Availability,c.Permanent])]);
  if (signature === layoutEditorSignature) return;
  layoutEditorSignature = signature;
  const catalog = new Map(controls.map(control => [control.ID, control]));
  const used = new Set(ids);
  const lists = {
    toggle: document.getElementById("toggle-layout-list"),
    trigger: document.getElementById("trigger-layout-list"),
  };
  Object.values(lists).forEach(list => list.replaceChildren());
  for (const id of ids) {
    const control = catalog.get(id);
    if (!control) continue;
    const kind = controlKind(control);
    const siblings = ids.filter(key => catalog.has(key) && controlKind(catalog.get(key)) === kind);
    const position = siblings.indexOf(id);
    const row = document.createElement("div");
    row.className = "control-layout-row";
    row.dataset.action = id;
    const field = document.createElement("div");
    const label = document.createElement("label");
    label.htmlFor = "assign-control-" + id;
    label.textContent = control.Label;
    if (permanentControl(control)) {
      const permanent = document.createElement("div");
      permanent.className = "control-layout-row";
      const body = document.createElement("div");
      const name = document.createElement("strong"); name.textContent = control.Label;
      const detail = document.createElement("p"); detail.className = "muted"; detail.textContent = control.Key + " · Permanent on every ship";
      body.append(name,detail); permanent.append(body); lists[kind].append(permanent); continue;
    }
    const select = document.createElement("select");
    select.id = label.htmlFor;
    select.dataset.reassign = id;
    assignmentOptions(select, controls.filter(c => !permanentControl(c) && (c.ID === id || shipControlAvailable(c))), used, id);
    select.addEventListener("change", () => {
      const next = [...configuredControlIDs(state)];
      const from = next.indexOf(id), other = next.indexOf(select.value);
      if (other >= 0) [next[from], next[other]] = [next[other], next[from]];
      else next[from] = select.value;
      saveCockpitLayout(next);
    });
    field.append(label, select);
    const detail = document.createElement("p");
    detail.className = "muted";
    detail.textContent = control.Description || "";
    field.append(detail);
    const actions = document.createElement("div");
    actions.className = "control-layout-row-actions";
    for (const [direction, delta] of [["up", -1], ["down", 1]]) {
      const button = document.createElement("button");
      button.type = "button";
      button.textContent = direction === "up" ? "↑" : "↓";
      button.setAttribute("aria-label", "Move " + control.Label + " " + direction);
      button.disabled = position + delta < 0 || position + delta >= siblings.length;
      button.addEventListener("click", () => {
        const next = [...configuredControlIDs(state)];
        const from = next.indexOf(id), to = next.indexOf(siblings[position + delta]);
        [next[from], next[to]] = [next[to], next[from]];
        saveCockpitLayout(next);
      });
      actions.append(button);
    }
    const remove = document.createElement("button");
    remove.type = "button";
    remove.textContent = "Remove";
    remove.dataset.remove = id;
    remove.setAttribute("aria-label", "Remove " + control.Label + " from cockpit");
    remove.disabled = Boolean(data.Preferences.autoAddShipControls) && shipControlAvailable(control);
    if (remove.disabled) remove.title = "Turn off automatic ship utilities to remove an available utility.";
    remove.addEventListener("click", () => saveCockpitLayout(configuredControlIDs(state).filter(action => action !== id)));
    actions.append(remove);
    row.append(field, actions);
    lists[kind].append(row);
  }
  for (const kind of ["toggle", "trigger"]) document.getElementById(kind + "-layout-empty").hidden = lists[kind].children.length > 0;
  document.getElementById("add-control-action-picker").disabled = savingCockpitLayout;
  document.getElementById("reset-cockpit-layout").disabled = savingCockpitLayout || Boolean(data.Preferences.autoAddShipControls) || !controls.some(control => ids.includes(control.ID) && !permanentControl(control));
  if (savingCockpitLayout) document.querySelectorAll("#control-layout-editor button, #control-layout-editor select").forEach(control => { control.disabled = true; });
}
document.getElementById("reset-cockpit-layout").addEventListener("click", () => saveCockpitLayout([]));
document.getElementById("customise-controls").addEventListener("click", () => {
  act(() => api().SelectPage(3)).then(() => {
    if (state?.Page === 3) document.getElementById("control-layout-editor").scrollIntoView({block: "start"});
  });
});
