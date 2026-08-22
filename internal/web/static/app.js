"use strict";
(() => {
  const $ = q => document.querySelector(q);
  const esc = s => String(s ?? "").replace(/[&<>"']/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
  const NUM = new Intl.NumberFormat("en-US");
  const fmt = n => NUM.format(n);
  const css = n => getComputedStyle(document.documentElement).getPropertyValue(n).trim();
  const pretty = s => String(s).toLowerCase().split(" ")
    .map(w => /^(n|s|e|w|ne|nw|se|sw)$/.test(w) ? w.toUpperCase() : /^\d/.test(w) ? w : w.charAt(0).toUpperCase() + w.slice(1)).join(" ");
  const getJSON = async u => {
    const r = await fetch(u);
    if (!r.ok) throw new Error(r.status + " " + u.split("?")[0]);
    return r.json();
  };
  const LABELS = { prop409_major: "Prop 409 major streets", prop409_residential: "Prop 409 residential street areas" };
  const darkMQ = matchMedia("(prefers-color-scheme: dark)");

  // The city's per-year permit counts roughly double in 2005 (about 940 in 2004, 2,394 in
  // 2005, 3,595 in 2006), consistent with its note that older records are not fully
  // digitized. Starting at 2005 keeps early years from looking quiet when they were not.
  const SPARSE_BEFORE = 2005;
  const st = { from: 0, to: 0, cats: new Set(), nowork: false, sel: null, meta: null, seq: 0, detailSeq: 0, overlays: {}, shown: 40,
    focus: null, focusSeq: 0, searchSeq: 0, focusShown: 15 };

  // ---- map ------------------------------------------------------------
  const map = L.map("map", { minZoom: 9, maxZoom: 19 }).setView([32.2226, -110.9747], 11);
  map.attributionControl.setPrefix('<a href="https://leafletjs.com">Leaflet</a>');
  map.createPane("lines").style.zIndex = 410;
  map.createPane("dots").style.zIndex = 420;
  const linesR = L.canvas({ pane: "lines" });
  const dotsR = L.canvas({ pane: "dots", tolerance: 5 });
  const dots = L.layerGroup().addTo(map);
  let ring = null, tiles = null;

  // The server decides which basemap to use (see /api/config): providers change their
  // terms, and a hardcoded one stops working without warning.
  let basemap = null;
  const FILTERS = ["mute", "invert"]; // visual treatments the stylesheet knows
  function setTiles() {
    if (!basemap) return;
    const src = darkMQ.matches ? basemap.dark : basemap.light;
    if (tiles) map.removeLayer(tiles);
    tiles = L.tileLayer(src.url, { maxZoom: src.max_zoom || 19, attribution: src.attribution }).addTo(map);
    const box = map.getContainer();
    FILTERS.forEach(f => box.classList.remove("tiles-" + f));
    if (FILTERS.includes(src.filter)) box.classList.add("tiles-" + src.filter);
  }

  // A hundred-block is about 100 ft long. Sizing each dot from that real length
  // (with a floor so it stays visible) keeps a whole city from turning into one
  // solid blob when zoomed out, and makes dots grow as the blocks do on screen.
  function radius(count, zoom) {
    const metersPerPx = 156543.03 * Math.cos(map.getCenter().lat * Math.PI / 180) / Math.pow(2, zoom);
    const base = Math.min(11, Math.max(1.1, (30.5 / metersPerPx) * 0.42));
    return Math.min(18, base * (1 + 0.3 * Math.log(Math.max(1, count))));
  }
  const bucket = n => (n < 4 ? 0 : n < 7 ? 1 : n < 10 ? 2 : 3);

  // ---- data ----------------------------------------------------------
  const qs = () => new URLSearchParams({ from: st.from, to: st.to, cats: [...st.cats].join(","), nowork: st.nowork ? 1 : 0 }).toString();
  const setStatus = t => { $("#status").textContent = t; };

  async function load() {
    const my = ++st.seq;
    const b = map.getBounds().pad(0.05);
    const bbox = [b.getSouth(), b.getWest(), b.getNorth(), b.getEast()].map(x => x.toFixed(5)).join(",");
    let resp;
    try {
      resp = await fetch("/api/blocks?" + qs() + "&bbox=" + bbox + "&limit=8000");
      if (!resp.ok) throw new Error(resp.status + " /api/blocks");
    } catch (e) {
      setStatus("Could not load blocks: " + e.message);
      return;
    }
    const bs = await resp.json();
    if (my !== st.seq) return; // a newer request superseded this one
    const inView = +resp.headers.get("X-Total-Blocks") || bs.length;
    drawBlocks(bs);
    drawTop(bs);
    const permits = bs.reduce((p, x) => p + x.count, 0);
    setStatus(inView > bs.length
      ? `Showing the ${fmt(bs.length)} busiest of ${fmt(inView)} blocks in view. Zoom in to see the rest.`
      : `${fmt(permits)} permits on ${fmt(bs.length)} blocks in view`);
  }

  function drawBlocks(bs) {
    dots.clearLayers();
    const z = map.getZoom();
    const ramp = [css("--c1"), css("--c2"), css("--c3"), css("--c4")], edge = css("--paper");
    for (let i = bs.length - 1; i >= 0; i--) { // busiest last, so they sit on top
      const b = bs[i], r = radius(b.count, z);
      const m = L.circleMarker([b.lat, b.lng], {
        renderer: dotsR, pane: "dots", radius: r, fillColor: ramp[bucket(b.count)], fillOpacity: 0.82,
        color: edge, weight: z >= 15 ? 1 : 0, opacity: 0.9,
      });
      m.bindTooltip(`${esc(b.block)} block of ${esc(pretty(b.street))}: ${fmt(b.count)}`, { direction: "top", offset: [0, -r] });
      m.on("click", () => select(b));
      dots.addLayer(m);
    }
    placeRing();
  }

  function placeRing() {
    if (ring) { map.removeLayer(ring); ring = null; }
    if (!st.sel) return;
    ring = L.circleMarker([st.sel.lat, st.sel.lng], {
      renderer: dotsR, pane: "dots", interactive: false, fill: false,
      radius: radius(st.sel.count || 1, map.getZoom()) + 4, color: css("--ink"), weight: 2,
    }).addTo(map);
  }

  function drawTop(bs) {
    $("#top").innerHTML = bs.slice(0, 8).map(b =>
      `<li><button data-k="${esc(b.key)}" data-lat="${esc(b.lat)}" data-lng="${esc(b.lng)}" data-n="${esc(b.count)}">` +
      `<span class="num">${fmt(b.count)}</span><span class="name">${esc(b.block)} block of ${esc(pretty(b.street))}</span></button></li>`).join("")
      || '<li class="mut">Nothing in view with these filters.</li>';
  }

  // ---- search: an address, a street, or two cross streets -------------------
  // The server searches its own permit data (no outside geocoder), so it only knows
  // streets with recorded permits and places an address at the nearest recorded block.
  const focusLayer = L.layerGroup().addTo(map);
  const feet = m => {
    const ft = m * 3.28084;
    return ft < 1000 ? fmt(Math.max(10, Math.round(ft / 10) * 10)) + " ft" : (ft / 5280).toFixed(ft < 5280 * 10 ? 1 : 0) + " mi";
  };
  const radiusLabel = () => $("#radius").selectedOptions[0].textContent;

  function placeLabel(p) {
    if (p.kind === "street") return pretty(p.street);
    if (p.kind === "intersection") return pretty(p.street) + " & " + pretty(p.cross);
    return p.block + " block of " + pretty(p.street);
  }
  function placeNote(p) {
    if (p.kind === "street") return "Street · " + fmt(p.permits) + " permits";
    if (p.kind === "intersection") return p.exact ? "Cross streets" : "These streets don’t seem to cross (closest points " + feet(p.gap_m) + " apart)";
    return p.exact ? "Address" : "Nearest recorded block to the " + p.wanted + " block";
  }

  let results = [], active = -1, searchTimer = null;
  function showResults(list, query) {
    results = list;
    active = list.length ? 0 : -1;
    const box = $("#results");
    if (!query) { box.hidden = true; $("#q").setAttribute("aria-expanded", "false"); return; }
    box.innerHTML = list.length
      ? list.map((p, i) => `<li role="option" id="res-${i}" data-i="${i}"${i === active ? ' aria-selected="true"' : ""}>` +
          `<span class="rname">${esc(placeLabel(p))}</span><span class="rnote">${esc(placeNote(p))}</span></li>`).join("")
      : `<li class="none">No street called “${esc(query)}” has recorded permits. Try a nearby cross street.</li>`;
    box.hidden = false;
    $("#q").setAttribute("aria-expanded", "true");
    $("#q").setAttribute("aria-activedescendant", active >= 0 ? "res-" + active : "");
  }
  function highlight(i) {
    if (!results.length) return;
    active = (i + results.length) % results.length;
    document.querySelectorAll("#results [role=option]").forEach(li => li.removeAttribute("aria-selected"));
    const li = $("#res-" + active);
    if (li) { li.setAttribute("aria-selected", "true"); li.scrollIntoView({ block: "nearest" }); }
    $("#q").setAttribute("aria-activedescendant", "res-" + active);
  }
  async function runSearch(query, pickFirst) {
    const my = ++st.searchSeq;
    if (query.trim().length < 2) { showResults([], ""); return; }
    let r;
    try { r = await getJSON("/api/search?limit=6&q=" + encodeURIComponent(query)); } catch (e) { return; }
    if (my !== st.searchSeq) return;
    showResults(r.places, query);
    if (pickFirst && r.places.length) choose(r.places[0]);
  }
  function closeResults() { $("#results").hidden = true; $("#q").setAttribute("aria-expanded", "false"); }

  function choose(p) {
    closeResults();
    // Empty the box once a place is chosen: its name is the heading of the panel below,
    // and leftover text would get the next search typed onto the end of it.
    $("#q").value = "";
    st.focus = p;
    st.focusShown = 15;
    st.streetBlocks = null;
    drawFocus(true);
    refreshFocus();
  }

  function drawFocus(move) {
    focusLayer.clearLayers();
    const p = st.focus;
    if (!p) return;
    const ink = css("--ink");
    if (p.kind === "street") {
      const b = p.bounds;
      if (move) map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: [30, 30], maxZoom: 17 });
      // Outline the street's own blocks so it stands out from its cross streets.
      (st.streetBlocks || []).forEach(sb => L.circleMarker([sb.lat, sb.lng], { pane: "dots", renderer: dotsR, interactive: false,
        fill: false, radius: radius(Math.max(1, sb.count), map.getZoom()) + 3, color: css("--on-line"), weight: 2 }).addTo(focusLayer));
      return;
    }
    const r = +$("#radius").value;
    const circle = L.circle([p.lat, p.lng], { radius: r, pane: "lines", renderer: linesR, interactive: false,
      color: ink, weight: 1.5, dashArray: "6 5", fillColor: ink, fillOpacity: 0.05 }).addTo(focusLayer);
    L.circleMarker([p.lat, p.lng], { pane: "dots", renderer: dotsR, interactive: false, radius: 5,
      color: css("--paper"), weight: 2, fillColor: ink, fillOpacity: 1 }).addTo(focusLayer);
    if (move) map.fitBounds(circle.getBounds(), { padding: [40, 40], maxZoom: 18 });
  }

  async function refreshFocus() {
    const p = st.focus, box = $("#focus");
    if (!p) { box.hidden = true; box.innerHTML = ""; return; }
    const my = ++st.focusSeq;
    box.hidden = false;
    try {
      if (p.kind === "street") {
        const s = await getJSON("/api/street?name=" + encodeURIComponent(p.street) + "&" + qs());
        if (my === st.focusSeq) { box.innerHTML = streetHTML(p, s); st.streetBlocks = s.blocks; drawFocus(false); }
      } else {
        const n = await getJSON(`/api/nearby?lat=${p.lat}&lng=${p.lng}&r=${+$("#radius").value}&` + qs());
        if (my === st.focusSeq) box.innerHTML = nearbyHTML(p, n);
      }
    } catch (e) {
      if (my === st.focusSeq) box.innerHTML = `<p class="warn">Could not load this place: ${esc(e.message)}</p>`;
    }
  }

  const clearButton = '<button type="button" class="linkish" data-clear="1">Clear</button>';

  function barsHTML(rows, max) {
    return `<ul class="bars">${rows.map(r =>
      `<li><span class="bl">${esc(r.label)}</span><span class="bt"><i style="width:${esc(Math.round(100 * r.n / Math.max(1, max)))}%"></i></span><span class="bn">${fmt(r.n)}</span></li>`).join("")}</ul>`;
  }

  function nearbyHTML(p, n) {
    const head = `<div class="fhead"><h3>Within ${esc(radiusLabel())} of ${esc(placeLabel(p))}</h3>${clearButton}</div>`;
    const caveat = p.kind === "address" && !p.exact
      ? `<p class="hint note2">No permits are recorded on the ${esc(p.wanted)} block, so the circle is centred on the nearest block that has some.</p>`
      : p.kind === "intersection" && !p.exact ? `<p class="hint note2">${esc(placeNote(p))}.</p>` : "";
    if (!n.count) return head + caveat + '<p class="sub">No permits match the current filters inside this circle.</p>';
    const extra = n.unplaced ? ` · ${fmt(n.unplaced)} of them have no street number (often at intersections), so they are not on the block map` : "";
    const cats = n.categories.slice(0, 6).map(c => ({ label: c.key, n: c.n }));
    const counts = {};
    n.per_year.forEach(y => { counts[y.key] = y.n; });
    const shown = n.records.slice(0, st.focusShown);
    const more = n.records.length > shown.length
      ? `<button class="more" data-fmore="1">Show ${fmt(n.records.length - shown.length)} more</button>`
      : n.truncated ? `<p class="hint note2">Listing the newest ${fmt(n.records.length)}; the counts above cover all ${fmt(n.count)}.</p>` : "";
    return head + caveat +
      `<p class="sub">${fmt(n.count)} permit${n.count === 1 ? "" : "s"} on ${fmt(n.blocks)} block${n.blocks === 1 ? "" : "s"}${esc(extra)}</p>` +
      barsHTML(cats, cats.length ? cats[0].n : 1) + sparkCounts(counts) +
      `<ul class="records">${shown.map(r => recordHTML(r, [esc(feet(r.distance_m) + " away"), blockLink(r)])).join("")}</ul>` + more;
  }

  function blockLink(r) {
    if (!r.block_key) return "no street number";
    return `<button type="button" class="linkish" data-block="${esc(r.block_key)}" data-lat="${esc(r.lat)}" data-lng="${esc(r.lng)}">${esc(r.block)} block of ${esc(pretty(r.street))}</button>`;
  }

  function streetHTML(p, s) {
    const head = `<div class="fhead"><h3>${esc(pretty(p.street))}</h3>${clearButton}</div>`;
    if (!s.blocks.length) return head + '<p class="sub">No mapped blocks.</p>';
    const busy = s.blocks.filter(b => b.count > 0).length;
    const max = Math.max(1, ...s.blocks.map(b => b.count));
    const rows = s.blocks.map(b =>
      `<li><button type="button" class="srow" data-block="${esc(b.key)}" data-lat="${esc(b.lat)}" data-lng="${esc(b.lng)}">` +
      `<span class="bl">${esc(b.block)}</span><span class="bt"><i style="width:${esc(Math.round(100 * b.count / max))}%"></i></span><span class="bn">${fmt(b.count)}</span></button></li>`).join("");
    return head + `<p class="sub">${fmt(s.count)} permits on ${fmt(busy)} of ${fmt(s.blocks.length)} recorded blocks, in address order. Select a block to see its history.</p>` +
      `<ul class="bars street">${rows}</ul>`;
  }

  // ---- selected block -------------------------------------------------
  async function select(b) {
    st.sel = { key: b.key, lat: b.lat, lng: b.lng, count: b.count };
    st.shown = 40;
    placeRing();
    await showDetail();
  }

  async function showDetail() {
    if (!st.sel) return;
    const my = ++st.detailSeq;
    let d;
    try {
      d = await getJSON("/api/block?key=" + encodeURIComponent(st.sel.key) + "&" + qs());
    } catch (e) {
      $("#detail").innerHTML = `<p class="warn">Could not load this block: ${esc(e.message)}</p>`;
      return;
    }
    if (my !== st.detailSeq) return;
    st.sel.count = d.count;
    placeRing();
    const dates = d.records.map(r => r.date).filter(Boolean).sort();
    const span = dates.length ? `${esc(dates[0].slice(0, 4))}–${esc(dates[dates.length - 1].slice(0, 4))}` : "";
    const sub = d.count ? `${fmt(d.count)} permit${d.count === 1 ? "" : "s"}${span ? " · " + span : ""}` : "No permits match the current filters";
    const shown = d.records.slice(0, st.shown);
    $("#detail").innerHTML =
      `<h3>${esc(d.block)} block of ${esc(pretty(d.street))}</h3><p class="sub">${sub}</p>` + spark(d.records) +
      `<ul class="records">${shown.map(recordHTML).join("")}</ul>` +
      (d.records.length > shown.length ? `<button class="more" data-more="1">Show all ${fmt(d.records.length)}</button>` : "");
  }

  // extra: notes that are already HTML (escaped by the caller), shown first.
  function recordHTML(r, extra = []) {
    const cls = r.no_work ? " nowork" : "";
    const what = r.description || r.sub_type || "No description";
    const permit = r.pro_url ? `<a href="${esc(r.pro_url)}" target="_blank" rel="noopener noreferrer">${esc(r.permit)}</a>` : esc(r.permit);
    const notes = [...extra, permit, `as filed: “${esc(r.raw_street)}”`];
    if (r.status) notes.push(esc(r.status.toLowerCase()));
    if (r.date_basis && r.date_basis !== "DATE_ISSUED") notes.push("dated by " + esc(r.date_basis.replace("DATE_", "").toLowerCase()));
    if (r.date_flag) notes.push("ignored an implausible date");
    if (r.street_source) notes.push("street read from the location text");
    return `<li class="rec${cls}"><time>${esc(r.date || "No date")}</time>` +
      `<div class="what"><span class="cat">${esc(r.category)}</span>${esc(what)}</div><div class="meta">${notes.join(" · ")}</div></li>`;
  }

  // Permits per year for the selected block, across the chosen year range.
  function spark(records) {
    const counts = {};
    records.forEach(r => { const y = String(r.date || "").slice(0, 4); if (y) counts[y] = (counts[y] || 0) + 1; });
    return sparkCounts(counts);
  }

  // counts: {"2019": 3, ...}, drawn across the chosen year range.
  function sparkCounts(counts) {
    const y0 = st.from, y1 = st.to;
    if (!y0 || y1 < y0) return "";
    const n = y1 - y0 + 1, by = new Array(n).fill(0);
    for (let y = y0; y <= y1; y++) by[y - y0] = counts[y] || 0;
    const mx = Math.max(1, ...by), W = 336, H = 40, bw = W / n;
    const bars = by.map((c, i) => {
      if (!c) return "";
      const h = (c / mx) * (H - 12) + 2;
      return `<rect x="${(i * bw + 0.5).toFixed(1)}" y="${(H - h).toFixed(1)}" width="${Math.max(1, bw - 1).toFixed(1)}" height="${h.toFixed(1)}"><title>${y0 + i}: ${c}</title></rect>`;
    }).join("");
    return `<svg class="spark" viewBox="0 0 ${W} ${H + 12}" role="img" aria-label="Permits per year">${bars}` +
      `<line x1="0" x2="${W}" y1="${H}" y2="${H}"/><text x="0" y="${H + 11}">${y0}</text><text x="${W}" y="${H + 11}" text-anchor="end">${y1}</text></svg>`;
  }

  // ---- overlays (resurfacing layers) -----------------------------------
  const ovLabel = n => LABELS[n] || n.replace(/_/g, " ");

  function overlayStyle(f) {
    const road = css("--road");
    return /Polygon/.test((f.geometry && f.geometry.type) || "")
      ? { color: road, weight: 1.5, fillColor: road, fillOpacity: 0.12 }
      : { color: road, weight: 3, opacity: 0.8 };
  }

  function popupHTML(props) {
    const rows = Object.entries(props || {})
      .filter(([k, v]) => v !== null && v !== "" && !/^(objectid|globalid|shape|datasource)/i.test(k))
      .slice(0, 12).map(([k, v]) => `<tr><td>${esc(k)}</td><td>${esc(v)}</td></tr>`);
    return `<table>${rows.join("")}</table>`;
  }

  async function toggleOverlay(name, on, note) {
    note.textContent = "";
    if (!on) {
      if (st.overlays[name]) { map.removeLayer(st.overlays[name]); delete st.overlays[name]; }
      return;
    }
    try {
      const g = await getJSON("/api/overlay/" + encodeURIComponent(name));
      st.overlays[name] = L.geoJSON(g, {
        pane: "lines", renderer: linesR, style: overlayStyle,
        onEachFeature: (f, l) => l.bindPopup(popupHTML(f.properties)),
      }).addTo(map);
      if (!g.features.length) note.textContent = "This layer has no rows.";
    } catch (e) {
      note.textContent = "Could not load: " + e.message;
    }
  }

  // ---- setup ----------------------------------------------------------
  function updateYearNote() {
    $("#yearnote").textContent = st.from < SPARSE_BEFORE
      ? "The city\u2019s records before " + SPARSE_BEFORE + " are sparse, so older years look quieter than they were." : "";
  }

  function buildControls(m) {
    const y0 = +m.date_min.slice(0, 4), y1 = +m.date_max.slice(0, 4);
    for (let y = y0; y <= y1; y++) for (const id of ["#from", "#to"]) {
      const o = document.createElement("option"); o.value = o.textContent = y; $(id).append(o);
    }
    st.from = y0 < SPARSE_BEFORE && y1 >= SPARSE_BEFORE ? SPARSE_BEFORE : y0;
    st.to = y1; $("#from").value = st.from; $("#to").value = y1;
    updateYearNote();
    // Street closures for events and equipment on poles are not street work, so start with them off.
    m.categories.forEach(c => { if (c.key !== "Civic event" && c.key !== "Small wireless") st.cats.add(c.key); });
    $("#cats").innerHTML = m.categories.map(c =>
      `<label class="chip"><input type="checkbox" data-c="${esc(c.key)}"${st.cats.has(c.key) ? " checked" : ""}><span>${esc(c.key)}<i>${fmt(c.n)}</i></span></label>`).join("");

    const rows = n => { const s = m.sources.find(x => x.name === n); return s ? s.count : null; };
    $("#overlays").innerHTML = m.overlays.map(n => {
      const c = rows(n), empty = c === 0;
      return `<label${empty ? ' class="off"' : ""}><input type="checkbox" data-o="${esc(n)}"${empty ? " disabled" : ""}> <span class="swatch"></span>` +
        `<span>${esc(ovLabel(n))} <span class="n">${c === null ? "" : empty ? "no rows downloaded" : fmt(c) + " segments"}</span></span><span class="warn"></span></label>`;
    }).join("");

    const pct = n => (m.unique_permits ? (100 * n / m.unique_permits).toFixed(1) + "%" : "0%");
    $("#gaps").innerHTML = [
      `${fmt(m.no_block)} permits (${pct(m.no_block)}) have no street number, so they can’t be placed on a block and aren’t on the map.`,
      `${fmt(m.no_date)} permits have no usable date and are hidden whenever a year range is set. ${fmt(m.bad_dates)} had an implausible date, such as a year in the 2040s, which was skipped.`,
      `${fmt(m.no_work)} permits were voided, withdrawn, cancelled or denied, or are still waiting for review, so no work followed (yet). They are hidden unless you include them.`,
      "Permits are grouped by street name and hundred-block. A permit near an intersection may be filed under either street.",
      "Records before 2006 are not fully digitized by the city, and work done without a permit never appears.",
      "A permit that follows a resurfacing shows sequence, not cause.",
    ].map(t => `<li>${esc(t)}</li>`).join("");
    $("#foot").textContent = m.attribution + " Downloaded " + m.sources.map(s => `${s.name}: ${fmt(s.count)} rows on ${s.fetched_at.slice(0, 10)}`).join("; ") + ".";
  }

  let timer = null;
  const reload = () => { load(); if (st.sel) showDetail(); refreshFocus(); };
  const soon = () => { clearTimeout(timer); timer = setTimeout(load, 180); };

  async function init() {
    let m;
    try { m = await getJSON("/api/meta"); } catch (e) { setStatus("Could not reach the server: " + e.message); return; }
    st.meta = m;
    if (!m.unique_permits || !m.date_min) {
      setStatus("No permits are loaded. Run streetworks ingest, then restart serve.");
      $("#detail").innerHTML = '<p class="warn">No permit data found.</p>';
      return;
    }
    buildControls(m);
    try { basemap = (await getJSON("/api/config")).basemap; } catch (e) { setStatus("Could not load map settings: " + e.message); }
    setTiles();
    darkMQ.addEventListener("change", () => { setTiles(); load(); });
    map.on("moveend", soon);
    map.on("zoomend", () => { if (st.focus && st.focus.kind === "street") drawFocus(false); });
    window.addEventListener("resize", () => map.invalidateSize());
    const b = m.bounds;
    if (b.some(Boolean)) map.fitBounds([[b[0], b[1]], [b[2], b[3]]]);
    load();
  }

  $("#from").onchange = e => { st.from = +e.target.value; if (st.from > st.to) { st.to = st.from; $("#to").value = st.to; } updateYearNote(); reload(); };
  $("#to").onchange = e => { st.to = +e.target.value; if (st.to < st.from) { st.from = st.to; $("#from").value = st.from; } reload(); };
  const setAllCats = on => {
    document.querySelectorAll("#cats input").forEach(i => { i.checked = on; on ? st.cats.add(i.dataset.c) : st.cats.delete(i.dataset.c); });
    reload();
  };
  $("#cats-all").onclick = () => setAllCats(true);
  $("#cats-none").onclick = () => setAllCats(false);
  $("#cats").addEventListener("change", e => { const c = e.target.dataset.c; if (!c) return; e.target.checked ? st.cats.add(c) : st.cats.delete(c); reload(); });
  $("#nowork").onchange = e => { st.nowork = e.target.checked; reload(); };
  $("#overlays").addEventListener("change", e => {
    const n = e.target.dataset.o;
    if (n) toggleOverlay(n, e.target.checked, e.target.closest("label").querySelector(".warn"));
  });
  $("#top").addEventListener("click", e => {
    const b = e.target.closest("button");
    if (!b) return;
    const pos = { key: b.dataset.k, lat: +b.dataset.lat, lng: +b.dataset.lng, count: +b.dataset.n };
    map.setView([pos.lat, pos.lng], Math.max(map.getZoom(), 16));
    select(pos);
  });
  $("#q").addEventListener("input", e => {
    clearTimeout(searchTimer);
    const v = e.target.value;
    searchTimer = setTimeout(() => runSearch(v, false), 180);
  });
  $("#q").addEventListener("keydown", e => {
    if ($("#results").hidden) return;
    if (e.key === "ArrowDown") { e.preventDefault(); highlight(active + 1); }
    else if (e.key === "ArrowUp") { e.preventDefault(); highlight(active - 1); }
    else if (e.key === "Escape") { closeResults(); }
  });
  $("#search").addEventListener("submit", e => {
    e.preventDefault();
    clearTimeout(searchTimer);
    if (!$("#results").hidden && results[active]) choose(results[active]);
    else runSearch($("#q").value, true);
  });
  $("#results").addEventListener("mousedown", e => { // mousedown fires before the input loses focus
    const li = e.target.closest("[data-i]");
    if (li) { e.preventDefault(); choose(results[+li.dataset.i]); }
  });
  $("#q").addEventListener("blur", () => setTimeout(closeResults, 120));
  $("#radius").onchange = () => { if (st.focus && st.focus.kind !== "street") { drawFocus(true); refreshFocus(); } };
  $("#focus").addEventListener("click", e => {
    if (e.target.closest("[data-clear]")) {
      st.focus = null; focusLayer.clearLayers(); refreshFocus(); $("#q").focus(); return;
    }
    if (e.target.closest("[data-fmore]")) { st.focusShown = Infinity; refreshFocus(); return; }
    const b = e.target.closest("[data-block]");
    if (b) {
      const pos = { key: b.dataset.block, lat: +b.dataset.lat, lng: +b.dataset.lng, count: 1 };
      map.setView([pos.lat, pos.lng], Math.max(map.getZoom(), 17));
      select(pos);
      $("#detail").scrollIntoView({ behavior: "smooth", block: "start" });
    }
  });
  $("#detail").addEventListener("click", e => {
    if (!e.target.closest("[data-more]")) return;
    st.shown = Infinity;
    showDetail();
  });

  init();
})();
