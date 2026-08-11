// gorumsplot — cetz-plot helpers for gorums benchmark reports.
//
// This library is embedded in the `sweep` binary and copied verbatim into each
// report directory next to a generated `report.typ`, so a report compiles on
// any machine with Typst installed. Figures are drawn natively at compile time
// from the CSV files `sweep -plot` writes.
//
// The generated report.typ loads the tidy-long `agg.csv` (one row per
// benchmark configuration, rep-averaged, with `_sd`/`_ci95` spread columns)
// and calls the figure functions below. Each function derives its panels
// (facets) and series (one line per benchmark × stream mode × held-fixed
// dimensions) from the data itself; the caller supplies only the axis columns,
// labels, and toggles.
//
// Labelling convention: a figure names only what varies within it. Its legend
// carries the dimensions that differ between its series, the panel titles carry
// the faceted dimension, and everything the whole figure holds fixed (the
// benchmark, a single stream mode) belongs in the section heading the generator
// writes, not in every legend entry.

#import "@preview/cetz:0.4.2"
#import "@preview/cetz-plot:0.1.3": plot

// ── Font sizes (resize plot text everywhere) ────────────────────────────────
#let tick-size = 8pt
#let axis-label-size = 9.5pt
#let panel-title-size = 11pt
#let legend-size = 9pt

// ── Palettes ────────────────────────────────────────────────────────────────
// Benchmarks differ by marker + fallback color; stream modes drive line color
// when two or more modes are compared (the mode is then the primary contrast).
#let bench-palette = (
  rgb("#1f77b4"), rgb("#d62728"), rgb("#2ca02c"), rgb("#9467bd"),
  rgb("#ff7f0e"), rgb("#8c564b"), rgb("#17becf"), rgb("#bcbd22"),
)
#let bench-marks = ("o", "square", "triangle", "diamond", "x", "+", "*", "o")
#let mode-colors = (dual: rgb("#1f77b4"), dedup: rgb("#d62728"), baseline: rgb("#7f7f7f"))
#let line-dashes = (none, "dashed", "dash-dotted", "dotted")

// The dimension columns a figure can hold fixed as series identity. Buffer
// capacities are included so a buffer sweep's arms stay distinct series
// instead of collapsing onto each other (row-at then returns only the first
// matching row per x value, silently dropping the rest).
#let dim-cols = ("nodes", "workers", "payload", "rate", "send_buffer", "recv_buffer")

// Short tags for the dimension columns, so a legend entry reads "R1000, dedup"
// rather than "rate=1000, dedup". A column without a tag keeps its name.
#let dim-short = (nodes: "N", workers: "W", payload: "P", rate: "R", send_buffer: "SB", recv_buffer: "RB")
#let dim-tag(col, value) = dim-short.at(col, default: col + "=") + value

// Band overlays use the `_ci95` half-width the CSV already carries (Go computed
// the 95% CI of the mean). band-opacity is the fill transparency.
#let band-opacity = 82%

// ── Small helpers ────────────────────────────────────────────────────────────

// Distinct values of a column, kept as strings but ordered by numeric value.
#let uniq-numsorted(data, col) = data.map(r => r.at(col)).dedup().sorted(key: v => float(v))

// Distinct values of a column in first-seen order (for non-numeric columns).
#let uniq(data, col) = data.map(r => r.at(col)).dedup()

// Scale arbitrary content (e.g. a figure grid) to the full available width.
#let fitwidth(body) = layout(size => {
  let m = measure(body)
  let f = if m.width == 0pt { 1.0 } else { size.width / m.width }
  scale(x: f * 100%, y: f * 100%, origin: top + left, reflow: true, body)
})

// gridcols is the column count for a panel grid holding count panels, at most
// max-cols wide and never zero: grid(columns: 0) is not a legal Typst grid, so a
// figure whose panel list came out empty renders an empty grid rather than
// failing the compilation.
#let gridcols(max-cols, count) = calc.max(1, calc.min(max-cols, count))

// Which axes panel i of a count-panel, cols-wide grid labels: the y label only
// in the first column and the x label only where no panel sits below, so a row
// of facets sharing one unit prints it once and spends the reclaimed width on
// the data instead.
#let show-y-label(i, cols) = calc.rem(i, cols) == 0
#let show-x-label(i, count, cols) = i + cols >= count

// nice-step is a round tick step that divides span into about target intervals:
// 1, 2, 2.5, or 5 times a power of ten. A small panel needs this because the
// automatic step is chosen from the data range alone, which packs a panel a
// third of the page wide with more labels than fit side by side.
#let nice-step(span, target: 5) = {
  if span <= 0 { return auto }
  let raw = span / target
  let mag = calc.pow(10, calc.floor(calc.log(raw)))
  let norm = raw / mag
  let mult = if norm <= 1 { 1 } else if norm <= 2 { 2 } else if norm <= 2.5 { 2.5 } else if norm <= 5 { 5 } else { 10 }
  mult * mag
}

