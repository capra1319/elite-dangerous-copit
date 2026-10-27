"use strict";
const materialNoteDrafts = new Map();
let renderedMaterialNote = "";
function materialNoteIdentity(key, simulated) {
  return (simulated ? "mock:" : "real:") + key;
}
function renderMaterialNote(data) {
  const simulated = state?.Mode === "mock";
  const identity = materialNoteIdentity(data.Selected, simulated);
  const draft = materialNoteDrafts.get(identity);
  const field = document.getElementById("material-note");
  const selected = (data.Rows || []).some(row => row.Key === data.Selected);
  if (renderedMaterialNote !== identity || document.activeElement !== field)
    field.value = draft?.value ?? data.Note ?? "";
  renderedMaterialNote = identity;
  field.disabled = !selected || Boolean(data.NotesError);
  document.getElementById("save-material-note").disabled = field.disabled;
  document.getElementById("clear-material-note").disabled = field.disabled || (!field.value && !data.Note);
  text("material-note-error", data.NotesError || "");
  document.getElementById("material-note-error").hidden = !data.NotesError;
  text("material-note-status", draft ? "Unsaved comment" : data.Note ? "Saved comment" : "No comment saved");
  text("material-note-hint", simulated
    ? "Simulator comments stay in memory and are kept separate from your real comments."
    : "Your own reference · saved locally for this material across ships and restarts. Up to 2,000 characters.");
}
document.getElementById("material-note").addEventListener("input", event => {
  if (!selectedMaterial) return;
  const identity = materialNoteIdentity(selectedMaterial, state?.Mode === "mock");
  materialNoteDrafts.set(identity, { value: event.target.value });
  text("material-note-status", "Unsaved comment");
  document.getElementById("clear-material-note").disabled = !event.target.value && !state?.Engineering.Note;
});
function saveMaterialNote(clear = false) {
  const key = selectedMaterial;
  if (!key) return;
  const simulated = state?.Mode === "mock";
  const identity = materialNoteIdentity(key, simulated);
  const draft = materialNoteDrafts.get(identity);
  const note = clear ? "" : document.getElementById("material-note").value;
  act(async () => {
    await api().SaveMaterialNote(key, note, simulated);
    // Keep a newer edit if the user continued typing while this save was pending.
    if (materialNoteDrafts.get(identity) === draft) materialNoteDrafts.delete(identity);
  });
}
document.getElementById("save-material-note").addEventListener("click", () => saveMaterialNote());
document.getElementById("clear-material-note").addEventListener("click", () => saveMaterialNote(true));
