"use strict";
let graphicalPageSignature = "";
function pageText(tag, value, className = "") {
  const node = document.createElement(tag);
  node.textContent = String(value ?? "");
  if (className) node.className = className;
  return node;
}
function graphicalPanel(data) {
  const panel = document.createElement(data.Collapsible ? "details" : "article");
  panel.className = "panel graphical-panel";
  panel.append(pageText(data.Collapsible ? "summary" : "h2", data.Title));
  if (data.Hint) panel.append(pageText("p",data.Hint,"muted graphical-hint"));
  if (data.Fields?.length) {
    const list = document.createElement("dl");
    list.className = "graphical-fields";
    for (const field of data.Fields) {
      const row = document.createElement("div");
      row.append(pageText("dt",field.Label),pageText("dd",field.Value));
      list.append(row);
    }
    panel.append(list);
  }
  if (data.Lines?.length) {
    const list = document.createElement("ul");
    list.className = "graphical-notes";
    data.Lines.forEach(line => list.append(pageText("li",line)));
    panel.append(list);
  }
  if (data.Columns?.length) {
    const wrap = document.createElement("div");
    wrap.className = "graphical-table-wrap";
    const table = document.createElement("table");
    table.className = "graphical-table";
    const head = document.createElement("thead"), tr = document.createElement("tr");
    data.Columns.forEach(column => {const th=pageText("th",column);th.scope="col";tr.append(th);});
    head.append(tr);table.append(head);
    const body = document.createElement("tbody");
    for (const row of data.Rows || []) {
      const line = document.createElement("tr");
      row.forEach(value => line.append(pageText("td",value)));
      body.append(line);
    }
    table.append(body);wrap.append(table);panel.append(wrap);
  }
  return panel;
}
function hardpointCard(row) {
  const panel = document.createElement("article");
  panel.className = "panel graphical-hardpoint";
  panel.dataset.module = row.Key;
  panel.append(pageText("h3",row.Name),pageText("p",row.Slot,"muted"));
  const values=document.createElement("dl");
  values.className="graphical-fields";
  for(const [label,value] of [["Power",row.Power],["Integrity",row.Health],["Priority",row.Priority],["Ammo snapshot",row.Ammo]]) {
    const item=document.createElement("div");item.append(pageText("dt",label),pageText("dd",value));values.append(item);
  }
  panel.append(values);
  const selectors=document.createElement("div");
  selectors.className="firegroup-selectors";
  for(let group=0;group<8;group++) {
    const label=document.createElement("label");
    label.append(document.createTextNode("Group "+String.fromCharCode(65+group)));
    const select=document.createElement("select");
    select.setAttribute("aria-label",row.Name+", group "+String.fromCharCode(65+group));
    select.dataset.group=String(group);
    for(const [value,name] of [[-1,"Unknown"],[0,"None"],[1,"Primary"],[2,"Secondary"]]) {
      const option=pageText("option",name);option.value=String(value);select.append(option);
    }
    select.value=String(row.Groups?.[group] ?? -1);
    select.addEventListener("change",()=>{
      const value=Number(select.value);select.disabled=true;
      act(()=>api().SetFireGroupReference(row.Key,group,value)).then(()=>{
        const current=state?.PageView?.Hardpoints?.find(module=>module.Key===row.Key);
        if(select.isConnected) {select.value=String(current?.Groups?.[group] ?? -1);select.disabled=false;}
      });
    });
    label.append(select);selectors.append(label);
  }
  panel.append(selectors);
  return panel;
}
function renderGraphicalPage(data) {
  const container=document.getElementById("graphical-page");
  const visible=[1,2,4,5,6,8].includes(data.Page)&&!(data.Page===5&&data.Outfitting.Active);
  container.hidden=!visible;
  if(!visible) return;
  const view=data.PageView || {Panels:[],Hardpoints:[]};
  text("graphical-page-source",view.Source || "Waiting for reported data");
  text("graphical-page-message",data.Page === 8 ? data.Editor || "" : "");
  document.getElementById("graphical-page-message").hidden = data.Page !== 8 || !data.Editor;
  const signature=JSON.stringify([data.Page,view.Panels,view.Hardpoints]);
  if(signature===graphicalPageSignature) return;
  graphicalPageSignature=signature;
  const panels=document.getElementById("graphical-page-panels");
  panels.replaceChildren();
  for(const panel of view.Panels || []) panels.append(graphicalPanel(panel));
  if(data.Page===8) {
    const grid=document.createElement("div");grid.className="hardpoint-card-grid";
    for(const row of view.Hardpoints || []) grid.append(hardpointCard(row));
    if(!grid.children.length) grid.append(pageText("p","Waiting for reported hardpoints or utility modules.","muted"));
    panels.append(grid);
  }
  if(data.Page===2) {
    const links=document.createElement("div");links.className="help-page-actions";
    for(const [label,page] of [["Settings",3],["Your cockpit",0],["Station navigation",5]]) {
      const button=pageText("button",label);button.type="button";button.addEventListener("click",()=>setPage(page));links.append(button);
    }
    panels.append(links);
  }
}
