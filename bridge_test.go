// Copyright (c) 2026 the go-widgets/tui authors. All rights reserved.
// Use of this source code is governed by a BSD-3-Clause license that can be
// found in the LICENSE file at the root of this repository.

package tui

import (
	"bytes"
	"errors"
	"testing"

	"github.com/go-widgets/toolkit"
)

// factory names a toolkit.Widget kind and how to construct it in a
// pre-sized rect. Kept small on purpose — one representative per Draw
// codepath (no need to enumerate every constructor to catch a cell-
// backend regression; if Button renders, all button variants render).
type factory struct {
	name string
	make func() toolkit.Widget
}

// widgetFactories covers at least one widget from each of the three
// v0.7 / v0.8 / v0.9 additive waves plus the pre-v0.7 core. Anything
// with a distinctive Draw path (double-stroked Value in Stat, dashed
// border in DropZone, marker-per-kind in Timeline, per-line tint in
// Diff, …) is listed so a cell-backend regression trips its case
// rather than sailing through under a shared happy-path.
func widgetFactories() []factory {
	return []factory{
		// pre-v0.7 core — sanity anchors
		{"button", func() toolkit.Widget {
			w := toolkit.NewButton("OK", nil)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3})
			return w
		}},
		{"label", func() toolkit.Widget {
			w := &toolkit.Label{Text: "hello"}
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 1})
			return w
		}},
		{"progressbar", func() toolkit.Widget {
			w := toolkit.NewProgressBar()
			w.Fraction = 0.5
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
			return w
		}},

		// v0.7 — GTK-alignment wave
		{"switch", func() toolkit.Widget {
			w := toolkit.NewSwitch(true)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
			return w
		}},
		{"badge", func() toolkit.Widget {
			w := toolkit.NewBadge("9")
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 3})
			return w
		}},
		{"kbd", func() toolkit.Widget {
			w := toolkit.NewKbd("Ctrl+K")
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 12, H: 3})
			return w
		}},
		{"alert", func() toolkit.Widget {
			w := toolkit.NewAlert("saved", toolkit.AlertSuccess)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 5})
			return w
		}},
		{"card", func() toolkit.Widget {
			w := toolkit.NewCard("Title", "line one\nline two", "footer")
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 12})
			return w
		}},
		{"breadcrumbs", func() toolkit.Widget {
			w := toolkit.NewBreadcrumbs([]string{"a", "b", "c"})
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 3})
			return w
		}},
		{"steps", func() toolkit.Widget {
			w := toolkit.NewSteps([]string{"one", "two", "three"}, 1)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 5})
			return w
		}},
		{"headerbar", func() toolkit.Widget {
			w := toolkit.NewHeaderBar("Title")
			w.Subtitle = "sub"
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 4})
			return w
		}},
		{"table", func() toolkit.Widget {
			cols := []toolkit.TableColumn{{Title: "A", Width: 10}, {Title: "B"}}
			w := toolkit.NewTable(cols, [][]string{{"a1", "b1"}, {"a2", "b2"}})
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 8})
			return w
		}},

		// v0.8 — DaisyUI / libadwaita wave
		{"avatar", func() toolkit.Widget {
			w := toolkit.NewAvatar("DL")
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 6, H: 3})
			return w
		}},
		{"skeleton", func() toolkit.Widget {
			w := toolkit.NewSkeleton(toolkit.SkeletonText, 3)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 8})
			return w
		}},
		{"rating", func() toolkit.Widget {
			w := toolkit.NewRating(3, 5)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3})
			return w
		}},
		{"toast", func() toolkit.Widget {
			w := toolkit.NewToast("saved", toolkit.ToastSuccess)
			w.Visible = true
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
			return w
		}},
		{"banner", func() toolkit.Widget {
			w := toolkit.NewBanner("update ready")
			w.ButtonLabel = "Install"
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 3})
			return w
		}},
		{"popover", func() toolkit.Widget {
			child := toolkit.NewLabel("child")
			child.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3})
			w := toolkit.NewPopover(child)
			w.Title = "Menu"
			w.Visible = true
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 8})
			return w
		}},
		{"actionrow", func() toolkit.Widget {
			w := toolkit.NewActionRow("Language")
			w.Subtitle = "English"
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 40, H: 5})
			return w
		}},
		{"viewswitcher", func() toolkit.Widget {
			w := toolkit.NewViewSwitcher([]string{"A", "B", "C"}, 0)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
			return w
		}},
		{"chatbubble", func() toolkit.Widget {
			w := toolkit.NewChatBubble("hello", toolkit.ChatFromUser)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
			return w
		}},
		{"searchentry", func() toolkit.Widget {
			w := toolkit.NewSearchEntry("query")
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
			return w
		}},
		{"diff", func() toolkit.Widget {
			w := toolkit.NewDiff([]toolkit.DiffLine{
				{Text: "old", Kind: toolkit.DiffRemoved},
				{Text: "new", Kind: toolkit.DiffAdded},
				{Text: "ctx", Kind: toolkit.DiffContext},
			})
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 6})
			return w
		}},
		{"pagination", func() toolkit.Widget {
			w := toolkit.NewPagination(2, 5)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
			return w
		}},

		// v0.9 — finishing wave
		{"splitbutton", func() toolkit.Widget {
			w := toolkit.NewSplitButton("Deploy", nil)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 25, H: 3})
			return w
		}},
		{"iconbutton", func() toolkit.Widget {
			w := toolkit.NewIconButton("+", nil)
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 5, H: 3})
			return w
		}},
		{"stat", func() toolkit.Widget {
			w := toolkit.NewStat("Requests", "12,845")
			w.Change = "+8%"
			w.Trend = toolkit.StatUp
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 8})
			return w
		}},
		{"timeline", func() toolkit.Widget {
			w := toolkit.NewTimeline([]toolkit.TimelineEvent{
				{Title: "opened", Kind: toolkit.TimelineDefault},
				{Title: "reviewed", Detail: "LGTM", Kind: toolkit.TimelineSuccess},
			})
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 12})
			return w
		}},
		{"dropzone", func() toolkit.Widget {
			w := toolkit.NewDropZone("Drop here")
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 10})
			return w
		}},
		{"chip", func() toolkit.Widget {
			w := toolkit.NewChip("frontend")
			w.Closable = true
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 20, H: 3})
			return w
		}},
		{"formfield", func() toolkit.Widget {
			e := toolkit.NewEntry("value")
			e.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 3})
			w := toolkit.NewFormField("Name", e)
			w.Help = "min 3 chars"
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 30, H: 8})
			return w
		}},
		{"progresscircle", func() toolkit.Widget {
			w := toolkit.NewProgressCircle()
			w.Fraction = 0.66
			w.SetBounds(toolkit.Rect{X: 0, Y: 0, W: 8, H: 4})
			return w
		}},
	}
}

