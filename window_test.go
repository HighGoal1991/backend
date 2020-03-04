// Copyright 2013 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package backend

import (
	"path/filepath"
	"testing"
)

func TestWindowNewFile(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	if len(w.Views()) != 1 {
		t.Errorf("Expected 1 view, but got %d", len(w.Views()))
	}
}

func TestWindowRemove(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v0 := w.NewFile()
	defer v0.Close()

	v1 := w.NewFile()
	v2 := w.NewFile()
	defer v2.Close()

	l := len(w.Views())

	w.remove(v1)
	if len(w.Views()) != l-1 {
		t.Errorf("Expected %d open views, but got %d", l-1, len(w.Views()))
	}
}

func TestWindowActiveView(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v0 := w.NewFile()
	defer v0.Close()

	v1 := w.NewFile()
	defer v1.Close()

	if w.ActiveView() != v1 {
		t.Error("Expected v1 to be the active view, but it wasn't")
	}
}

func TestWindowClose(t *testing.T) {
	ed := GetEditor()
	l := len(ed.Windows())
	w := ed.NewWindow()

	for _, v := range w.Views() {
		v.SetScratch(true)
		v.Close()
	}

	w.Close()

	if len(ed.Windows()) != l {
		t.Errorf("Expected window to close, but we have %d still open", len(ed.Windows()))
	}
}

func TestWindowCloseFail(t *testing.T) {
	ed := GetEditor()
	ed.SetFrontend(&dummyFrontend{})
	fe := ed.Frontend()
	if dfe, ok := fe.(*dummyFrontend); ok {
		dfe.SetDefaultAction(false)
	}

	w := ed.NewWindow()
	l := len(ed.Windows())

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	edit := v.BeginEdit()
	v.Insert(edit, 0, "test")
	v.EndEdit(edit)

	if w.Close() {
		t.Errorf("Expected window to fail to close, but it didn't")
	}

	if len(ed.Windows()) != l {
		t.Error("Expected window not to close, but it did")
	}
}

func TestWindowCloseAllViews(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	w.NewFile()
	w.NewFile()

	w.CloseAllViews()

	if len(w.Views()) != 0 {
		t.Errorf("Expected 0 open views, but got %d", len(w.Views()))
	}
}

func TestWindowCloseAllViewsFail(t *testing.T) {
	ed := GetEditor()
	ed.SetFrontend(&dummyFrontend{})
	fe := ed.Frontend()
	if dfe, ok := fe.(*dummyFrontend); ok {
		dfe.SetDefaultAction(false)
	}

	w := ed.NewWindow()
	defer w.Close()

	w.NewFile()
	v := w.NewFile()

	l := len(w.Views())

	w.NewFile()
	defer func() {
		for _, vw := range w.Views() {
			vw.SetScratch(true)
			vw.Close()
		}
	}()

	edit := v.BeginEdit()
	v.Insert(edit, 0, "test")
	v.EndEdit(edit)

	if w.CloseAllViews() {
		t.Errorf("Expected views to fail to close, but they didn't")
	}

	if len(w.Views()) != l {
		t.Error("Expected only one view to close, but more did")
	}
}

func TestOpenFile(t *testing.T) {
	tests := []struct {
		in  string
		exp string
	}{
		{
			"testdata/file",
			abs("testdata/file"),
		},
		{
			"/nonexistdir/o",
			"/nonexistdir/o",
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()
	for i, test := range tests {
		v := w.OpenFile(test.in, 0644)

		if out := v.FileName(); out != test.exp {
			t.Errorf("Test %d: Expected view file name %s, but got %s", i, test.exp, out)
		}
		if l := v.Sel().Len(); l != 1 {
			t.Errorf("Test %d: Expected one selection after openning file, but got %d",
				i, l)
		}
		if r := v.Sel().Get(0); r.A != 0 || r.B != 0 {
			t.Errorf("Test %d: Expected initial selection on (0, 0), but got %s",
				i, r)
		}

		v.Close()
	}
}

func TestOpenProject(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	if p := w.OpenProject("testdata/project"); p != nil {
		if got, exp := p.FileName(), abs("testdata/project"); got != exp {
			t.Errorf("Expected project file name %s, but got %s", exp, got)
		}
	} else {
		t.Errorf("OpenProject failed. Project is nil")
	}
}

func abs(path string) string {
	abs, _ := filepath.Abs(path)
	return abs
}
