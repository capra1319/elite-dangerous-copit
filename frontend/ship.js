"use strict";
let shipModuleSignature = "";
function renderShipModules(data) {
  const panel = document.getElementById("ship-modules-panel");
  panel.hidden = data.Page !== 4;
  if (data.Page !== 4) return;
  const rows = data.ShipModules || [];
  text("ship-power-report", "Draw / priority report: " + (data.ShipPowerAt || "not reported") + ". Open the game’s Modules panel to refresh. Configured switch is a loadout snapshot; actual supply and overdraw shutdowns are not reported." + (data.ShipPowerError ? " Latest read: " + data.ShipPowerError : ""));
  text("ship-modules-reported", "Last loadout report: " + (data.ShipModulesAt || "not reported"));
  text("ship-module-total", String(rows.length).padStart(2, "0") + " INSTALLED");
  const damagedKey = rows.filter(row => row.Condition === "critical").length;
  const otherDamage = rows.filter(row => row.Condition === "damaged").length;
  const unknown = rows.filter(row => row.Condition === "unknown").length;
  text("ship-module-summary", rows.length
    ? damagedKey + " key systems damaged · " + otherDamage + " other damaged · " + unknown + " integrity unknown"
    : "No installed modules reported yet.");
  document.getElementById("ship-module-summary").dataset.warning = String(damagedKey > 0);
  const signature = JSON.stringify(rows);
  if (signature === shipModuleSignature) return;
  shipModuleSignature = signature;
  const list = document.getElementById("ship-module-list");
  list.replaceChildren();
  for (const row of rows) {
    const item = document.createElement("li");
    item.className = "ship-module";
    item.dataset.condition = row.Condition;
    item.dataset.keySystem = String(row.KeySystem);
    const body = document.createElement("div");
    const heading = document.createElement("div");
    heading.className = "ship-module-heading";
    const name = document.createElement("h3");
    name.textContent = row.Name;
    const label = document.createElement("span");
    label.className = "ship-module-label";
    label.textContent = (row.Condition === "critical" ? "! " : "") + row.Label;
    heading.append(name, label);
    const detail = document.createElement("p");
    detail.className = "ship-module-detail";
    detail.textContent = row.Slot;
    body.append(heading, detail);
    const power = document.createElement("dl"); power.className = "module-power-fields";
    for (const [title,value] of [["Power draw",row.Draw],["Priority",row.Priority],["Configured switch",row.ConfiguredPower],["Actual supply",row.Supply]]) {
      const field=document.createElement("div"), label=document.createElement("dt"), reading=document.createElement("dd");
      label.textContent=title; reading.textContent=value || "Unknown"; field.append(label,reading); power.append(field);
    }
    body.append(power);
    if (row.KeySystem) {
      const consequence = document.createElement("p");
      consequence.className = "ship-module-consequence";
      consequence.textContent = row.Consequence;
      body.append(consequence);
    }
    const integrity = document.createElement("div");
    integrity.className = "ship-module-integrity";
    const value = document.createElement("strong");
    value.textContent = row.Health;
    integrity.append(value);
    if (Number.isFinite(row.HealthPercent) && row.HealthPercent >= 0 && row.HealthPercent <= 100) {
      const meter = document.createElement("progress");
      meter.max = 100;
      meter.value = row.HealthPercent;
      meter.setAttribute("aria-label", row.Name + " last reported integrity");
      meter.setAttribute("aria-valuetext", row.Health + ", last reported loadout");
      integrity.append(meter);
    }
    item.append(body, integrity);
    list.append(item);
  }
}