// ── Shared legend, framed and placed above the panel grid ────────────────────
// items: array of (paint, label) or (paint, label, dash). The frame ties the
// legend to the figure below it; cols keeps it no wider than the panel grid.
#let swatch(paint, dash: none) = box(
  baseline: -0.18em,
  cetz.canvas(length: 1em, {
    import cetz.draw: *
    line((0, 0), (1.5, 0), stroke: (paint: paint, thickness: 2pt, dash: dash))
  }),
)

#let legend-gutter = 12pt

#let legend-entry(item, size) = {
  let dash = if item.len() > 2 { item.at(2) } else { none }
  box(text(size: size)[#swatch(item.at(0), dash: dash)#h(0.35em)#item.at(1)])
}

// legend-cells arranges entries down each column and then across, so a legend
// needing more than one row reads in series order downward: the entries the
// series order pairs — the two stream modes of one offered rate, say — sit above
// one another in a column instead of being split by a line break. The column
// count is then reduced to the rows the entries actually need, so the last column
// is the only ragged one and none is left empty.
#let legend-cells(entries, max-cols) = {
  let n = entries.len()
  let cols = calc.max(1, calc.min(max-cols, n))
  let rows = calc.ceil(n / cols)
  cols = calc.ceil(n / rows)
  let cells = ()
  for r in range(rows) {
    for k in range(cols) {
      let i = k * rows + r
      cells.push(if i < n { entries.at(i) } else { [] })
    }
  }
  (cols: cols, cells: cells)
}

#let legend-frame(entries, max-cols, caption, size) = {
  let arranged = legend-cells(entries, max-cols)
  block(
    stroke: 0.5pt + luma(180), radius: 2pt, inset: (x: 6pt, y: 4pt),
    {
      if caption != none { block(below: 0.4em, text(size: size, emph(caption))) }
      grid(
        columns: arranged.cols,
        column-gutter: legend-gutter,
        row-gutter: 0.35em,
        ..arranged.cells,
      )
    },
  )
}

// An empty legend draws nothing: grid(columns: 0) is not a legal Typst grid, so
// a caller whose series list came out empty gets a blank legend instead of a
// failed compilation. caption states what every entry shares, printed once above
// them rather than repeated in each. Given width, the entries are laid out as
// many to a row as fit it (measured against the widest of them), so a legend of
// short labels fills the figure's width instead of breaking after a fixed count.
#let hlegend(items, cols: auto, width: none, size: legend-size, caption: none) = {
  if items.len() == 0 { return [] }
  let entries = items.map(item => legend-entry(item, size))
  if cols != auto { return legend-frame(entries, cols, caption, size) }
  if width == none { return legend-frame(entries, 3, caption, size) }
  context {
    let widest = entries.fold(0pt, (m, e) => calc.max(m, measure(e).width))
    let cols = if widest <= 0pt { entries.len() } else {
      calc.max(1, int((width - 12pt + legend-gutter) / (widest + legend-gutter)))
    }
    legend-frame(entries, cols, caption, size)
  }
}

// ── One figure: a shared legend above a grid of panels ───────────────────────
// The legend is fitted to the panel grid's own width, so it never dictates the
// figure's size: fitwidth scales a figure to the text width, and a legend wider
// than the panels would spend that width on labels instead of data.
#let figure-stack(items, panels, cols, gutter: 2mm, caption: none) = context {
  let body = grid(columns: cols, gutter: gutter, ..panels)
  stack(
    spacing: 2mm,
    hlegend(items, width: measure(body).width, caption: caption),
    body,
  )
}

// ── Per-node figures: colors and labels for a run's nodes ────────────────────
// One line per node needs as many distinguishable colors as the run has nodes.
// Up to the palette's size the named colors are used; beyond it, hues are spread
// evenly over the wheel, since cycling the palette would give two nodes of one
// run the same color and make the legend ambiguous.
#let node-color(ni, count) = if count <= bench-palette.len() {
  bench-palette.at(calc.rem(ni, bench-palette.len()))
} else {
  color.hsl(360deg * ni / count, 68%, 42%)
}

// A node is labeled host:port, and every node of a run shares the port and,
// within one subnet, all but the last component of the address. elide-nodes
// strips what they all share and returns the remaining labels together with the
// pattern it stripped, so a legend states the shared part once instead of in
// every entry. Nodes that share nothing keep their full labels.
#let elide-nodes(nodes) = {
  if nodes.len() == 0 { return (labels: (), caption: none) }
  let split = nodes.map(nd => nd.split(":"))
  let hosts = split.map(p => p.at(0))
  let ports = split.map(p => if p.len() > 1 { ":" + p.slice(1).join(":") } else { "" })
  let port = if ports.dedup().len() == 1 { ports.at(0) } else { "" }
  let heads = hosts.map(h => h.split(".").slice(0, -1).join("."))
  let head = if heads.dedup().len() == 1 and heads.at(0) != "" { heads.at(0) + "." } else { "" }
  if head == "" and port == "" { return (labels: nodes, caption: none) }
  (
    labels: hosts.enumerate().map(((i, h)) => h.slice(head.len()) + ports.at(i).slice(port.len())),
    caption: "nodes " + head + "*" + port,
  )
}

