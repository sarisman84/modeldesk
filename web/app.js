"use strict";
const $ = (s) => document.querySelector(s),
  values = Object.values;
let state,
  tab = "runtime",
  revision = 0,
  cfg,
  polling = false;
function node(tag, text, cls) {
  const n = document.createElement(tag);
  if (text != null) n.textContent = text;
  if (cls) n.className = cls;
  return n;
}
function notice(s) {
  $("#notice").textContent = s;
}
async function api(action, data = {}, signal) {
  const r = await fetch("/api/" + action, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
    signal,
  });
  if (!r.ok) throw Error((await r.text()).trim());
  return r.json();
}
function button(text, fn) {
  const b = node("button", text, "quiet");
  b.type = "button";
  b.onclick = async () => {
    b.disabled = true;
    try {
      await fn();
    } catch (e) {
      notice(e.message);
    } finally {
      b.disabled = false;
    }
  };
  return b;
}
function size(n) {
  return n >= 2 ** 30
    ? (n / 2 ** 30).toFixed(2) + " GiB"
    : (n / 2 ** 20).toFixed(1) + " MiB";
}
function card(parent, title, sub) {
  const a = node("article");
  a.append(node("h3", title), node("p", sub, "meta"));
  parent.append(a);
  return a;
}
function actions(a, ...buttons) {
  const row = node("div", null, "actions");
  row.append(...buttons);
  a.append(row);
}
function empty(parent, text) {
  if (!parent.children.length) parent.append(node("p", text, "empty"));
}
function show(name) {
  tab = name;
  for (const s of document.querySelectorAll("main section"))
    s.hidden = s.id !== name;
  for (const b of document.querySelectorAll("[data-tab]"))
    b.setAttribute("aria-current", String(b.dataset.tab === name));
  if (name === "runtime" && state) poll();
}
for (const b of document.querySelectorAll("[data-tab]"))
  b.onclick = () => show(b.dataset.tab);