// TestRenderToolkitEveryWidgetSurvivesCellBackend asserts that every
// widget from the three additive waves can be rendered through the
// cell backend without panicking and produces a non-empty ANSI stream.
// The claim under test is "one Painter interface renders everywhere":
// a toolkit widget that draws pixels for the pixel backend must also
// produce cells for the cell backend.
func TestRenderToolkitEveryWidgetSurvivesCellBackend(t *testing.T) {
	for _, f := range widgetFactories() {
		t.Run(f.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := RenderToolkitSized(&buf, 60, 20, []toolkit.Widget{f.make()}, toolkit.DefaultLight())
			if err != nil {
				t.Fatalf("RenderToolkitSized: %v", err)
			}
			if buf.Len() == 0 {
				t.Fatal("ANSI stream is empty — widget likely produced no cells")
			}
		})
	}
}

// TestRenderToolkitNilThemeUsesDefault verifies the nil-theme
// convenience: passing nil should not panic and should fall through
// to toolkit.DefaultLight() so the CLI form
// `tui.RenderToolkit(os.Stdout, widgets, nil)` works.
func TestRenderToolkitNilThemeUsesDefault(t *testing.T) {
	var buf bytes.Buffer
	widgets := []toolkit.Widget{&toolkit.Label{Text: "x"}}
	widgets[0].SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	if err := RenderToolkitSized(&buf, 20, 5, widgets, nil); err != nil {
		t.Fatalf("RenderToolkitSized nil theme: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("nil-theme render produced empty output")
	}
}

// TestRenderToolkitNonPositiveDimsFallBackToDefaults verifies that
// zero / negative cols or rows are replaced by DefaultCols / DefaultRows
// rather than producing an empty grid.
func TestRenderToolkitNonPositiveDimsFallBackToDefaults(t *testing.T) {
	var buf bytes.Buffer
	widgets := []toolkit.Widget{&toolkit.Label{Text: "x"}}
	widgets[0].SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	// cols = 0, rows = -1 should both fall through to defaults.
	if err := RenderToolkitSized(&buf, 0, -1, widgets, nil); err != nil {
		t.Fatalf("RenderToolkitSized non-positive dims: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("non-positive-dim render produced empty output")
	}
}

// TestRenderToolkitEnvSize covers the RenderToolkit path (as opposed
// to RenderToolkitSized) — it queries the environment via
// SizeOrDefault. Setting COLUMNS + LINES so SizeOrDefault returns a
// reasonable grid; verifying no panic + non-empty output.
func TestRenderToolkitEnvSize(t *testing.T) {
	t.Setenv("COLUMNS", "40")
	t.Setenv("LINES", "12")
	var buf bytes.Buffer
	widgets := []toolkit.Widget{&toolkit.Label{Text: "x"}}
	widgets[0].SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	if err := RenderToolkit(&buf, widgets, nil); err != nil {
		t.Fatalf("RenderToolkit: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("env-size render produced empty output")
	}
}

// toolkitErrWriter always fails on Write, exercising the WriteANSI
// error branch of RenderToolkitSized. Named distinctly from
// render_test.go's errWriter to avoid a same-package collision.
type toolkitErrWriter struct{}

func (toolkitErrWriter) Write([]byte) (int, error) { return 0, errToolkitBoom }

var errToolkitBoom = errors.New("boom")

// TestRenderToolkitSizedWriteError verifies the io.Writer error path
// is wrapped and returned rather than dropped.
func TestRenderToolkitSizedWriteError(t *testing.T) {
	widgets := []toolkit.Widget{&toolkit.Label{Text: "x"}}
	widgets[0].SetBounds(toolkit.Rect{X: 0, Y: 0, W: 10, H: 1})
	err := RenderToolkitSized(toolkitErrWriter{}, 20, 5, widgets, toolkit.DefaultLight())
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !errors.Is(err, errToolkitBoom) {
		t.Fatalf("expected wrapped errToolkitBoom, got %v", err)
	}
}

// TestToPainterRGBAAllFields verifies each byte-field flows through
// the RGBA type conversion. The helper is trivial but coverage-
// mandatory, and defends against a future edit that dropped a field.
func TestToPainterRGBAAllFields(t *testing.T) {
	got := toPainterRGBA(toolkit.RGBA{R: 1, G: 2, B: 3, A: 4})
	if got.R != 1 || got.G != 2 || got.B != 3 || got.A != 4 {
		t.Fatalf("toPainterRGBA: got %+v", got)
	}
}
