"use strict";
let pickerSelection = new Set();
let pickerSignature = "";
function shipControlAvailable(control) {
  return ["BUILT IN", "IN LAST LOADOUT", "SIMULATED"].includes(control.Availability);
}
function updatePickerSelection() {
  const add = document.getElementById("confirm-add-controls");
  add.textContent = pickerSelection.size ? "Add selected (" + pickerSelection.size + ")" : "Add selected";
  add.disabled = !pickerSelection.size || savingCockpitLayout;
}
function renderShipControlPicker(data) {
  const dialog = document.getElementById("ship-control-picker");
  if (!dialog.open) return;
  const actions = (data.Controls || []).filter(shipControlAvailable);
  const used = new Set(configuredControlIDs(data));
  const signature = JSON.stringify([data.Ship, data.ShipModulesAt, actions.map(c => [c.ID,c.Label,c.Key,c.Availability]), [...used]]);
  if (signature === pickerSignature) return;
  pickerSignature = signature;
  const available = new Set(actions.map(control => control.ID));
  pickerSelection = new Set([...pickerSelection].filter(id => available.has(id) && !used.has(id)));
  text("control-picker-ship", data.Mode === "mock" ? "SIMULATED SHIP · " + data.Ship : "SHIP · " + (data.Ship || "waiting for ship"));
  text("control-picker-loadout", "Availability uses the last reported loadout: " + (data.ShipModulesAt || "not reported") + ".");
  for (const kind of ["toggle", "trigger"]) {
    const list = document.getElementById(kind + "-picker-list");
    list.replaceChildren();
    const options = actions.filter(control => controlKind(control) === kind);
    document.getElementById(kind + "-picker-empty").hidden = options.length > 0;
    for (const control of options) {
      const label = document.createElement("label");
      label.className = "control-picker-option";
      label.dataset.action = control.ID;
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.value = control.ID;
      checkbox.checked = used.has(control.ID) || pickerSelection.has(control.ID);
      checkbox.disabled = used.has(control.ID) || savingCockpitLayout;
      const body = document.createElement("span");
      const name = document.createElement("strong");
      name.textContent = control.Label;
      const key = document.createElement("small");
      key.textContent = control.Key + (permanentControl(control) ? " · permanent" : used.has(control.ID) ? " · already added" : "");
      const description = document.createElement("small");
      description.textContent = control.Description || "";
      body.append(name,key,description);
      checkbox.addEventListener("change", () => {
        if (checkbox.checked) pickerSelection.add(control.ID);
        else pickerSelection.delete(control.ID);
        updatePickerSelection();
      });
      label.append(checkbox,body);
      list.append(label);
    }
  }
  updatePickerSelection();
}
function openShipControlPicker() {
  if (!state) return;
  pickerSelection.clear();
  pickerSignature = "";
  document.getElementById("control-picker-error").hidden = true;
  const dialog = document.getElementById("ship-control-picker");
  if (!dialog.open) dialog.showModal();
  renderShipControlPicker(state);
}
for (const id of ["open-control-picker","add-control-action-picker"])
  document.getElementById(id).addEventListener("click",openShipControlPicker);
document.getElementById("close-control-picker").addEventListener("click", () => {
  if (!savingCockpitLayout) document.getElementById("ship-control-picker").close();
});
document.getElementById("ship-control-picker").addEventListener("cancel",event => {
  if (savingCockpitLayout) event.preventDefault();
});
document.getElementById("control-picker-stop").addEventListener("click", () => act(() => api().Disarm()));
document.getElementById("confirm-add-controls").addEventListener("click", () => {
  const ids = [...new Set([...configuredControlIDs(state),...pickerSelection])];
  saveCockpitLayout(ids).then(saved => {
    if (saved) document.getElementById("ship-control-picker").close();
    else {
      text("control-picker-error",clientError || "The ship or layout changed. Check the available controls and try again.");
      document.getElementById("control-picker-error").hidden = false;
      pickerSignature = "";
      renderShipControlPicker(state);
    }
  });
});
