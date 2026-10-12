package app

import (
	"image"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// cellWrite is one SetCell a layer's content makes, relative to the layer's
// top-left corner.
type cellWrite struct {
	x, y int
	cell uv.Cell
}

// layerCells is what composeLayers knows about a layer: its size, as lipgloss
// measures it for the compositor bounds, where the last frame drew it, and,
// once the layer has to be drawn again, the cell writes uv.StyledString makes
// to draw it. Replaying the writes onto the canvas is what drawing the layer
// did, without segmenting graphemes or parsing ANSI again.
type layerCells struct {
	width, height int
	decoded       bool
	// writes holds the decoded cell writes grouped by row, in draw order
	// within a row. writes[rowIdx[i]:rowIdx[i+1]] lie on layer row rowOff+i.
	// Rows are independent on a uv buffer (Line.Set touches only its own
	// line), so a row's writes can be replayed without the others.
	writes []cellWrite
	rowOff int
	rowIdx []int
	// starts marks, row-major over width x height, the positions where a
	// write begins a cell. Replay clears only the other positions: a write
	// replaces its cell anyway, and Line.Set breaks up a wide cell beneath it
	// just as the clear would. The second column of a wide write is not a
	// start, so it is still cleared first, which is what breaks up a wide cell
	// beneath that column.
	starts []bool

	// x, y and order are where the last compose drew the layer and its index
	// in that frame's z order. fresh marks a layer first composed this frame.
	// gen is the compose that last drew it, and content its content then.
	x, y, order int
	fresh       bool
	gen         uint64
	content     string
}

// cellRecorder is a uv.Screen that records the cells drawn on it instead of
// storing them, so StyledString.Draw can decode a layer once into a replayable
// list. StyledString only writes; it never reads the screen back.
type cellRecorder struct {
	method uv.WidthMethod
	area   uv.Rectangle
	writes []cellWrite
}

func (r *cellRecorder) Bounds() uv.Rectangle        { return r.area }
func (r *cellRecorder) CellAt(int, int) *uv.Cell    { return nil }
func (r *cellRecorder) WidthMethod() uv.WidthMethod { return r.method }

// SetCell records a write. Nil writes are the area clear StyledString.Draw does
// first; replay redoes that clear itself, so they are dropped here.
func (r *cellRecorder) SetCell(x, y int, c *uv.Cell) {
	if c == nil {
		return
	}
	r.writes = append(r.writes, cellWrite{x: x, y: y, cell: *c})
}

// measureLayer sizes a layer as lipgloss's Compositor does for its bounds.
// NewLayer measured the content already; a layer without children (tuios
// never adds any) is exactly its content's size.
func measureLayer(layer *lipgloss.Layer) *layerCells {
	return &layerCells{width: layer.Width(), height: layer.Height()}
}

// decode records the cell writes Layer.Draw makes for the layer's content.
func (lc *layerCells) decode(layer *lipgloss.Layer, method uv.WidthMethod) {
	lc.decoded = true
	if lc.width == 0 {
		return
	}
	rec := &cellRecorder{
		method: method,
		area:   uv.Rect(0, 0, lc.width, lc.height),
		writes: make([]cellWrite, 0, lc.width*lc.height),
	}
	uv.NewStyledString(layer.GetContent()).Draw(rec, rec.area)
	lc.writes = rec.writes
	lc.starts = make([]bool, lc.width*lc.height)
	if len(lc.writes) == 0 {
		return
	}

	// StyledString writes top to bottom already, except a trailing
	// zero-width cluster it flushes wherever printing stopped; the stable
	// sort keeps each row's own order.
	slices.SortStableFunc(lc.writes, func(a, b cellWrite) int { return a.y - b.y })
	first, last := lc.writes[0].y, lc.writes[len(lc.writes)-1].y
	lc.rowOff = first
	lc.rowIdx = make([]int, last-first+2)
	i := 0
	for r := first; r <= last; r++ {
		lc.rowIdx[r-first] = i
		for i < len(lc.writes) && lc.writes[i].y == r {
			i++
		}
	}
	lc.rowIdx[last-first+1] = len(lc.writes)

	for _, w := range lc.writes {
		if w.x >= 0 && w.x < lc.width && w.y >= 0 && w.y < lc.height {
			lc.starts[w.y*lc.width+w.x] = true
		}
	}
}

// rowSpan returns the layer rows, relative to its top, that drawing it can
// change: its area, and any row a decoded write lies on outside it.
// StyledString writes within the area's rows; a trailing zero-width cluster
// it flushes lands on the last row, just past the right edge at worst.
func (lc *layerCells) rowSpan() (lo, hi int) {
	lo, hi = 0, lc.height
	if len(lc.rowIdx) > 0 {
		lo = min(lo, lc.rowOff)
		hi = max(hi, lc.rowOff+len(lc.rowIdx)-1)
	}
	return lo, hi
}

// markRows marks the canvas rows a layer at row y covers as damaged.
func markRows(damage []bool, y int, lc *layerCells) {
	lo, hi := lc.rowSpan()
	for r := max(y+lo, 0); r < min(y+hi, len(damage)); r++ {
		damage[r] = true
	}
}

// replay draws the layer, decoded, at (x, y) on the damaged canvas rows only,
// clipped to the screen.
func (lc *layerCells) replay(canvas *lipgloss.Canvas, x, y int, damage []bool, screen image.Rectangle) {
	visX0, visX1 := max(x, screen.Min.X), min(x+lc.width, screen.Max.X)
	lo, hi := lc.rowSpan()
	for r := lo; r < hi; r++ {
		cy := y + r
		if cy < 0 || cy >= len(damage) || !damage[cy] {
			continue
		}
		// Clear the layer's part of the row first, as StyledString.Draw
		// does, so its blank cells cover whatever lies beneath it.
		if r >= 0 && r < lc.height {
			rowStart := r*lc.width - x
			for cx := visX0; cx < visX1; cx++ {
				if !lc.starts[rowStart+cx] {
					canvas.SetCell(cx, cy, nil)
				}
			}
		}
		if i := r - lc.rowOff; i >= 0 && i+1 < len(lc.rowIdx) {
			for j := lc.rowIdx[i]; j < lc.rowIdx[i+1]; j++ {
				w := &lc.writes[j]
				canvas.SetCell(w.x+x, cy, &w.cell)
			}
		}
	}
}

// composeLayers draws layers onto the canvas in z order, leaving it exactly as
// canvas.Clear() followed by canvas.Compose(lipgloss.NewCompositor(layers...))
// would, but it only redraws the canvas rows that changed since the last call.
//
// The canvas keeps the previous frame. Comparing this frame's layers with the
// last one's gives the damaged rows: those of a layer that is new, gone or
// moved. Only those rows are cleared and recomposed, from every layer that
// crosses them, in z order. Rows are independent on a uv buffer, so a row
// recomposed this way comes out as a full compose would draw it. A busy pane
// so costs its own rows, not the whole screen.
//
// Layer content is immutable once built (lipgloss has no setter for it), so the
// layer pointer identifies unchanged content; a window's CachedLayer survives
// every frame its content does not change. A layer is drawn directly the first
// time it is composed, which damages all its rows anyway. It is decoded into
// cell writes only when it has to be redrawn later, so the layers rebuilt every
// frame (a pane with new output, the sidebar, the dock, overlays) are never
// recorded. Entries for layers not drawn this frame are dropped.
//
// The whole canvas is recomposed when it is new or resized, when two layers
// that stayed swap z order, or when a layer is passed twice. composeLayers
// owns the canvas contents between calls: anything else drawing on it would
// go unseen.
func (m *OS) composeLayers(canvas *lipgloss.Canvas, layers []*lipgloss.Layer) {
	if m.layerCells == nil {
		m.layerCells = make(map[*lipgloss.Layer]*layerCells)
		m.layerCellsNext = make(map[*lipgloss.Layer]*layerCells)
		m.layersByID = make(map[string]*layerCells)
		m.layersByIDNext = make(map[string]*layerCells)
		m.layersByContent = make(map[string]*layerCells)
		m.layersByContentNext = make(map[string]*layerCells)
	}

	// The compositor sorted by z with an unstable sort; for the short lists
	// it sees that is an insertion sort, which keeps equal z in append order.
	// A stable sort keeps that order for any length.
	sorted := append(m.composeScratch[:0], layers...)
	slices.SortStableFunc(sorted, func(a, b *lipgloss.Layer) int {
		return a.GetZ() - b.GetZ()
	})
	m.composeScratch = sorted

	screen := canvas.Bounds()
	full := canvas != m.composedCanvas || screen != m.composedBounds
	m.composedCanvas, m.composedBounds = canvas, screen
	damage := m.damageRows[:0]
	damage = append(damage, make([]bool, screen.Dy())...)
	m.damageRows = damage

	m.composeGen++
	gen := m.composeGen
	lastOrder := -1
	for i, layer := range sorted {
		if _, dup := m.layerCellsNext[layer]; dup {
			full = true
			continue
		}
		x, y := layer.GetX(), layer.GetY()
		lc, ok := m.layerCells[layer]
		if ok && lc.gen == gen {
			// A layer rebuilt in its place this frame already took its
			// cells; this one is drawn as new.
			ok = false
		}
		if !ok {
			lc, ok = m.matchRebuiltLayer(layer, gen)
		}
		if !ok {
			lc = measureLayer(layer)
			lc.fresh = true
			markRows(damage, y, lc)
		} else {
			lc.fresh = false
			if lc.order < lastOrder {
				full = true
			}
			lastOrder = lc.order
			if lc.x != x || lc.y != y {
				markRows(damage, lc.y, lc)
				markRows(damage, y, lc)
			}
		}
		lc.x, lc.y, lc.order, lc.gen, lc.content = x, y, i, gen, layer.GetContent()
		m.layerCellsNext[layer] = lc
		if id := layer.GetID(); id != "" {
			m.layersByIDNext[id] = lc
		} else {
			m.layersByContentNext[lc.content] = lc
		}
	}
	for _, lc := range m.layerCells {
		if lc.gen != gen {
			markRows(damage, lc.y, lc)
		}
	}

	// Clearing a row cell by cell goes through SetCell's compare and
	// wide-cell bookkeeping, which costs about what replaying a clean row
	// does. Once most rows are damaged, as when every pane changed, clearing
	// the whole canvas and recomposing it is the cheaper way.
	damaged := 0
	for _, d := range damage {
		if d {
			damaged++
		}
	}
	if full || 2*damaged > len(damage) {
		canvas.Clear()
		for i := range damage {
			damage[i] = true
		}
	} else {
		for cy, d := range damage {
			if d {
				for cx := screen.Min.X; cx < screen.Max.X; cx++ {
					canvas.SetCell(cx, cy, nil)
				}
			}
		}
	}

	method := canvas.WidthMethod()
	for _, layer := range sorted {
		lc := m.layerCellsNext[layer]
		area := image.Rect(lc.x, lc.y, lc.x+lc.width, lc.y+lc.height)
		if !area.Overlaps(screen) {
			continue
		}
		if lc.fresh {
			layer.Draw(canvas, area)
			continue
		}
		lo, hi := lc.rowSpan()
		if !slices.Contains(damage[max(lc.y+lo, 0):max(min(lc.y+hi, len(damage)), 0)], true) {
			continue
		}
		if !lc.decoded {
			lc.decode(layer, method)
		}
		lc.replay(canvas, lc.x, lc.y, damage, screen)
	}

	// Rows changed here have to be rendered again by renderComposed.
	if len(m.rowDirty) != len(damage) {
		m.rowDirty = make([]bool, len(damage))
		m.rowStrings = make([]string, len(damage))
		for i := range m.rowDirty {
			m.rowDirty[i] = true
		}
	}
	for i, d := range damage {
		m.rowDirty[i] = m.rowDirty[i] || d
	}

	// What was drawn this frame is next frame's cache; the rest is released.
	clear(m.layerCells)
	m.layerCells, m.layerCellsNext = m.layerCellsNext, m.layerCells
	clear(m.layersByID)
	m.layersByID, m.layersByIDNext = m.layersByIDNext, m.layersByID
	clear(m.layersByContent)
	m.layersByContent, m.layersByContentNext = m.layersByContentNext, m.layersByContent
	clear(m.composeScratch)
}

// matchRebuiltLayer finds the layer last frame drew that layer stands in for:
// the dock, the sidebar, scrollbars, separators and overlays are built anew
// every frame, as a new layer with, most frames, the same content. One with
// the same ID, or with no ID and the same content, and the same content
// either way, is that layer again: it keeps its cells and, unless it moved,
// damages nothing. Each old layer stands in for one new layer at most.
func (m *OS) matchRebuiltLayer(layer *lipgloss.Layer, gen uint64) (*layerCells, bool) {
	content := layer.GetContent()
	var lc *layerCells
	if id := layer.GetID(); id != "" {
		lc = m.layersByID[id]
	} else {
		lc = m.layersByContent[content]
	}
	if lc == nil || lc.gen == gen || lc.content != content {
		return nil, false
	}
	return lc, true
}

// renderComposed returns canvas.Render() for the canvas composeLayers last
// drew, rendering again only the rows that changed since the previous call.
// Each row renders on its own (uv starts every line with a reset pen), so
// joining cached rows gives the same string as rendering the whole canvas.
func (m *OS) renderComposed(canvas *lipgloss.Canvas) string {
	if canvas != m.composedCanvas || len(m.rowDirty) != canvas.Height() {
		return canvas.Render()
	}
	width := canvas.Width()
	if cap(m.rowScratch) < width {
		m.rowScratch = make(uv.Line, width)
	}
	line := m.rowScratch[:width]
	changed := m.frameString == "" && len(m.rowDirty) > 0
	for y, dirty := range m.rowDirty {
		if !dirty {
			continue
		}
		for x := range line {
			line[x] = *canvas.CellAt(x, y)
		}
		m.rowStrings[y] = uv.Lines{line}.Render()
		m.rowDirty[y] = false
		changed = true
	}
	if changed {
		m.frameString = strings.Join(m.rowStrings, "\n")
	}
	return m.frameString
}