async function refresh() {
  state = await api("library");
  revision++;
  const { Files, Profiles, Servers } = state.library;
  const targets = $("#servers"),
    weights = $("#weights"),
    presets = $("#presets");
  for (const n of [targets, weights, presets]) n.replaceChildren();
  const field = $("#profile").elements.File,
    previous = field.value;
  field.replaceChildren();
  for (const f of values(Files)) {
    const name = f.Path.split(/[\\/]/).pop();
    field.add(new Option(name, f.ID));
    const a = card(weights, name, `${size(f.Size)} · ${f.Source} · ${f.Path}`);
    const refs = values(Profiles).filter((p) => p.File === f.ID);
    if (refs.length)
      a.append(node("p", "Used by " + refs.map((p) => p.Name).join(", ")));
    else
      actions(
        a,
        button("Forget", async () => {
          await api("removeFile", { ID: f.ID });
          await refresh();
        }),
        button("Delete from disk", async () => {
          if (
            confirm(
              `Permanently delete ${size(f.Size)} from disk?\n\n${f.Path}\n\nOther tools may still use this file.`,
            )
          ) {
            await api("removeFile", { ID: f.ID, Path: f.Path, Delete: true });
            await refresh();
          }
        }),
      );
  }
  if (Files[previous]) field.value = previous;
  function target(v, local) {
    const a = card(
      targets,
      v.Name,
      (v.Engine === "swap" ? "llama-swap" : "llama.cpp") + " · " + v.URL,
    );
    const box = node("div", "Checking…", "status");
    box.dataset.status = v.ID;
    a.append(box);
    if (!local) {
      const controls = [
        button("Disconnect", async () => {
          await api("disconnect", { ID: v.ID });
          await refresh();
        }),
      ];
      if (v.Config)
        controls.unshift(button("Edit configuration", () => openConfig(v)));
      actions(a, ...controls);
    }
  }
  for (const s of values(Servers)) target(s, false);
  for (const p of values(Profiles)) {
    const live = !!state.running[p.ID],
      a = card(
        presets,
        p.Name,
        `${p.Engine === "swap" ? "llama-swap" : "llama.cpp"} · port ${p.Port} · ${Files[p.File]?.Path}`,
      );
    if (state.exits[p.ID]) a.append(node("p", state.exits[p.ID]));
    actions(
      a,
      button(live ? "Stop" : "Launch", async () => {
        await api(live ? "stop" : "launch", { ID: p.ID });
        await refresh();
        if (!live) show("runtime");
      }),
      button("Edit", () => {
        const form = $("#profile");
        for (const [k, v] of Object.entries(p))
          if (form.elements[k])
            form.elements[k].value = k === "Args" ? (v || []).join("\n") : v;
        $("#profileEditor").open = true;
        form.elements.Name.focus();
      }),
      button("Remove profile", async () => {
        if (confirm("Remove " + p.Name + "? Model files will be kept.")) {
          await api("removeProfile", { ID: p.ID });
          await refresh();
        }
      }),
    );
    if (live) target({ ...p, URL: "http://127.0.0.1:" + p.Port }, true);
  }
  empty(
    targets,
    "No servers connected. Connect an existing endpoint below, or launch a profile.",
  );
  empty(
    weights,
    "No files in the library yet. Import a GGUF or choose a download below.",
  );
  empty(
    presets,
    "No profiles yet. Import model files first, then create a profile.",
  );
  if (tab === "runtime") poll();
}
async function poll() {
  if (polling || document.hidden || tab !== "runtime") return;
  polling = true;
  const rev = revision;
  try {
    await Promise.all(
      [...document.querySelectorAll("[data-status]")].map(async (b) => {
        try {
          const h = await api("status", { ID: b.dataset.status });
          if (rev !== revision) return;
          b.replaceChildren(
            node(
              "strong",
              h.State + " · " + new Date(h.Checked).toLocaleTimeString(),
            ),
          );
          if (h.Detail) b.append(node("p", h.Detail));
          for (const [name, state] of Object.entries(h.Models))
            b.append(node("p", `${name} — ${state}`));
          const metrics = Object.entries(h.Metrics);
          if (!metrics.length)
            b.append(node("p", "Performance metrics unavailable", "meta"));
          else
            for (const [k, v] of metrics)
              b.append(node("p", `${k}: ${v}`, "metric"));
        } catch (e) {
          b.textContent = e.message;
        }
      }),
    );
    const fresh = await api("library");
    if (
      rev === revision &&
      JSON.stringify(fresh.running) !== JSON.stringify(state.running)
    )
      await refresh();
  } catch (e) {
    notice(e.message);
  } finally {
    polling = false;
  }
}
function form(id, fn) {
  $(id).onsubmit = async (e) => {
    e.preventDefault();
    const f = e.currentTarget,
      b = f.querySelector("button");
    b.disabled = true;
    try {
      await fn(Object.fromEntries(new FormData(f)), f);
    } catch (e) {
      notice(e.message);
    } finally {
      b.disabled = false;
    }
  };
}
form("#import", async (d, f) => {
  await api("import", d);
  f.reset();
  await refresh();
  notice("Model file registered.");
});
form("#server", async (d, f) => {
  await api("server", { Server: d });
  f.reset();
  await refresh();
  notice("Server connected.");
});
form("#profile", async (d, f) => {
  for (const k of ["Port", "Context", "GPU", "Threads"]) d[k] = Number(d[k]);
  d.Args = d.Args.split("\n").filter(Boolean);
  await api("profile", { Profile: d });
  f.reset();
  await refresh();
  notice("Profile saved.");
});
form("#catalog", async (d) => {
  const box = $("#choices");
  box.replaceChildren();
  notice("Reading repository…");
  const c = await api("catalog", d);
  for (const f of c.Files || []) {
    const a = card(box, f.rfilename, size(f.size));
    actions(
      a,
      button("Download", async () => {
        const ctl = new AbortController();
        const cancel = button("Cancel", () => ctl.abort());
        a.append(cancel);
        notice("Downloading " + f.rfilename + "… Keep this page open.");
        try {
          await api(
            "download",
            { Repo: c.Repo, Revision: c.Revision, Filename: f.rfilename },
            ctl.signal,
          );
          await refresh();
          notice("Downloaded " + f.rfilename);
        } finally {
          cancel.remove();
        }
      }),
    );
  }
  notice(
    c.Files?.length
      ? `${c.Files.length} GGUF files available.`
      : "No GGUF files found.",
  );
});
async function openConfig(s) {
  cfg = { ...(await api("config", { ID: s.ID })), id: s.ID };
  $("#configPath").textContent = s.Config;
  $("#original").value = $("#proposed").value = cfg.text;
  $("#apply").hidden = $("#preview").hidden = true;
  $("#configMessage").textContent = "";
  $("#editor").showModal();
}
$("#proposed").oninput = () => {
  $("#preview").hidden = $("#apply").hidden = true;
};
$("#review").onclick = () => {
  const before = $("#original").value.split("\n"),
    after = $("#proposed").value.split("\n");
  let lo = 0;
  while (lo < before.length && lo < after.length && before[lo] === after[lo])
    lo++;
  let x = before.length,
    y = after.length;
  while (x > lo && y > lo && before[x - 1] === after[y - 1]) {
    x--;
    y--;
  }
  const changed = x !== lo || y !== lo;
  $("#preview").textContent = changed
    ? [
        ...before.slice(lo, x).map((t) => "- " + t),
        ...after.slice(lo, y).map((t) => "+ " + t),
      ].join("\n")
    : "No changes";
  $("#preview").hidden = false;
  $("#apply").hidden = !changed;
};
$("#close").onclick = () => $("#editor").close();
$("#apply").onclick = async () => {
  try {
    let text = $("#proposed").value;
    if (cfg.text.includes("\r\n")) text = text.replaceAll("\n", "\r\n");
    const backup = await api("apply", {
      ID: cfg.id,
      Hash: cfg.hash,
      Text: text,
    });
    $("#editor").close();
    notice(
      "Saved. Backup: " +
        backup +
        ". Restart your external runtime unless config watching is enabled.",
    );
  } catch (e) {
    $("#configMessage").textContent = e.message;
  }
};
show("runtime");
refresh().catch((e) => notice(e.message));
setInterval(poll, 5000);