// The legend entries for a run's nodes, with what they all share elided into the
// caption figure-stack prints above them.
#let node-items(nodes) = {
  let short = elide-nodes(nodes)
  (
    items: nodes.enumerate().map(((ni, nd)) => (node-color(ni, nodes.len()), short.labels.at(ni))),
    caption: short.caption,
  )
}

// ── One panel: a plot box with a centered title drawn just above it ──────────
#let panel(psize, title, body, x-label: none, y-label: none, x-ticks: (),
  x-tick-step: auto, y-tick-step: auto,
  x-min: auto, x-max: auto, y-min: auto, y-max: auto) = {
  set text(size: tick-size)
  cetz.canvas({
    import cetz.draw: content
    plot.plot(
      size: psize,
      x-label: if x-label == none { none } else { text(size: axis-label-size, x-label) },
      y-label: if y-label == none { none } else { text(size: axis-label-size, y-label) },
      x-ticks: x-ticks,
      x-tick-step: x-tick-step,
      y-tick-step: y-tick-step,
      x-min: x-min, x-max: x-max, y-min: y-min, y-max: y-max,
      legend: none,
      body,
    )
    if title != none {
      content(
        (psize.at(0) / 2, psize.at(1) + 0.3),
        text(size: panel-title-size, weight: "semibold", title),
        anchor: "south",
      )
    }
  })
}

// ── Series model ─────────────────────────────────────────────────────────────
// A series is one drawn line: a benchmark, a stream mode, and the values of
// every numeric dimension other than the x column and the facet column (which
// is fixed within a panel). Color encodes the mode when several modes are
// compared, else the benchmark; the marker always encodes the benchmark; the
// line dash distinguishes the held-fixed dimension combination.

// The numeric dimensions held fixed per series (all dims except x and facet).
#let fixed-dims(xcol, facet) = dim-cols.filter(c => c != xcol and c != facet)

// Distinct series keys present in `data`, each a dict (bench, mode, fixed:
// (values…)) over `fixed`. First-seen order.
#let series-keys(data, fixed) = {
  let keys = ()
  for r in data {
    let k = (bench: r.benchmark, mode: r.stream_mode, fixed: fixed.map(c => r.at(c)))
    if k not in keys { keys.push(k) }
  }
  keys
}

#let bench-index(benches, b) = {
  let i = benches.position(x => x == b)
  if i == none { 0 } else { i }
}

// Color for a stream mode: the named color for the modes gorums compares, or a
// palette slot keyed on the mode's position among the modes present, so any
// mode set stays visually distinct even beyond the named ones.
#let mode-color(modes, mode) = {
  if mode in mode-colors { mode-colors.at(mode) } else {
    bench-palette.at(calc.rem(modes.position(m => m == mode), bench-palette.len()))
  }
}

// Line color: the stream mode drives color when several modes are compared,
// otherwise the benchmark does.
#let series-color(modes, benches, key) = {
  if modes.len() > 1 {
    mode-color(modes, key.mode)
  } else {
    bench-palette.at(calc.rem(bench-index(benches, key.bench), bench-palette.len()))
  }
}

#let series-mark(benches, key) = bench-marks.at(calc.rem(bench-index(benches, key.bench), bench-marks.len()))

// A label for a series: only what distinguishes it from the figure's other
// series — the benchmark when several are drawn, every held-fixed dim that
// varies across the figure, and the mode when several are present. What the
// whole figure holds fixed belongs in the section heading instead. A figure
// with a single series has nothing to distinguish, so it is named by its
// benchmark.
#let series-label(key, fixed, varying-fixed, multi-bench, multi-mode) = {
  let parts = ()
  if multi-bench { parts.push(key.bench) }
  for (i, c) in fixed.enumerate() {
    if c in varying-fixed { parts.push(dim-tag(c, key.fixed.at(i))) }
  }
  if multi-mode { parts.push(key.mode) }
  if parts.len() == 0 { parts.push(key.bench) }
  parts.join(", ")
}

// Distinct held-fixed dimension tuples across the data, in first-seen order.
// A series' position in this list selects its line dash, so lines that share a
// color and marker (same benchmark and stream mode) stay distinguishable by
// the dimensions held fixed (e.g. two payloads within one node-count panel).
#let fixed-tuples(data, fixed) = {
  let ts = ()
  for r in data {
    let t = fixed.map(c => r.at(c))
    if t not in ts { ts.push(t) }
  }
  ts
}

