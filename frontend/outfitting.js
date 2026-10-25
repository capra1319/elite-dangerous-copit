"use strict";
let outfitStationSignature = "",
  outfitModuleSignature = "",
  outfitWasOpen = false,
  outfitSystemDirty = false,
  outfitFilterTimer = null;
const outfitEl = (id) => document.getElementById(id);
function outfitSelectOptions(id, values, allLabel) {
  const select = outfitEl(id);
  const signature = JSON.stringify(values);
  if (select.dataset.options === signature) return;
  select.dataset.options = signature;
  select.replaceChildren();
  if (allLabel) {
    const all = document.createElement("option");
    all.value = "";
    all.textContent = allLabel;
    select.append(all);
  }
  for (const value of values) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = value;
    select.append(option);
  }
}
function renderOutfitting(data) {
  const o = data.Outfitting;
  const active = data.Page === 5 && o.Active;
  outfitEl("outfitting-launch").hidden = data.Page !== 5 || active;
  outfitEl("outfitting-view").hidden = !active;
  if (active !== outfitWasOpen) outfitEl("main").scrollTop = 0;
  if (!active) {
    if (outfitWasOpen) clearTimeout(outfitFilterTimer);
    outfitWasOpen = false;
    return;
  }
  if (!outfitWasOpen) {
    outfitSystemDirty = false;
    outfitEl("outfitting-system").value = o.Input || data.System || "";
  }
  outfitWasOpen = true;
  if (
    !outfitSystemDirty &&
    document.activeElement !== outfitEl("outfitting-system")
  )
    outfitEl("outfitting-system").value = o.Input || "";
  text("page-title", "Station outfitting");
  text(
    "outfitting-source",
    o.Simulated
      ? "SIMULATED · no online request"
      : "EDSM · last-reported availability",
  );
  text(
    "outfitting-message",
    o.Loading
      ? "Loading from " + (o.Simulated ? "the simulator…" : "EDSM…")
      : o.Message || "Enter a system, load stations, then choose one.",
  );
  outfitEl("outfitting-cancel").hidden = !o.Loading;
  outfitEl("outfitting-refresh").disabled = o.Loading;
  outfitEl("outfitting-error").hidden = !o.Error;
  if (o.Error) text("outfitting-error", o.Error);
  const stations = o.Stations || [];
  text(
    "outfit-station-title",
    o.System ? `Stations in ${o.System}` : "Stations",
  );
  if (document.activeElement !== outfitEl("outfit-station-filter"))
    outfitEl("outfit-station-filter").value = o.StationFilter || "";
  const stationSignature = JSON.stringify(stations);
  if (stationSignature !== outfitStationSignature) {
    outfitStationSignature = stationSignature;
    outfitEl("outfit-stations").replaceChildren();
    for (const station of stations) {
      const row = document.createElement("button");
      row.type = "button";
      row.className = "outfit-station-row";
      row.dataset.key = station.Key;
      row.setAttribute("role", "option");
      const name = document.createElement("strong");
      name.textContent = station.Name;
      const detail = document.createElement("span");
      const distance =
        station.Distance == null
          ? "arrival distance unknown"
          : `${Math.round(station.Distance).toLocaleString()} ls`;
      detail.textContent = `${station.Type} · ${distance}`;
      const service = document.createElement("small");
      service.textContent = station.HaveOutfitting
        ? "Outfitting reported"
        : "Outfitting not reported";
      row.append(name, detail, service);
      row.addEventListener("click", () =>
        act(() => api().SelectOutfittingStation(station.Key, false)),
      );
      outfitEl("outfit-stations").append(row);
    }
  }
  for (const row of outfitEl("outfit-stations").children) {
    row.setAttribute(
      "aria-selected",
      String(row.dataset.key === o.SelectedKey),
    );
    row.disabled = o.Loading && row.dataset.key === o.SelectedKey;
  }
  outfitEl("outfit-stations-empty").hidden = stations.length > 0;
  text(
    "outfit-stations-empty",
    o.Loading && o.Stage === "stations"
      ? "Loading stations…"
      : o.System
        ? "No matching stations reported."
        : "Enter a system and load its stations.",
  );
  text("outfit-station-name", o.Station?.Name || "Select a station");
  text(
    "outfit-station-description",
    o.Station ? `${o.Station.System} · ${o.Station.Type}` : "",
  );
  text("outfit-update-age", o.Station ? o.UpdatedLabel : "");
  outfitEl("outfit-update-age").dataset.old = String(o.OldReport);
  outfitSelectOptions("outfit-category", o.Categories || ["All types"]);
  outfitSelectOptions("outfit-class", o.Classes || [], "All");
  outfitSelectOptions("outfit-rating", o.Ratings || [], "All");
  for (const [id, value] of [
    ["outfit-category", o.Category || "All types"],
    ["outfit-class", o.Class || ""],
    ["outfit-rating", o.Rating || ""],
  ]) {
    if (outfitEl(id).value !== value) outfitEl(id).value = value;
  }
  if (document.activeElement !== outfitEl("outfit-module-filter"))
    outfitEl("outfit-module-filter").value = o.Filter || "";
  const modules = o.Modules || [];
  const moduleSignature = JSON.stringify(modules);
  if (moduleSignature !== outfitModuleSignature) {
    outfitModuleSignature = moduleSignature;
    outfitEl("outfit-modules").replaceChildren();
    for (const module of modules) {
      const row = document.createElement("tr");
      const name = document.createElement("td");
      name.className = "outfit-module-name";
      const label = document.createElement("span");
      label.textContent = module.Name;
      name.append(label);
      if (module.Ship) {
        const ship = document.createElement("small");
        ship.textContent = `For ${module.Ship}`;
        name.append(ship);
      }
      const typeSmall = document.createElement("small");
      typeSmall.className = "outfit-type-small";
      typeSmall.textContent = module.Category;
      name.append(typeSmall);
      const size = document.createElement("td");
      size.textContent = module.Class == null ? "?" : String(module.Class);
      const rating = document.createElement("td");
      rating.textContent = module.Rating || "?";
      const category = document.createElement("td");
      category.className = "outfit-type-column";
      category.textContent = module.Category;
      row.append(name, size, rating, category);
      outfitEl("outfit-modules").append(row);
    }
  }
  text(
    "outfit-module-count",
    o.Loading && o.Stage === "modules"
      ? "Loading reported modules…"
      : o.Error && o.Stage === "modules"
        ? "Module report unavailable"
        : o.Stage === "modules"
          ? `${modules.length} shown / ${o.TotalModules} modules reported`
      : "",
  );
  outfitEl("outfit-modules-empty").hidden = modules.length > 0;
  text(
    "outfit-modules-empty",
    o.Loading && o.Stage === "modules"
      ? "Loading reported modules…"
      : o.Error
        ? "No result loaded. Resolve the lookup error or try another station."
        : o.Stage === "modules"
          ? "No matching module data reported. Try clearing filters."
          : "Choose a station to load its reported outfitting.",
  );
  text(
    "outfit-retrieved",
    o.RetrievedLabel
      ? `${o.RetrievedLabel}${o.Cached ? " · using memory cache (up to 10 minutes)" : ""}`
      : "",
  );
}
function applyOutfitFilters() {
  if (!state?.Outfitting.Active) return;
  const values = [
    "outfit-station-filter",
    "outfit-module-filter",
    "outfit-category",
    "outfit-class",
    "outfit-rating",
  ].map((id) => outfitEl(id).value);
  act(() => api().FilterOutfitting(...values));
}
function refreshOutfitting() {
  const o = state?.Outfitting;
  if (!o || o.Loading) return;
  if (o.Station?.Key && o.Stage === "modules")
    act(() => api().SelectOutfittingStation(o.Station.Key, true));
  else {
    const system = outfitEl("outfitting-system").value;
    act(() => api().SearchOutfittingStations(system, true));
  }
}
outfitEl("outfitting-view-open").addEventListener("click", () =>
  act(() => api().ShowOutfitting(true)),
);
outfitEl("outfitting-back").addEventListener("click", () =>
  act(() => api().ShowOutfitting(false)),
);
outfitEl("outfitting-browser").addEventListener("click", () =>
  act(() => api().OpenStationSearch(1)),
);
outfitEl("outfitting-system").addEventListener("input", () => {
  outfitSystemDirty = true;
});
outfitEl("outfitting-current").addEventListener("click", () => {
  outfitEl("outfitting-system").value = state?.System || "";
  outfitSystemDirty = true;
  outfitEl("outfitting-system").focus();
});
outfitEl("outfitting-system-form").addEventListener("submit", (event) => {
  event.preventDefault();
  clearTimeout(outfitFilterTimer);
  const system = outfitEl("outfitting-system").value;
  act(async () => {
    await api().SearchOutfittingStations(system, false);
    outfitSystemDirty = false;
  });
});
outfitEl("outfitting-cancel").addEventListener("click", () =>
  act(() => api().CancelOutfitting()),
);
outfitEl("outfitting-refresh").addEventListener("click", refreshOutfitting);
for (const id of ["outfit-station-filter", "outfit-module-filter"])
  outfitEl(id).addEventListener("input", () => {
    clearTimeout(outfitFilterTimer);
    outfitFilterTimer = setTimeout(applyOutfitFilters, 100);
  });
for (const id of ["outfit-category", "outfit-class", "outfit-rating"])
  outfitEl(id).addEventListener("change", () => {
    clearTimeout(outfitFilterTimer);
    applyOutfitFilters();
  });
window.addEventListener("beforeunload", () => clearTimeout(outfitFilterTimer));
