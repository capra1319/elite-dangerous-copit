/* The native backend owns arming, process identity and all game input checks. */
"use strict";
const $ = (id) => document.getElementById(id);
const api = () => window.go?.main?.DesktopApp;
let state = null,
  preferences = null,
  renderedPage = -1,
  navigationSignature = "",
  rowSignature = "",
  selectedMaterial = "",
  clientError = "",
  stopping = false,
  polling = false,
  filterTimer = null;
let mutation = Promise.resolve();
let materialTargetDraft = null;
function text(id, value, fallback = "Unavailable") {
  const next = String(value ?? fallback);
  if ($(id).textContent !== next) $(id).textContent = next;
}
function message(error) {
  clientError = String(error?.message || error);
  text("error", clientError);
  $("error").hidden = false;
}
function act(work) {
  mutation = mutation
    .then(async () => {
      if (!api()) throw Error("Desktop bridge is not available.");
      clientError = "";
      await work();
      await refresh();
    })
    .catch(message);
  return mutation;
}
function resolveOrientation() {
  const layout = preferences?.layout || "auto";
  const portrait =
    layout === "portrait" || (layout === "auto" && innerHeight > innerWidth);
  const orientation = portrait ? "portrait" : "landscape";
  if (document.documentElement.dataset.orientation !== orientation) document.documentElement.dataset.orientation = orientation;
  text("orientation", portrait ? "PORTRAIT" : "LANDSCAPE");
}
function applyScanlineSpacing(value) {
  document.documentElement.style.setProperty("--scanline-spacing", value + "px");
  document.documentElement.style.setProperty("--scanline-start", (value - 2) + "px");
  text("scanline-spacing-value",value + " px");
}
function resolveContentLayout() {
  const main = $("main"), unit = parseFloat(getComputedStyle(document.documentElement).fontSize);
  const compactNav = String(innerWidth / unit < 42);
  if (document.documentElement.dataset.compactNav !== compactNav) document.documentElement.dataset.compactNav = compactNav;
  const width = (main.clientWidth - 2 * parseFloat(getComputedStyle(main).paddingLeft)) / unit;
  if (main.dataset.narrow !== String(width < 62)) main.dataset.narrow = String(width < 62);
  if (main.dataset.small !== String(width < 34)) main.dataset.small = String(width < 34);
  if (main.dataset.shipStack !== String(width < 44)) main.dataset.shipStack = String(width < 44);
  if (main.dataset.tightHeight !== String(main.clientHeight / unit < 34)) main.dataset.tightHeight = String(main.clientHeight / unit < 34);
  const columns = String(Math.max(2,Math.min(5,Math.floor(innerWidth / unit / 9))));
  if (document.documentElement.style.getPropertyValue("--nav-columns") !== columns) document.documentElement.style.setProperty("--nav-columns",columns);
  updateControlColumns();
}
function applyUIScale(value) {
  const scale = [100,125,150,175,200].includes(value) ? value : 100;
  const size = (16 * scale / 100) + "px";
  if (document.documentElement.style.fontSize !== size) document.documentElement.style.fontSize = size;
  $("ui-scale").value = String(scale);
}
function applyPreferences(p) {
  preferences = p;
  applyUIScale(p.uiScale || 100);
  $("health-display").value = p.healthDisplay || "ship";
  document.documentElement.dataset.theme = p.theme;
  document.body.classList.toggle("scanlines", p.scanlines);
  $("theme").value = p.theme;
  $("layout").value = p.layout;
  $("control-columns").value = String(p.controlColumns || 0);
  $("scanlines").checked = p.scanlines;
  $("auto-ship-controls").checked = Boolean(p.autoAddShipControls);
  if (document.activeElement !== $("scanline-spacing")) $("scanline-spacing").value = String(p.scanlineSpacing || 12);
  applyScanlineSpacing(Number($("scanline-spacing").value));
  $("fullscreen-setting").checked = p.fullscreen;
  $("fullscreen").setAttribute("aria-pressed", String(p.fullscreen));
  resolveOrientation();
}
function setPage(page) {
  act(() => api().SelectPage(page));
}
function cyclePage(delta) {
  const pages = state.Pages;
  const position = pages.findIndex((p) => p.ID === state.Page);
  setPage(pages[(position + delta + pages.length) % pages.length].ID);
}
function renderNavigation(pages) {
  const signature = JSON.stringify(pages);
  if (signature === navigationSignature) return;
  navigationSignature = signature;
  $("navigation").replaceChildren();
  for (const [position, page] of pages.entries()) {
    const button = document.createElement("button");
    button.dataset.page = String(page.ID);
    const number = document.createElement("span");
    number.className = "navigation-index";
    number.textContent = String(position + 1).padStart(2, "0");
    const label = document.createElement("span");
    label.className = "navigation-label";
    label.textContent = page.Name;
    button.append(number, label);
    button.title = page.Shortcut
      ? `${page.Name} (${page.Shortcut})`
      : page.Name;
    button.addEventListener("click", () => setPage(page.ID));
    $("navigation").append(button);
  }
}
function renderEngineering(data) {
  renderMaterialNote(data);
  if (document.activeElement !== $("material-search"))
    $("material-search").value = data.Filter || "";
  $("material-view").value = data.Pinned ? "pinned" : "all";
  const rows = data.Rows || [];
  renderMaterialRows(data);
  const selected = rows.find((row) => row.Key === data.Selected);
  $("material-details").hidden = !selected;
  document.querySelector(".engineering-grid").dataset.details = String(Boolean(selected));
  $("pin-material").textContent = selected?.Pinned ? "Unpin material" : "Pin material";
  $("pin-material").setAttribute("aria-pressed", String(Boolean(selected?.Pinned)));
  $("pin-material").disabled = !selected || Boolean(data.PinsError);
  text("material-pins-error", data.PinsError || "");
  $("material-pins-error").hidden = !data.PinsError;
  const changed = selectedMaterial !== data.Selected;
  selectedMaterial = data.Selected || "";
  if (changed) {
    materialTargetDraft = null;
    $("material-details").scrollTop = 0;
  }
  $("material-empty").hidden = rows.length !== 0;
  text("material-name", selected?.Display || "Select a material");
  text("material-category", selected?.Category || "");
  text("material-owned", selected?.Known ? selected.Count : "?");
  text(
    "material-needed",
    selected?.Target && selected.Known
      ? Math.max(0, selected.Target - selected.Count)
      : "—",
  );
  if (!materialTargetDraft && (changed || document.activeElement !== $("material-target")))
    $("material-target").value = selected?.Target || "";
  $("save-target").disabled = !selected;
  $("remove-target").disabled = !selected?.Target;
  text(
    "material-message",
    data.Message || "Press Save target to keep this quantity.",
  );
}
function renderStationSearch(data) {
  $("station-search-tools").hidden = data.Page !== 5 || data.Outfitting.Active;
  if (data.Page !== 5) return;
  const select = $("station-search-kind");
  if (!select.options.length) {
    for (const [index, provider] of data.StationSearch.entries()) {
      const option = document.createElement("option");
      option.value = String(index);
      option.textContent = provider.Name;
      select.append(option);
    }
  }
  const provider = data.StationSearch[Number(select.value)];
  text(
    "station-search-description",
    provider?.Description || "Choose a search category.",
  );
  text(
    "station-search-reference",
    data.System
      ? `Reference system: ${data.System} · enter this in Inara's near-system filter.`
      : "Enter a reference system in Inara to find nearby stations.",
  );
}
function render(data) {
  const oldPage = renderedPage;
  if (state && state.Mode !== data.Mode) {
    materialTargetDraft = null;
    selectedMaterial = "";
  }
  state = data;
  renderedPage = data.Page;
  applyPreferences(data.Preferences);
  renderNavigation(data.Pages);
  renderStationSearch(data);
  renderOutfitting(data);
  renderShipModules(data);
  renderShipHealth(data);
  renderControlLayoutEditor(data);
  renderShipControlPicker(data);
  renderGraphicalPage(data);
  $("main").dataset.page = String(data.Page);
  const page = data.Pages.find((p) => p.ID === data.Page);
  text("page-title", page?.Name || "Cockpit");
  text("eyebrow", data.Page === 3 ? "DISPLAY & PREFERENCES" : "CO-PIT");
  for (const button of $("navigation").children) {
    if (Number(button.dataset.page) === data.Page)
      button.setAttribute("aria-current", "page");
    else button.removeAttribute("aria-current");
  }
  $("connection").dataset.ready = String(data.Ready);
  $("connection").dataset.armed = String(data.Armed);
  $("connection").dataset.paused = String(data.Paused);
  $("connection").dataset.mode = data.Mode;
  text("status", data.Status);
  text("status-detail", data.Detail);
  $("cockpit").hidden = data.Page !== 0;
  $("settings").hidden = data.Page !== 3;
  $("engineering").hidden = data.Page !== 7;
  $("mock").checked = data.Mode === "mock";
  if (data.Page === 5 && data.Outfitting.Active)
    text("page-title", "Station outfitting");
  const error = data.Error || clientError;
  $("error").hidden = !error;
  if (error) text("error", error);
  text(
    "mode",
    data.Mode === "mock"
      ? "SIMULATOR · MOCK DATA"
      : data.Armed
        ? "ARMED"
        : data.Paused
          ? "PAUSED · AUTO-RESUME"
          : "DISARMED",
  );
  if (data.Page === 0) {
    text("commander", data.Commander || "Waiting for commander");
    text("ship", data.Ship || "Unreported ship");
    text("context", data.Context || "Searching for Elite Dangerous");
    text("system", data.System || "NO SYSTEM REPORTED");
    text("fuel", data.Fuel);
    text("pips", data.Pips);
    text("warning", data.Warning || "No flight warnings reported");
    text("action", data.Action || "Ready");
    renderControls(data.Controls || [], data.ControlLayout);
    text(
      "control-hint",
      data.Mode === "mock"
        ? "Simulator only. Buttons change sample data."
        : data.Armed || data.Paused
          ? "Clicking a control returns focus to Elite. The glass changes only when game telemetry confirms."
          : "Focus Elite and press Ctrl+Alt+F9 to arm once. STOP cancels arming and auto-resume.",
    );
  }
  if (data.Page === 7) renderEngineering(data.Engineering);
  resolveContentLayout();
  if (oldPage !== data.Page) {
    $("main").scrollTop = 0;
  }
}
async function refresh() {
  if (polling || stopping || !api()) return;
  polling = true;
  try {
    render(await api().GetState(Math.floor(($("main").clientWidth - 80) / 8)));
  } finally {
    polling = false;
  }
}
async function poll() {
  try {
    await refresh();
  } catch (error) {
    message(error);
  }
  if (!stopping) setTimeout(poll, 150);
}
function savePreference(key, value) {
  act(() => api().SavePreferences({ ...preferences, [key]: value }));
}
function toggleFullscreen() {
  act(() => api().SetFullscreen(!preferences.fullscreen));
}
$("fullscreen").addEventListener("click", toggleFullscreen);
$("fullscreen-setting").addEventListener("change", (event) => {
  const enabled = event.target.checked;
  act(() => api().SetFullscreen(enabled));
});
$("minimise").addEventListener("click", () => act(() => api().Minimise()));
$("close").addEventListener("click", () =>
  act(async () => {
    stopping = true;
    await api().Close();
  }),
);
$("settings-shortcut").addEventListener("click", () => setPage(3));
$("disarm").addEventListener("click", () => act(() => api().Disarm()));
$("portrait-window").addEventListener("click", () =>
  act(() => api().PortraitWindow()),
);
$("layout").addEventListener("change", (event) =>
  savePreference("layout", event.target.value),
);
$("control-columns").addEventListener("change", (event) =>
  savePreference("controlColumns", Number(event.target.value)),
);
$("theme").addEventListener("change", (event) =>
  savePreference("theme", event.target.value),
);
$("scanlines").addEventListener("change", (event) =>
  savePreference("scanlines", event.target.checked),
);
$("auto-ship-controls").addEventListener("change",event => savePreference("autoAddShipControls",event.target.checked));
$("scanline-spacing").addEventListener("input",event => applyScanlineSpacing(Number(event.target.value)));
$("scanline-spacing").addEventListener("change",event => savePreference("scanlineSpacing",Number(event.target.value)));
$("mock").addEventListener("change", (event) => {
  const enabled = event.target.checked;
  event.target.disabled = true;
  act(async () => {
    try {
      await api().SetMock(enabled);
    } finally {
      $("mock").disabled = false;
    }
  });
});
$("station-search-open").addEventListener("click", () => {
  const index = Number($("station-search-kind").value);
  act(() => api().OpenStationSearch(index));
});
$("station-search-kind").addEventListener("change", () => {
  if (state) renderStationSearch(state);
});
function filterMaterials() {
  const query = $("material-search").value;
  const pinned = $("material-view").value === "pinned";
  act(() => api().FilterMaterials(query, pinned));
}
$("material-search").addEventListener("input", () => {
  clearTimeout(filterTimer);
  filterTimer = setTimeout(filterMaterials, 120);
});
$("material-view").addEventListener("change", () => {
  clearTimeout(filterTimer);
  filterMaterials();
});
// An unsaved edit belongs to the selected material, regardless of field focus.
$("material-target").addEventListener("input", () => {
  materialTargetDraft = { key: selectedMaterial };
});
function saveMaterialTarget(target) {
  const key = selectedMaterial;
  const submittedDraft = materialTargetDraft;
  act(async () => {
    await api().SaveMaterialTarget(key, target);
    // A failed save or an edit made while saving must retain its draft.
    if (materialTargetDraft === submittedDraft) materialTargetDraft = null;
  });
}
$("target-form").addEventListener("submit", (event) => {
  event.preventDefault();
  saveMaterialTarget(Number($("material-target").value));
});
$("remove-target").addEventListener("click", () => saveMaterialTarget(0));
$("material-list").addEventListener("keydown", (event) => {
  const rows = state?.Engineering.Rows || [];
  if (!rows.length || event.target.closest(".material-pin")) return;
  const focusedKey = event.target.closest(".material-row")?.dataset.key;
  const position = rows.findIndex((row) => row.Key === (focusedKey || selectedMaterial));
  if (["ArrowUp", "ArrowDown"].includes(event.key)) {
    event.preventDefault();
    const index = Math.max(
      0,
      Math.min(
        rows.length - 1,
        position + (event.key === "ArrowDown" ? 1 : -1),
      ),
    );
    act(() => api().SelectMaterial(rows[index].Key));
  }
  if (event.key === "Enter") {
    event.preventDefault();
    const key = rows[Math.max(0, position)].Key;
    act(() => api().SelectMaterial(key)).then(() => {
      if (!$("material-details").hidden) {
        $("material-target").focus();
        $("material-target").select();
      }
    });
  }
});
window.addEventListener("resize", resolveOrientation);
window.addEventListener("beforeunload", () => {
  stopping = true;
  clearTimeout(filterTimer);
});
window.addEventListener("keydown", (event) => {
  if (!state || event.repeat) return;
  if ($("ship-control-picker").open) return;
  if (event.key === "F11") {
    event.preventDefault();
    toggleFullscreen();
    return;
  }
  if (event.key === "Escape" && preferences.fullscreen) {
    event.preventDefault();
    act(() => api().SetFullscreen(false));
    return;
  }
  if (event.ctrlKey && event.key === "Tab") {
    event.preventDefault();
    cyclePage(event.shiftKey ? -1 : 1);
    return;
  }
  if (
    event.ctrlKey &&
    event.key.toLowerCase() === "r" &&
    state.Outfitting.Active &&
    state.Page === 5
  ) {
    event.preventDefault();
    refreshOutfitting();
    return;
  }
  if (event.ctrlKey || event.altKey || event.metaKey) return;
  const shortcuts = { F1: 2, F5: 3, F6: 4, F7: 5, F8: 6, F9: 7, F10: 8 };
  if (shortcuts[event.key] !== undefined) {
    event.preventDefault();
    setPage(shortcuts[event.key]);
    return;
  }
  if (event.key === "F4") {
    event.preventDefault();
    act(() => api().SetMock(state.Mode !== "mock"));
    return;
  }
  if (event.target.matches("input,select,textarea")) return;
  if (
    state.Page === 5 &&
    !state.Outfitting.Active &&
    event.key.toLowerCase() === "o"
  ) {
    event.preventDefault();
    act(() => api().ShowOutfitting(true));
    return;
  }
  if (state.Page === 5 && state.Outfitting.Active && event.key === "Escape") {
    event.preventDefault();
    act(() => api().ShowOutfitting(false));
    return;
  }
  if (
    state.Page === 5 &&
    !state.Outfitting.Active &&
    event.key.toLowerCase() === "s"
  ) {
    event.preventDefault();
    $("station-search-kind").focus();
    return;
  }
  if (
    ["NumpadAdd", "NumpadSubtract"].includes(event.code) &&
    (state.Mode === "mock" ||
      (state.PageKeys || "").startsWith("Global page keys unavailable"))
  ) {
    event.preventDefault();
    cyclePage(event.code === "NumpadAdd" ? 1 : -1);
    return;
  }
  if (
    state.Mode === "mock" &&
    state.Page === 0 &&
    /^Numpad[0-9]$/.test(event.code)
  ) {
    event.preventDefault();
    const n = Number(event.code.slice(-1));
    const control = state.Controls[n === 0 ? 9 : n - 1];
    act(() => api().PressControl(control.ID));
    return;
  }

});
setInterval(
  () => text("clock", new Date().toISOString().slice(11, 19) + " UTC"),
  1000,
);
resolveOrientation();
poll();

$("health-display").addEventListener("change",event => savePreference("healthDisplay",event.target.value));
$("cockpit-open-navigation").addEventListener("click",()=>setPage(5));

$("ui-scale").addEventListener("change",event => { const scale=Number(event.target.value);applyUIScale(scale);resolveContentLayout();savePreference("uiScale",scale); });
$("reset-ui-scale").addEventListener("click",()=>{applyUIScale(100);resolveContentLayout();savePreference("uiScale",100);});
window.addEventListener("resize",resolveContentLayout);