#let series-dash(tuples, key) = {
  let i = tuples.position(t => t == key.fixed)
  if i == none { none } else { line-dashes.at(calc.rem(i, line-dashes.len())) }
}

// Rows of one series, in the order given.
#let series-rows(sub, key, fixed) = sub.filter(r => (
  r.benchmark == key.bench and r.stream_mode == key.mode
    and fixed.enumerate().all(((i, c)) => r.at(c) == key.fixed.at(i))
))

// Match a data row to a series key at a given x value.
#let row-at(sub, key, fixed, xcol, xv) = sub.find(r => (
  r.benchmark == key.bench and r.stream_mode == key.mode and r.at(xcol) == xv
    and fixed.enumerate().all(((i, c)) => r.at(c) == key.fixed.at(i))
))

// The legend items for a series list, one entry per distinct label.
#let series-legend(keys, modes, benches, tuples, fixed, varying-fixed) = {
  let items = ()
  let seen = ()
  for key in keys {
    let lbl = series-label(key, fixed, varying-fixed, benches.len() > 1, modes.len() > 1)
    if lbl not in seen {
      seen.push(lbl)
      items.push((series-color(modes, benches, key), lbl, series-dash(tuples, key)))
    }
  }
  items
}

// ── metric-vs: one metric against a numeric sweep dimension ──────────────────
// Categorical (evenly spaced) x with the swept values as tick labels — this
// reads cleanly for the log-spaced values gorums sweeps (2,4,8,… or 1Ki,4Ki,…)
// and avoids fragile log-axis tick math. Facets into one panel per value of
// `facet` (a dimension column) when given; series and their styling are derived
// from the data. `yscale` maps the CSV unit to the display unit named in
// `ylabel` (e.g. 1/1000 for ops/s → kops/s). `band-col` names the spread column
// for the shaded ±band (e.g. "throughput_ci95"); pass none to omit bands.
#let metric-vs(
  data, xcol: "workers", ycol: "throughput", band-col: none,
  ylabel: [], xlabel: none, yscale: 1.0,
  facet: none, facet-label: none, psize: (4.6, 3.1), y-min: 0, y-tick-step: auto,
  cols: 3,
) = {
  let benches = uniq(data, "benchmark")
  let modes = uniq(data, "stream_mode")
  let fixed = fixed-dims(xcol, facet)
  // Which held-fixed dims actually vary across the whole figure (for labels).
  let varying-fixed = fixed.filter(c => uniq(data, c).len() > 1)
  let tuples = fixed-tuples(data, fixed)
  let facet-vals = if facet == none { (none,) } else { uniq-numsorted(data, facet) }
  let ncols = gridcols(cols, facet-vals.len())

  let mk-panel(i, fval) = {
    let sub = if facet == none { data } else { data.filter(r => r.at(facet) == fval) }
    let xvals = uniq-numsorted(sub, xcol)
    let keys = series-keys(sub, fixed)
    let title = if facet == none { none } else {
      if facet-label == none { facet + " = " + fval } else { facet-label + " = " + fval }
    }
    panel(
      psize, title,
      x-label: if show-x-label(i, facet-vals.len(), ncols) { xlabel } else { none },
      y-label: if show-y-label(i, ncols) { ylabel } else { none },
      x-ticks: xvals.enumerate().map(((xi, v)) => (xi, raw(v))), x-tick-step: none,
      x-min: -0.3, x-max: xvals.len() - 0.7, y-min: y-min, y-tick-step: y-tick-step,
      {
        for key in keys {
          let paint = series-color(modes, benches, key)
          let dash = series-dash(tuples, key)
          let pts = xvals.enumerate().map(((xi, xv)) => {
            let hit = row-at(sub, key, fixed, xcol, xv)
            if hit == none or hit.at(ycol) == "" { none } else {
              let y = float(hit.at(ycol)) * yscale
              let e = if band-col == none or hit.at(band-col, default: "") == "" { 0.0 } else {
                float(hit.at(band-col)) * yscale
              }
              (xi, y, e)
            }
          }).filter(x => x != none)
          if pts.len() > 0 {
            if band-col != none {
              plot.add-fill-between(
                pts.map(t => (t.at(0), t.at(1) + t.at(2))),
                pts.map(t => (t.at(0), t.at(1) - t.at(2))),
                style: (fill: paint.transparentize(band-opacity), stroke: none),
              )
            }
            plot.add(
              pts.map(t => (t.at(0), t.at(1))),
              mark: series-mark(benches, key), mark-size: 0.11,
              style: (stroke: (paint: paint, thickness: 1pt, dash: dash)),
              mark-style: (stroke: paint, fill: paint),
            )
          }
        }
      },
    )
  }

  let legend-items = series-legend(
    series-keys(data, fixed), modes, benches, tuples, fixed, varying-fixed,
  )
  let panels = facet-vals.enumerate().map(((i, fval)) => mk-panel(i, fval))
  figure-stack(legend-items, panels, ncols)
}

