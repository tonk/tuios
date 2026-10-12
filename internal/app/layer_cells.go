package app

import (
	"image"
	"slices"

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
// measures it for the compositor bounds, and, once the layer has been composed
// a second time, the cell writes uv.StyledString makes to draw it. Replaying
// the writes onto the canvas is what drawing the layer did, without segmenting
// graphemes or parsing ANSI again.
type layerCells struct {
	width, height int
	decoded       bool
	writes        []cellWrite
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
// first; composeLayers redoes that clear itself, so they are dropped here.
func (r *cellRecorder) SetCell(x, y int, c *uv.Cell) {
	if c == nil {
		return
	}
	r.writes = append(r.writes, cellWrite{x: x, y: y, cell: *c})
}

// measureLayer sizes a layer as lipgloss's Compositor does for its bounds.
func measureLayer(layer *lipgloss.Layer) *layerCells {
	content := layer.GetContent()
	return &layerCells{
		width:  lipgloss.Width(content),
		height: lipgloss.Height(content),
	}
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
}

// composeLayers draws layers onto the canvas in z order, as
// canvas.Compose(lipgloss.NewCompositor(layers...)) does, but reuses each
// layer's decoded cells across frames. A window's CachedLayer survives every
// frame its content does not change, so a busy pane no longer makes every idle
// pane's content be measured and parsed again.
//
// A layer is drawn directly the first time it is composed and decoded only
// when it comes back the next frame. Most layers are built fresh every frame
// (a pane with new output, the sidebar, the dock, overlays) and are never
// reused, so recording them would cost more than drawing them.
//
// Layer content is immutable once built (lipgloss has no setter for it), so the
// layer pointer is the cache key. Position is applied at replay time, so a
// layer moved with X/Y keeps its cells. Entries for layers not drawn this frame
// are dropped.
func (m *OS) composeLayers(canvas *lipgloss.Canvas, layers []*lipgloss.Layer) {
	if m.layerCells == nil {
		m.layerCells = make(map[*lipgloss.Layer]*layerCells)
		m.layerCellsNext = make(map[*lipgloss.Layer]*layerCells)
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
	method := canvas.WidthMethod()
	for _, layer := range sorted {
		lc, ok := m.layerCellsNext[layer]
		if !ok {
			if lc, ok = m.layerCells[layer]; !ok {
				lc = measureLayer(layer)
			} else if !lc.decoded {
				lc.decode(layer, method)
			}
			m.layerCellsNext[layer] = lc
		}

		x, y := layer.GetX(), layer.GetY()
		area := image.Rect(x, y, x+lc.width, y+lc.height)
		if !area.Overlaps(screen) {
			continue
		}

		// Clear the layer's area first, as StyledString.Draw does, so the
		// blank cells of a layer cover whatever lies beneath it. Writes off the
		// canvas are no-ops on it, so only the visible part is touched.
		if !lc.decoded {
			layer.Draw(canvas, area)
			continue
		}

		visible := area.Intersect(screen)
		for cy := visible.Min.Y; cy < visible.Max.Y; cy++ {
			for cx := visible.Min.X; cx < visible.Max.X; cx++ {
				canvas.SetCell(cx, cy, nil)
			}
		}
		for i := range lc.writes {
			w := &lc.writes[i]
			canvas.SetCell(w.x+x, w.y+y, &w.cell)
		}
	}

	// What was drawn this frame is next frame's cache; the rest is released.
	clear(m.layerCells)
	m.layerCells, m.layerCellsNext = m.layerCellsNext, m.layerCells
	clear(m.composeScratch)
}
