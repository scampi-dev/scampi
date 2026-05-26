---
title: Benchmarks
linkTitle: Benchmarks
weight: 4
type: docs
sidebar:
  open: true
description: Time series of the scampi integration benchmark suite. Recorded by CI on every push to main, served fresh from raw text without a site rebuild.
toc: true
---

**Runner**: 2 vCPU, 2 GB RAM — Forgejo Actions agent on the
scampi.dev VPS. Recorded fresh on every push to `main`.

<div id="bench-status" class="bench-status">loading…</div>

{{% bench-charts %}}

<script>
"use strict";
(function () {
  const SUFFIX = "ci-gh-scampi";
  const BASE = "/raw-bench/" + SUFFIX;
  const status = document.getElementById("bench-status");

  const COLORS = [
    "#458588", "#d65d0e", "#98971a", "#b16286",
    "#cc241d", "#689d6a", "#d79921", "#928374",
  ];
  const MARGIN = { top: 10, right: 20, bottom: 50, left: 72 };
  const WIDTH = 780;
  const HEIGHT = 280;
  const innerW = WIDTH - MARGIN.left - MARGIN.right;
  const innerH = HEIGHT - MARGIN.top - MARGIN.bottom;

  // Benchstat output line:
  //   BenchmarkApplyNoOp/Size-1-2  	   1	  2845167 ns/op	  741392 B/op	   17234 allocs/op
  const BENCH_RE = /^Benchmark([^/\s]+)(?:\/Size-(\d+))?-\d+\s+\d+\s+([\d.]+)\s+ns\/op/;
  const TS_RE = /(\d{4}-\d{2}-\d{2})T(\d{2})(\d{2})/;

  function fmtValue(v) {
    if (v >= 1) return v.toFixed(3) + " s";
    if (v >= 1e-3) return (v * 1e3).toFixed(3) + " ms";
    if (v >= 1e-6) return (v * 1e6).toFixed(3) + " µs";
    return (v * 1e9).toFixed(1) + " ns";
  }

  function fmtDate(d) { return d.replace("T", " "); }

  function median(xs) {
    const s = xs.slice().sort((a, b) => a - b);
    const n = s.length;
    if (n === 0) return 0;
    if (n % 2 === 1) return s[(n - 1) / 2];
    return (s[n / 2 - 1] + s[n / 2]) / 2;
  }

  function tsFromFilename(name) {
    const m = TS_RE.exec(name);
    if (!m) return null;
    return m[1] + "T" + m[2] + ":" + m[3];
  }

  function parseFile(text) {
    const rows = [];
    for (const line of text.split("\n")) {
      const m = BENCH_RE.exec(line.trim());
      if (!m) continue;
      rows.push({ family: m[1], size: m[2] || "1", ns: parseFloat(m[3]) });
    }
    return rows;
  }

  // values[family][size][ts] = [ns, ...]
  function ingest(values, ts, rows) {
    for (const r of rows) {
      if (!values[r.family]) values[r.family] = {};
      if (!values[r.family][r.size]) values[r.family][r.size] = {};
      if (!values[r.family][r.size][ts]) values[r.family][r.size][ts] = [];
      values[r.family][r.size][ts].push(r.ns);
    }
  }

  function familySeries(sizes, allDates) {
    const series = [];
    const sizeKeys = Object.keys(sizes).sort((a, b) => parseInt(a) - parseInt(b));
    for (const size of sizeKeys) {
      const pts = [];
      for (const ts of Object.keys(sizes[size]).sort()) {
        pts.push({ date: ts, value: median(sizes[size][ts]) / 1e9 });
      }
      series.push({ size, points: pts });
    }
    return series;
  }

  function logTicks(lo, hi) {
    const ticks = [];
    let exp = Math.floor(Math.log10(lo));
    const maxExp = Math.ceil(Math.log10(hi));
    while (exp <= maxExp) {
      const v = Math.pow(10, exp);
      if (v >= lo * 0.99 && v <= hi * 1.01) ticks.push(v);
      exp++;
    }
    if (ticks.length < 2) {
      ticks.length = 0;
      exp = Math.floor(Math.log10(lo));
      while (exp <= maxExp) {
        for (const m of [1, 2, 5]) {
          const v = m * Math.pow(10, exp);
          if (v >= lo * 0.99 && v <= hi * 1.01) ticks.push(v);
        }
        exp++;
      }
    }
    // Final safety net: a range narrow enough to miss every 1/2/5
    // multiplier (e.g. 130ns..145ns) still deserves at least one
    // labelled gridline. Fall back to the geometric midpoint.
    if (ticks.length === 0) {
      ticks.push(Math.sqrt(lo * hi));
    }
    return ticks;
  }

  function scaleLog(v, lo, hi) {
    const logLo = Math.log10(lo), logHi = Math.log10(hi);
    if (logHi === logLo) return 0.5;
    return (Math.log10(v) - logLo) / (logHi - logLo);
  }

  // Build one chart inside the given container. The container is
  // emitted by the bench-charts shortcode (one per benchmark known to
  // site.Data.benchmarks).
  function renderChart(container, family, series, allDates, tooltip) {
    const ns = "http://www.w3.org/2000/svg";
    container.innerHTML = "";

    if (!series.length || !series.some((s) => s.points.length)) {
      const empty = document.createElement("p");
      empty.className = "bench-empty";
      empty.textContent = "no data yet";
      container.appendChild(empty);
      return;
    }

    let minV = Infinity, maxV = -Infinity;
    for (const s of series) {
      for (const p of s.points) {
        if (p.value > 0) {
          if (p.value < minV) minV = p.value;
          if (p.value > maxV) maxV = p.value;
        }
      }
    }
    if (minV === Infinity) { minV = 1e-9; maxV = 1; }
    const pad = 0.15;
    const logRange = Math.log10(maxV) - Math.log10(minV);
    const lo = Math.pow(10, Math.log10(minV) - logRange * pad);
    const hi = Math.pow(10, Math.log10(maxV) + logRange * pad);
    const yPos = (v) => innerH - scaleLog(v, lo, hi) * innerH;

    const xStep = allDates.length > 1 ? innerW / (allDates.length - 1) : innerW / 2;
    const dateIndex = {};
    allDates.forEach((d, i) => { dateIndex[d] = i; });
    const xPos = (date) => allDates.length === 1 ? innerW / 2 : dateIndex[date] * xStep;

    const svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 " + WIDTH + " " + HEIGHT);
    svg.setAttribute("preserveAspectRatio", "xMidYMid meet");

    const g = document.createElementNS(ns, "g");
    g.setAttribute("transform", "translate(" + MARGIN.left + "," + MARGIN.top + ")");
    svg.appendChild(g);

    for (const v of logTicks(lo, hi)) {
      const y = yPos(v);
      const line = document.createElementNS(ns, "line");
      line.setAttribute("x1", 0); line.setAttribute("x2", innerW);
      line.setAttribute("y1", y); line.setAttribute("y2", y);
      line.setAttribute("class", "bench-grid");
      g.appendChild(line);

      const label = document.createElementNS(ns, "text");
      label.setAttribute("x", -8); label.setAttribute("y", y + 4);
      label.setAttribute("text-anchor", "end");
      label.setAttribute("class", "bench-axis-label");
      label.textContent = fmtValue(v);
      g.appendChild(label);
    }

    const maxLabels = Math.floor(innerW / 80);
    const step = Math.max(1, Math.ceil(allDates.length / maxLabels));
    allDates.forEach((d, i) => {
      if (i % step !== 0 && i !== allDates.length - 1) return;
      const x = xPos(d);
      const tick = document.createElementNS(ns, "line");
      tick.setAttribute("x1", x); tick.setAttribute("x2", x);
      tick.setAttribute("y1", innerH); tick.setAttribute("y2", innerH + 6);
      tick.setAttribute("class", "bench-axis");
      g.appendChild(tick);

      const parts = d.split("T");
      const lab1 = document.createElementNS(ns, "text");
      lab1.setAttribute("x", x); lab1.setAttribute("y", innerH + 20);
      lab1.setAttribute("text-anchor", "middle");
      lab1.setAttribute("class", "bench-axis-label");
      lab1.textContent = parts[0];
      g.appendChild(lab1);
      if (parts[1]) {
        const lab2 = document.createElementNS(ns, "text");
        lab2.setAttribute("x", x); lab2.setAttribute("y", innerH + 32);
        lab2.setAttribute("text-anchor", "middle");
        lab2.setAttribute("class", "bench-axis-label");
        lab2.textContent = parts[1];
        g.appendChild(lab2);
      }
    });

    for (const [x1, y1, x2, y2] of [[0, innerH, innerW, innerH], [0, 0, 0, innerH]]) {
      const ax = document.createElementNS(ns, "line");
      ax.setAttribute("x1", x1); ax.setAttribute("y1", y1);
      ax.setAttribute("x2", x2); ax.setAttribute("y2", y2);
      ax.setAttribute("class", "bench-axis");
      g.appendChild(ax);
    }

    const seriesGroups = [];
    series.forEach((s, si) => {
      const color = COLORS[si % COLORS.length];
      const sg = document.createElementNS(ns, "g");

      if (s.points.length > 1) {
        let d = "";
        s.points.forEach((p, pi) => {
          d += (pi === 0 ? "M" : "L") + xPos(p.date) + "," + yPos(p.value);
        });
        const path = document.createElementNS(ns, "path");
        path.setAttribute("d", d);
        path.setAttribute("fill", "none");
        path.setAttribute("stroke", color);
        path.setAttribute("stroke-width", "2");
        sg.appendChild(path);
      }

      s.points.forEach((p) => {
        const dot = document.createElementNS(ns, "circle");
        dot.setAttribute("cx", xPos(p.date));
        dot.setAttribute("cy", yPos(p.value));
        dot.setAttribute("r", "4");
        dot.setAttribute("fill", color);
        dot.setAttribute("class", "bench-dot");
        dot.addEventListener("mouseenter", () => {
          tooltip.style.display = "block";
          tooltip.innerHTML =
            '<span class="bench-tooltip-label">Size ' + s.size + '</span><br>' +
            fmtValue(p.value) + '<br>' +
            '<span class="bench-tooltip-label">' + fmtDate(p.date) + '</span>';
        });
        dot.addEventListener("mousemove", (e) => {
          tooltip.style.left = (e.clientX + 12) + "px";
          tooltip.style.top = (e.clientY - 10) + "px";
        });
        dot.addEventListener("mouseleave", () => { tooltip.style.display = "none"; });
        sg.appendChild(dot);
      });

      g.appendChild(sg);
      seriesGroups.push({ el: sg, color, size: s.size, visible: true });
    });

    container.appendChild(svg);

    const legend = document.createElement("div");
    legend.className = "bench-legend";
    seriesGroups.forEach((sg) => {
      const item = document.createElement("button");
      item.type = "button";
      item.className = "bench-legend-item";
      const swatch = document.createElement("span");
      swatch.className = "bench-legend-swatch";
      swatch.style.background = sg.color;
      const label = document.createElement("span");
      label.textContent = "Size " + sg.size;
      item.appendChild(swatch);
      item.appendChild(label);
      item.addEventListener("click", () => {
        sg.visible = !sg.visible;
        sg.el.style.display = sg.visible ? "" : "none";
        item.classList.toggle("hidden", !sg.visible);
      });
      legend.appendChild(item);
    });
    container.appendChild(legend);
  }

  async function load() {
    let manifest;
    try {
      const r = await fetch(BASE + "/index.json", { cache: "no-cache" });
      if (!r.ok) throw new Error("index.json: HTTP " + r.status);
      manifest = await r.json();
    } catch (e) {
      status.textContent = "failed to load manifest: " + e.message;
      return;
    }
    if (!Array.isArray(manifest) || manifest.length === 0) {
      status.textContent = "no benchmark runs recorded yet";
      return;
    }

    const values = {};
    const dates = [];
    const fetches = manifest.map(async (name) => {
      const ts = tsFromFilename(name);
      if (!ts) return null;
      try {
        const r = await fetch(BASE + "/" + name, { cache: "no-cache" });
        if (!r.ok) throw new Error("HTTP " + r.status);
        return { ts, rows: parseFile(await r.text()) };
      } catch (e) {
        console.warn("skip " + name + ": " + e.message);
        return null;
      }
    });
    const results = await Promise.all(fetches);
    for (const res of results) {
      if (!res) continue;
      if (!dates.includes(res.ts)) dates.push(res.ts);
      ingest(values, res.ts, res.rows);
    }
    dates.sort();

    status.textContent = dates.length + " run" + (dates.length === 1 ? "" : "s") +
      " (" + fmtDate(dates[0]) + " — " + fmtDate(dates[dates.length - 1]) + ")";

    const tooltip = document.createElement("div");
    tooltip.className = "bench-tooltip";
    document.body.appendChild(tooltip);

    // Render into each chart container the shortcode emitted. Families
    // present in the data but missing a container (e.g. a brand-new
    // benchmark since the last `just generate`) get a fallback section
    // appended at the end so nothing silently disappears.
    const seen = new Set();
    document.querySelectorAll("[data-bench]").forEach((el) => {
      const family = el.getAttribute("data-bench");
      seen.add(family);
      const series = values[family] ? familySeries(values[family], dates) : [];
      renderChart(el, family, series, dates, tooltip);
    });

    for (const family of Object.keys(values).sort()) {
      if (seen.has(family)) continue;
      const section = document.createElement("section");
      section.className = "bench-card";
      section.id = "bench-" + family.toLowerCase().replace(/[^a-z0-9]+/g, "-");
      const h = document.createElement("h2");
      h.className = "bench-card-title";
      h.textContent = "Benchmark" + family;
      const note = document.createElement("p");
      note.className = "bench-card-desc";
      note.textContent = "No description yet — run `just generate` after adding a doc comment.";
      const chart = document.createElement("div");
      chart.className = "bench-card-chart";
      section.appendChild(h);
      section.appendChild(note);
      section.appendChild(chart);
      document.getElementById("content").appendChild(section);
      renderChart(chart, family, familySeries(values[family], dates), dates, tooltip);
    }
  }

  load();
})();
</script>