// ── tl-curve: throughput–latency curves (one panel per node count) ───────────
// Reads the tl_curve.csv rows. `load` names the dimension the curve traces
// along — the offered load a saturation curve raises, either the worker count
// or the offered rate — so a sweep that varies either one gets a curve. Within
// a panel one curve per remaining dimension combination (benchmark, payload,
// buffers, stream mode) traces p50 latency against achieved throughput as the
// load rises; the shaded region is the p95–p99 tail band (it sits above the p50
// line by construction, not a symmetric error band). Curves are coloured by
// stream mode when several are compared, else by benchmark; the marker encodes
// the benchmark and the dash the remaining dimension combination.
// `group` selects the scale band to draw (see the Go-side banding); pass the
// band you want when the data was split, else 1.
#let tl-curve(data, group: 1, load: "workers", psize: (4.6, 3.1), x-step: auto, y-step: auto, cols: 3) = {
  let rows = data.filter(r => int(r.group) == group)
  if rows.len() == 0 { return [] }
  let benches = uniq(rows, "benchmark")
  let modes = uniq(rows, "stream_mode")
  // Curve identity is every dimension but the panel's (nodes) and the one the
  // curve traces along: points differing only in the traced dimension are
  // successive points of one curve.
  let fixed = dim-cols.filter(c => c != "nodes" and c != load)
  let varying-fixed = fixed.filter(c => uniq(rows, c).len() > 1)
  let tuples = fixed-tuples(rows, fixed)
  let keys = series-keys(rows, fixed)

  let node-vals = uniq-numsorted(rows, "nodes").filter(n => keys.any(k => (
    series-rows(rows.filter(r => r.nodes == n), k, fixed).len() > 1
  )))
  let ncols = gridcols(cols, node-vals.len())
  let mk-panel(i, n) = {
    let sub = rows.filter(r => r.nodes == n)
    panel(
      psize, "N = " + n,
      x-label: if show-x-label(i, node-vals.len(), ncols) { [Throughput (kops/s)] } else { none },
      y-label: if show-y-label(i, ncols) { [p50 (ms)] } else { none },
      x-min: 0, y-min: 0, x-tick-step: x-step, y-tick-step: y-step,
      {
        for key in keys {
          let pts = series-rows(sub, key, fixed).sorted(key: r => float(r.at(load)))
          if pts.len() > 1 {
            let paint = series-color(modes, benches, key)
            plot.add-fill-between(
              pts.map(r => (float(r.throughput_kops), float(r.p99_us) / 1000)),
              pts.map(r => (float(r.throughput_kops), float(r.p95_us) / 1000)),
              style: (fill: paint.transparentize(88%), stroke: none),
            )
            plot.add(
              pts.map(r => (float(r.throughput_kops), float(r.p50_us) / 1000)),
              mark: series-mark(benches, key), mark-size: 0.09,
              style: (stroke: (paint: paint, thickness: 1pt, dash: series-dash(tuples, key))),
              mark-style: (stroke: paint, fill: paint),
            )
          }
        }
      },
    )
  }

  let legend-items = series-legend(keys, modes, benches, tuples, fixed, varying-fixed)
  let panels = node-vals.enumerate().map(((i, n)) => mk-panel(i, n))
  figure-stack(legend-items, panels, ncols)
}

