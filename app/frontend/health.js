"use strict";
function renderShipHealth(data) {
  const health = data.Health || {};
  const percent = value => typeof value === "number" && Number.isFinite(value) && value >= 0 && value <= 100 ? value : null;
  const hull = percent(health.HullPercent), shield = percent(health.ShieldPercent);
  const offline = health.Offline === true && health.ShieldState === "OFFLINE" && shield === 0;
  document.body.dataset.shieldOffline = String(offline);
  $("shield-offline-banner").hidden = !offline;
  const display = data.Preferences?.healthDisplay === "bars" ? "bars" : "ship";
  $("ship-health").dataset.display = display;
  for (const [name,value] of [["hull",hull],["shield",shield]]) {
    const label = value === null ? name === "shield" && health.ShieldState === "UP" ? "UP" : "Unknown" : Math.round(value) + "%";
    document.querySelectorAll('[data-health-value="'+name+'"]').forEach(node => node.textContent = label);
    document.querySelectorAll('[data-health-meter="'+name+'"]').forEach(node => {
      node.style.setProperty("--health-level",(value ?? 0)+"%");
      node.dataset.unknown = String(value === null);
      node.setAttribute("aria-label",name + ": " + label);
    });
  }
  text("health-source",health.Source || "Waiting for ship health telemetry");
  const route = data.CockpitRoute || {};
  text("cockpit-destination",route.Available ? route.Destination : "No plotted destination reported");
  for (const name of ["Next","Remaining","Distance"]) text("cockpit-route-"+name.toLowerCase(),route[name] || "Not reported");
  text("cockpit-route-source",route.Source || "Route data unavailable");
}