// ── per-node-cdf: latency CDF per node, one panel per run ────────────────────
// Reads the node_cdf.csv rows and draws one panel per entry of `runs`, each a
// (base, title) pair naming the run to select and the title its panel carries;
// a run measuring several benchmarks contributes one panel per benchmark. Within
// a panel there is one cumulative-probability curve per node, shaded light→dark
// in node order so a slow node stands out even when there are too many nodes to
// name in a legend. Unlike the sweep figures this works for a single run, since
// it compares nodes within one configuration rather than across the sweep.
#let per-node-cdf(data, runs, psize: (4.2, 2.7), cols: 3) = {
  let multi-bench = if data.len() == 0 { false } else { uniq(data, "benchmark").len() > 1 }
  let entries = ()
  for run in runs {
    let rows = data.filter(r => r.base == run.base)
    for bench in uniq(rows, "benchmark") {
      entries.push((
        title: if multi-bench { run.title + " " + bench } else { run.title },
        rows: rows.filter(r => r.benchmark == bench),
      ))
    }
  }
  if entries.len() == 0 { return [] }
  let ncols = gridcols(cols, entries.len())
  let mk-panel(i, entry) = {
    let nodes = uniq(entry.rows, "node")
    let widest = entry.rows.fold(0.0, (m, r) => calc.max(m, float(r.cdf_us) / 1000))
    panel(
      psize, entry.title,
      x-label: if show-x-label(i, entries.len(), ncols) { [Latency (ms)] } else { none },
      y-label: if show-y-label(i, ncols) { [Cumulative probability] } else { none },
      x-min: 0, y-min: 0, y-max: 1, y-tick-step: 0.2,
      x-tick-step: nice-step(widest, target: 4),
      {
        for (ni, nd) in nodes.enumerate() {
          let shade = 0.45 + 0.55 * (ni / calc.max(nodes.len() - 1, 1))
          let pts = entry.rows
            .filter(r => r.node == nd)
            .sorted(key: r => float(r.prob))
            .map(r => (float(r.cdf_us) / 1000, float(r.prob)))
          if pts.len() > 1 {
            plot.add(
              pts, mark: none,
              style: (stroke: (paint: rgb("#1f77b4").lighten((1 - shade) * 100%), thickness: 1pt)),
            )
          }
        }
      },
    )
  }
  grid(
    columns: ncols, gutter: 3mm,
    ..entries.enumerate().map(((i, entry)) => mk-panel(i, entry)),
  )
}

// ── time-series: one run's throughput and latency over time ──────────────────
// Reads a throughput CSV (offset_s, throughput_ops_s, phase, node) and a
// latency CSV (offset_s, mean_ns, …, node) for one benchmark, drawing one panel
// each with a line per node against wall-clock offset. `sat` adds the run's
// saturation curve (offered_rate, throughput_ops_s, …, node) as a third panel,
// but only for a run that ramped the offered rate: a run measured at one rate
// contributes a single point per node, which says nothing the sweep's
// throughput-vs-rate figure does not. Node colors come from the benchmark
// palette by first-seen order. Guards on no data, so a benchmark whose event
// stream held no interval events renders nothing instead of failing the
// compilation.
#let time-series(tput, lat, sat: (), legend: true, psize: (4.4, 2.9)) = {
  if tput.len() == 0 and lat.len() == 0 { return [] }
  let nodes = uniq(tput, "node")
  let paint(ni) = node-color(ni, nodes.len())
  let node-line(sub, nd, ni, yfn) = {
    let pts = sub.filter(r => r.node == nd).sorted(key: r => float(r.offset_s)).map(yfn)
    if pts.len() > 1 {
      plot.add(pts, mark: none, style: (stroke: paint(ni) + 1pt))
    }
  }
  let panels = (
    panel(
      psize, "Throughput over time",
      x-label: [Time (s)], y-label: [kops/s], x-min: 0, y-min: 0,
      {
        for (ni, nd) in nodes.enumerate() {
          node-line(tput, nd, ni, r => (float(r.offset_s), float(r.throughput_ops_s) / 1000))
        }
      },
    ),
    panel(
      psize, "Mean latency over time",
      x-label: [Time (s)], y-label: [mean (ms)], x-min: 0, y-min: 0,
      {
        for (ni, nd) in uniq(lat, "node").enumerate() {
          node-line(lat, nd, ni, r => (float(r.offset_s), float(r.mean_ns) / 1e6))
        }
      },
    ),
  )
  if uniq(sat, "offered_rate").len() > 1 {
    panels.push(panel(
      psize, "Saturation curve",
      x-label: [Offered rate (ops/s)], y-label: [kops/s achieved], x-min: 0, y-min: 0,
      {
        for (ni, nd) in uniq(sat, "node").enumerate() {
          let pts = sat
            .filter(r => r.node == nd)
            .sorted(key: r => float(r.offered_rate))
            .map(r => (float(r.offered_rate), float(r.throughput_ops_s) / 1000))
          if pts.len() > 0 {
            plot.add(
              pts, mark: "o", mark-size: 0.09,
              style: (stroke: (paint: paint(ni), thickness: 1pt)),
              mark-style: (stroke: paint(ni), fill: paint(ni)),
            )
          }
        }
      },
    ))
  }
  let key = if legend { node-items(nodes) } else { (items: (), caption: none) }
  figure-stack(key.items, panels, panels.len(), gutter: 3mm, caption: key.caption)
}

// ── heatmap: a grid of cells colored by a value column ───────────────────────
// rows are dictionaries; xcol/ycol name the grid axes and valuecol the color.
// Values are mapped through a red→yellow→green scale over [vmin, vmax]; set
// reverse when a high value is bad (e.g. a degraded fraction) so it reads red.
// A cell with no matching row is drawn grey. `label` names what the color
// means; it is printed beside the color scale below the grid, without which a
// uniformly green grid says nothing about the range it is uniform over.
#let rdylgn = (rgb("#d73027"), rgb("#fee08b"), rgb("#1a9850"))
#let heat-lerp(a, b, f) = color.mix((a, (1 - f) * 100%), (b, f * 100%))
#let heat-color(v, vmin, vmax, reverse) = {
  let t = if vmax == vmin { 0.5 } else { calc.max(0.0, calc.min(1.0, (v - vmin) / (vmax - vmin))) }
  if reverse { t = 1.0 - t }
  if t < 0.5 { heat-lerp(rdylgn.at(0), rdylgn.at(1), t * 2) } else {
    heat-lerp(rdylgn.at(1), rdylgn.at(2), (t - 0.5) * 2)
  }
}

// heat-scale is the heatmap's key: the color ramp from vmin to vmax, what the
// value means, and the grey no-data swatch when the grid has empty cells.
#let heat-scale(vmin, vmax, reverse, label, missing) = {
  set text(size: tick-size)
  let stops = if reverse { rdylgn.rev() } else { rdylgn }
  let ramp = box(
    baseline: 0.1em, width: 2.6cm, height: 0.7em,
    stroke: 0.4pt + luma(150), fill: gradient.linear(..stops),
  )
  let num(v) = str(calc.round(v, digits: 2))
  let parts = ([#num(vmin) #ramp #num(vmax)],)
  if label != none { parts.push(label) }
  if missing {
    parts.push([#box(
      baseline: 0.1em, width: 0.7em, height: 0.7em,
      fill: luma(210), stroke: 0.4pt + luma(150),
    ) no data])
  }
  block(inset: (y: 2pt), parts.join(h(1.4em)))
}

#let heatmap(
  rows, xcol: "col", ycol: "host", valuecol: "rel",
  vmin: 0.0, vmax: 1.25, reverse: false, cell: auto, label: none,
) = {
  if rows.len() == 0 { return [] }
  let xname = xcol
  let yname = ycol
  if uniq(rows, xname).len() < uniq(rows, yname).len() {
    let swap = xname
    xname = yname
    yname = swap
  }
  let xs = uniq(rows, xname)
  let ys = uniq(rows, yname)
  // Bound the grid on both axes: 15cm of columns and 20cm of rows keep a wide
  // or a tall grid on the page once fitwidth has scaled it.
  let cell-size = if cell == auto {
    calc.min(1.2, 15 / xs.len(), 20 / ys.len())
  } else { cell }
  let cells = (:)
  for r in rows {
    cells.insert(r.at(xname) + "\u{1f}" + r.at(yname), r)
  }
  // A rotated column label occupies about 1.5 line widths of the tick font, so
  // label every column the cells are wide enough for and every n-th otherwise.
  let label-step = calc.max(1, calc.ceil(1.5 * tick-size.cm() / cell-size))
  let grid-canvas = {
    set text(size: tick-size)
    cetz.canvas(length: 1cm, {
      import cetz.draw: rect, content
      for (yi, y) in ys.enumerate() {
        for (xi, x) in xs.enumerate() {
          let hit = cells.at(x + "\u{1f}" + y, default: none)
          let fill = if hit == none { luma(210) } else {
            heat-color(float(hit.at(valuecol)), vmin, vmax, reverse)
          }
          rect(
            (xi * cell-size, -yi * cell-size),
            ((xi + 1) * cell-size, -(yi + 1) * cell-size),
            fill: fill, stroke: 0.5pt + white,
          )
        }
        content((-0.08, -(yi + 0.5) * cell-size), text(size: tick-size, str(y)), anchor: "east")
      }
      // Column labels sit wholly above the grid (anchor south), so a long
      // label extends upward instead of over the top row of cells.
      for (xi, x) in xs.enumerate() {
        if calc.rem(xi, label-step) == 0 {
          content(
            ((xi + 0.5) * cell-size, 0.08),
            rotate(-90deg, reflow: true, text(size: tick-size, str(x))),
            anchor: "south",
          )
        }
      }
    })
  }
  stack(
    spacing: 2mm,
    grid-canvas,
    heat-scale(vmin, vmax, reverse, label, cells.len() < xs.len() * ys.len()),
  )
}

// ── offset-cdf: empirical CDF of clock offset and drift ──────────────────────
// Reads the offsets CSV (metric, group, value_us, cdf); one panel per metric,
// one curve per group (the whole cluster and each node count).
#let offset-cdf(rows, psize: (4.6, 3.1)) = {
  let groups = uniq(rows, "group")
  let mk-panel(i, metric, mlabel) = {
    let sub = rows.filter(r => r.metric == metric)
    panel(
      psize, mlabel,
      x-label: [µs], y-label: if show-y-label(i, 2) { [cumulative probability] } else { none },
      x-min: 0, y-min: 0, y-max: 1,
      {
        for (gi, g) in groups.enumerate() {
          let pts = sub.filter(r => r.group == g).sorted(key: r => float(r.value_us)).map(r => (float(r.value_us), float(r.cdf)))
          if pts.len() > 1 {
            plot.add(pts, mark: none, style: (stroke: bench-palette.at(calc.rem(gi, bench-palette.len())) + 1pt))
          }
        }
      },
    )
  }
  figure-stack(
    groups.enumerate().map(((gi, g)) => (bench-palette.at(calc.rem(gi, bench-palette.len())), g)),
    (mk-panel(0, "offset", [Absolute offset]), mk-panel(1, "drift", [Residual drift])),
    2, gutter: 3mm,
  )
}

// ── ratio-vs: dedup/dual (non-baseline/baseline) metric ratio ────────────────
// Reads the comparison CSV; plots <metric>_ratio against xcol (a dimension
// column, "workers" by default) with a dashed reference line at 1.0 (parity),
// one line per benchmark plus every other held-fixed dimension, one panel per
// value of facet ("payload" by default). Above 1.0 the non-baseline mode is
// larger. xcol/facet mirror metric-vs's x-role split (fixed-dims computes the
// held-fixed series dimensions the same way); the series key itself stays
// mode-less, since a comparison row already pivots both modes into one row.
#let n-color(i) = bench-palette.at(calc.rem(i, bench-palette.len()))

#let ratio-vs(
  rows, ratiocol, xcol: "workers", facet: "payload",
  ylabel: [ratio], xlabel: none, facet-label: none,
  y-min: 0.8, y-max: 1.3, y-step: 0.1, psize: (4.6, 3.1), cols: 3,
) = {
  let benches = uniq(rows, "benchmark")
  let fixed = fixed-dims(xcol, facet)
  let varying-fixed = fixed.filter(c => uniq(rows, c).len() > 1)
  let facet-vals = if facet == none { (none,) } else { uniq-numsorted(rows, facet) }
  let ncols = gridcols(cols, facet-vals.len())
  let series = ()
  let series-key(r) = (bench: r.benchmark, fixed: fixed.map(c => r.at(c)))
  for r in rows {
    let k = series-key(r)
    if k not in series { series.push(k) }
  }
  let label-of(k) = series-label(k, fixed, varying-fixed, benches.len() > 1, false)
  let mk-panel(i, fval) = {
    let sub = rows.filter(r => (facet == none or r.at(facet) == fval) and r.at(ratiocol) != "")
    let xvals = uniq-numsorted(sub, xcol)
    let title = if facet == none { none } else {
      if facet-label == none { facet + " = " + fval } else { facet-label + " = " + fval }
    }
    panel(
      psize, title,
      x-label: if show-x-label(i, facet-vals.len(), ncols) { xlabel } else { none },
      y-label: if show-y-label(i, ncols) { ylabel } else { none },
      x-ticks: xvals.enumerate().map(((xi, v)) => (xi, raw(v))), x-tick-step: none,
      x-min: -0.3, x-max: xvals.len() - 0.7, y-min: y-min, y-max: y-max, y-tick-step: y-step,
      {
        plot.add-hline(1.0, style: (stroke: (dash: "dashed", paint: gray, thickness: 0.7pt)))
        for (si, key) in series.enumerate() {
          let paint = n-color(si)
          let pts = xvals.enumerate().map(((xi, xv)) => {
            let hit = sub.find(r => r.at(xcol) == xv and series-key(r) == key)
            if hit == none or hit.at(ratiocol) == "" { none } else { (xi, float(hit.at(ratiocol))) }
          }).filter(x => x != none)
          if pts.len() > 1 {
            plot.add(pts, mark: "o", mark-size: 0.09, style: (stroke: paint + 1pt), mark-style: (stroke: paint, fill: paint))
          }
        }
      },
    )
  }
  figure-stack(
    series.enumerate().map(((si, key)) => (n-color(si), label-of(key))),
    facet-vals.enumerate().map(((i, fval)) => mk-panel(i, fval)),
    ncols,
  )
}

// ── run-status table: run outcomes per node count ────────────────────────────
// Reads the run-status CSV (nodes, total, succeeded, degraded, failed, …) and
// renders a compact table; degraded/failed cells are tinted when non-zero.
#let run-status-table(rows) = {
  set text(size: 8pt)
  let tinted(v, paint) = if int(v) > 0 { table.cell(fill: paint.lighten(70%), str(v)) } else { str(v) }
  table(
    columns: 5, align: right, inset: (x: 4pt, y: 2.5pt), stroke: 0.4pt + luma(200),
    table.header([N], [total], [succeeded], [degraded], [failed]),
    ..rows.map(r => (
      str(r.nodes), str(r.total), str(r.succeeded),
      tinted(r.degraded, rgb("#f58518")), tinted(r.failed, rgb("#d62728")),
    )).flatten(),
  )
}
