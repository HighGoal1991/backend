// Copyright 2013 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package backend

import (
	"fmt"
	"github.com/limetext/lime/backend/textmate"
	"github.com/limetext/lime/backend/util"
	"github.com/limetext/text"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestView(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	edit := v.BeginEdit()
	v.Insert(edit, 0, "abcd")
	v.EndEdit(edit)
	v.selection.Clear()
	r := []text.Region{
		{A: 0, B: 0},
		{A: 1, B: 1},
		{A: 2, B: 2},
		{A: 3, B: 3},
	}
	for _, r2 := range r {
		v.selection.Add(r2)
	}

	edit = v.BeginEdit()
	for _, ins := range "1234" {
		for i := 0; i < v.selection.Len(); i++ {
			v.Insert(edit, v.selection.Get(i).Begin(), string(ins))
		}
	}
	v.EndEdit(edit)

	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "1234a1234b1234c1234d" {
		t.Error(d)
	}
	v.undoStack.Undo(true)
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "abcd" {
		t.Error("expected 'abcd', but got: ", d)
	}
	v.undoStack.Redo(true)
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "1234a1234b1234c1234d" {
		t.Error("expected '1234a1234b1234c1234d', but got: ", d)
	}

	v.selection.Clear()
	r = []text.Region{
		{A: 0, B: 0},
		{A: 5, B: 5},
		{A: 10, B: 10},
		{A: 15, B: 15},
	}
	for _, r2 := range r {
		v.selection.Add(r2)
	}

	edit = v.BeginEdit()
	for _, ins := range []string{"hello ", "world"} {
		for i := 0; i < v.selection.Len(); i++ {
			v.Insert(edit, v.selection.Get(i).Begin(), ins)
		}
	}
	v.EndEdit(edit)

	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "hello world1234ahello world1234bhello world1234chello world1234d" {
		t.Error(d)
	}
	v.undoStack.Undo(true)

	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "1234a1234b1234c1234d" {
		t.Error("expected '1234a1234b1234c1234d', but got: ", d)
	}
	v.undoStack.Undo(true)
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "abcd" {
		t.Error("expected 'abcd', but got: ", d)
	}
	v.undoStack.Undo(true)
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "" {
		t.Error("expected '', but got: ", d)
	}
	v.undoStack.Redo(true)
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "abcd" {
		t.Error("expected 'abcd', but got: ", d)
	}

	v.undoStack.Redo(true)
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "1234a1234b1234c1234d" {
		t.Error("expected '1234a1234b1234c1234d', but got: ", d)
	}

	v.undoStack.Redo(true)
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "hello world1234ahello world1234bhello world1234chello world1234d" {
		t.Error(d)
	}
}

func TestErase(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	s := v.Sel()

	edit := v.BeginEdit()
	v.Insert(edit, 0, "1234abcd5678abcd")
	v.EndEdit(edit)
	s.Clear()
	v.Sel().Add(text.Region{A: 4, B: 8})
	v.Sel().Add(text.Region{A: 12, B: 16})

	edit = v.BeginEdit()
	for i := 0; i < s.Len(); i++ {
		v.Erase(edit, s.Get(i))
	}
	v.EndEdit(edit)
	if !reflect.DeepEqual(s.Regions(), []text.Region{{A: 4, B: 4}, {A: 8, B: 8}}) {
		t.Error(s)
	}
	if d := v.buffer.Substr(text.Region{A: 0, B: v.buffer.Size()}); d != "12345678" {
		t.Error(d)
	}
}

// This is not 100% what ST3 does
func TestExtractScope(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	const (
		in      = "textmate/testdata/main.go"
		expfile = "testdata/scoperange.res"
		syntax  = "textmate/testdata/Go.tmLanguage"
	)
	v.Settings().Set("syntax", syntax)
	if d, err := ioutil.ReadFile(in); err != nil {
		t.Fatal(err)
	} else {
		//		v.rootNode = nil
		e := v.BeginEdit()
		v.Insert(e, 0, string(d))
		v.EndEdit(e)
		last := text.Region{A: -1, B: -1}
		str := ""
		nr := text.Region{A: 0, B: 0}
		for v.ExtractScope(1) == nr {
			time.Sleep(time.Millisecond)
		}
		for i := 0; i < v.buffer.Size(); i++ {
			if r := v.ExtractScope(i); r != last {
				str += fmt.Sprintf("%d (%d, %d)\n", i, r.A, r.B)
				last = r
			}
		}
		if d, err := ioutil.ReadFile(expfile); err != nil {
			if err := ioutil.WriteFile(expfile, []byte(str), 0644); err != nil {
				t.Error(err)
			}
		} else if diff := util.Diff(string(d), str); diff != "" {
			t.Error(diff)
		}
	}
}

// This is not 100% what ST3 does, but IMO ST3 is wrong
func TestScopeName(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	const (
		in      = "textmate/testdata/main.go"
		expfile = "testdata/scopename.res"
		syntax  = "textmate/testdata/Go.tmLanguage"
	)
	v.Settings().Set("syntax", syntax)
	if d, err := ioutil.ReadFile(in); err != nil {
		t.Fatal(err)
	} else {
		//		v.rootNode = nil
		e := v.BeginEdit()
		v.Insert(e, 0, string(d))
		v.EndEdit(e)
		last := ""
		str := ""
		lasti := 0
		for v.ScopeName(1) == "" {
			time.Sleep(250 * time.Millisecond)
		}
		for i := 0; i < v.buffer.Size(); i++ {
			if name := v.ScopeName(i); name != last {
				if last != "" {
					str += fmt.Sprintf("%d-%d: %s\n", lasti, i, last)
					lasti = i
				}
				last = name
			}
		}
		if i := v.Buffer().Size(); lasti != i {
			str += fmt.Sprintf("%d-%d: %s\n", lasti, i, last)
		}
		if d, err := ioutil.ReadFile(expfile); err != nil {
			if err := ioutil.WriteFile(expfile, []byte(str), 0644); err != nil {
				t.Error(err)
			}
		} else if diff := util.Diff(string(d), str); diff != "" {
			t.Error(diff)
		}
	}
}

func TestStress(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	ed := GetEditor()
	w := ed.NewWindow()
	defer w.Close()

	v := w.OpenFile("../frontend/termbox/main.go", 0)
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	syntax := "../packages/go.tmbundle/Syntaxes/Go.tmLanguage"
	v.Settings().Set("syntax", syntax)
	for i := 0; i < 1000; i++ {
		e := v.BeginEdit()
		for i := 0; i < 100; i++ {
			v.Insert(e, 0, "h")
		}
		for i := 0; i < 100; i++ {
			v.Erase(e, text.Region{A: 0, B: 1})
		}
		v.EndEdit(e)
	}
}

func TestTransform(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	sc, err := textmate.LoadTheme("../packages/themes/TextMate-Themes/GlitterBomb.tmTheme")
	if err != nil {
		t.Fatal(err)
	}

	d, err := ioutil.ReadFile("view.go")
	if err != nil {
		t.Fatal(err)
	}
	e := v.BeginEdit()
	v.Insert(e, 0, string(d))
	v.EndEdit(e)

	if v.Transform(sc, text.Region{A: 0, B: 100}) != nil {
		t.Error("Expected view.Transform return nil when the syntax isn't set yet")
	}

	v.Settings().Set("syntax", "textmate/testdata/Go.tmLanguage")

	time.Sleep(time.Second)
	a := v.Transform(sc, text.Region{A: 0, B: 100}).Transcribe()
	v.Transform(sc, text.Region{A: 100, B: 200}).Transcribe()
	c := v.Transform(sc, text.Region{A: 0, B: 100}).Transcribe()
	if !reflect.DeepEqual(a, c) {
		t.Errorf("not equal:\n%v\n%v", a, c)
	}
}

func BenchmarkTransformTranscribe(b *testing.B) {
	b.StopTimer()
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()

	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	sc, err := textmate.LoadTheme("../packages/themes/TextMate-Themes/GlitterBomb.tmTheme")
	if err != nil {
		b.Fatal(err)
	}

	v.Settings().Set("syntax", "textmate/testdata/Go.tmLanguage")

	d, err := ioutil.ReadFile("view.go")
	if err != nil {
		b.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	v.Settings().AddOnChange("benchmark", func(key string) {
		if key == "lime.syntax.updated" {
			wg.Done()
		}
	})
	e := v.BeginEdit()
	v.Insert(e, 0, string(d))
	v.EndEdit(e)
	wg.Wait()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		v.Transform(sc, text.Region{A: 0, B: v.Buffer().Size()}).Transcribe()
	}
	fmt.Println(util.Prof.String())
}

func TestSaveAsNewFile(t *testing.T) {
	tests := []struct {
		text   string
		atomic bool
		file   string
	}{
		{
			"abc",
			false,
			"testdata/test",
		},
		{
			"abc",
			true,
			"testdata/test",
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	for i, test := range tests {
		v := w.NewFile()

		v.Settings().Set("atomic_save", test.atomic)

		e := v.BeginEdit()

		v.Insert(e, 0, test.text)
		v.EndEdit(e)
		if err := v.SaveAs(test.file); err != nil {
			t.Fatalf("Test %d: Can't save to `%s`: %s", i, test.file, err)
		}

		if v.IsDirty() {
			t.Errorf("Test %d: Expected the view to be clean, but it wasn't", i)
		}

		data, err := ioutil.ReadFile(test.file)
		if err != nil {
			t.Fatalf("Test %d: Can't read `%s`: %s", i, test.file, err)
		}
		if string(data) != test.text {
			t.Errorf("Test %d: Expected `%s` contain %s, but got %s", i, test.file, test.text, data)
		}

		v.Close()

		if err = os.Remove(test.file); err != nil {
			t.Errorf("Test %d: Couldn't remove test file %s", i, test.file)
		}
	}
}

func TestSaveAsOpenFile(t *testing.T) {
	var testfile string = "testdata/Default.sublime-settings"

	buf, err := ioutil.ReadFile(testfile)
	if err != nil {
		t.Fatalf("Can't read test file `%s`: %s", testfile, err)
	}

	tests := []struct {
		atomic bool
		as     string
	}{
		{
			true,
			"User.sublime-settings",
		},
		{
			true,
			"testdata/User.sublime-settings",
		},
		{
			true,
			"../User.sublime-settings",
		},
		{
			true,
			os.TempDir() + "/User.sublime-settings",
		},
		{
			false,
			"User.sublime-settings",
		},
		{
			false,
			"testdata/User.sublime-settings",
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	for i, test := range tests {
		v := w.OpenFile(testfile, 0)

		v.Settings().Set("atomic_save", test.atomic)
		if err := v.SaveAs(test.as); err != nil {
			t.Fatalf("Test %d: Can't save to `%s`: %s", i, test.as, err)
		}

		if v.IsDirty() {
			t.Errorf("Test %d: Expected the view to be clean, but it wasn't", i)
		}

		if _, err := os.Stat(test.as); os.IsNotExist(err) {
			t.Fatalf("Test %d: The file `%s` wasn't created", i, test.as)
		}

		data, err := ioutil.ReadFile(test.as)
		if err != nil {
			t.Fatalf("Test %d: Can't read `%s`: %s", i, test.as, err)
		}
		if string(data) != string(buf) {
			t.Errorf("Test %d: Expected `%s` contain %s, but got %s", i, test.as, string(buf), data)
		}

		v.Close()

		if err := os.Remove(test.as); err != nil {
			t.Errorf("Test %d: Couldn't remove test file %s", i, test.as)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		text   string
		points []int
		expect []int
	}{
		{
			"",
			[]int{0, 10},
			[]int{3520, 3520},
		},
		{
			"abc Hi -test lime,te-xt\n\tclassify test-ing",
			[]int{0, 4, 5, 6, 7, 8, 13, 17, 18, 20, 21, 23, 24, 25, 34, 38, 39, 42},
			[]int{73, 49, 512, 2, 1028, 9, 1, 8198, 4105, 6, 9, 130, 64, 1, 1, 6, 9, 134},
		},
		{
			"(tes)ting cl][assify\n\npare(,,)nthe\\ses\n\t\n// Use",
			[]int{0, 4, 12, 13, 14, 20, 21, 22, 26, 27, 28, 29, 30, 34, 35, 39, 40, 41, 42, 43, 44, 47},
			[]int{5188, 8198, 8198, 12288, 4105, 130, 448, 65, 4102, 12288, 0, 12288, 8201, 6, 9, 64, 128, 1092, 0, 2056, 49, 134},
		},
		{
			"view__classify",
			[]int{4, 5, 6},
			[]int{544, 512, 528},
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	for i, test := range tests {
		v := w.NewFile()
		defer func() {
			v.SetScratch(true)
			v.Close()
		}()

		e := v.BeginEdit()
		v.Insert(e, 0, test.text)
		v.EndEdit(e)
		for j, point := range test.points {
			if res := v.Classify(point); test.expect[j] != res {
				t.Errorf("Test %d: Expected %d from view.Classify(%d) but, got %d", i, test.expect[j], point, res)
			}
		}
	}
}

func TestFindByClass(t *testing.T) {
	tests := []struct {
		text    string
		point   int
		forward bool
		classes int
		expect  int
	}{
		{
			"abc Hi -test lime",
			1,
			true,
			CLASS_PUNCTUATION_START,
			7,
		},
		{
			"abc Hi -test lime",
			8,
			true,
			CLASS_PUNCTUATION_START,
			17,
		},
		{
			"abc Hi -test lime",
			5,
			true,
			CLASS_WORD_START,
			8,
		},
		{
			"abc Hi -test lime",
			5,
			false,
			CLASS_EMPTY_LINE,
			0,
		},
		{
			"abc Hi -test lime",
			9,
			false,
			CLASS_SUB_WORD_START,
			4,
		},
		{
			"abc Hi -test lime",
			9,
			false,
			CLASS_WORD_END | CLASS_PUNCTUATION_END,
			8,
		},
		{
			"abc Hi -test lime",
			0,
			true,
			CLASS_WORD_START | CLASS_WORD_END,
			3,
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	for i, test := range tests {
		v := w.NewFile()
		defer func() {
			v.SetScratch(true)
			v.Close()
		}()

		e := v.BeginEdit()
		v.Insert(e, 0, test.text)
		v.EndEdit(e)
		if res := v.FindByClass(test.point, test.forward, test.classes); res != test.expect {
			t.Errorf("Test %d: Expected %d from view.FindByClass but, got %d", i, test.expect, res)
		}
	}
}

func TestExpandByClass(t *testing.T) {
	tests := []struct {
		text    string
		start   text.Region
		classes int
		expect  text.Region
	}{
		{
			"abc Hi -test lime",
			text.Region{A: 1, B: 2},
			CLASS_WORD_START,
			text.Region{A: 0, B: 4},
		},
		{
			"abc Hi -test lime",
			text.Region{A: 8, B: 10},
			CLASS_WORD_START | CLASS_WORD_END,
			text.Region{A: 6, B: 12},
		},
		{
			"abc Hi -test lime",
			text.Region{A: 12, B: 14},
			CLASS_PUNCTUATION_START,
			text.Region{A: 7, B: 17},
		},
		{
			"abc Hi -test lime",
			text.Region{A: 12, B: 14},
			CLASS_PUNCTUATION_END,
			text.Region{A: 8, B: 17},
		},
		{
			"abc Hi -test lime",
			text.Region{A: 9, B: 11},
			CLASS_WORD_START | CLASS_WORD_END,
			text.Region{A: 8, B: 12},
		},
		{
			"abc Hi -test lime",
			text.Region{A: -1, B: 20},
			CLASS_WORD_START,
			text.Region{A: 0, B: 17},
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	for i, test := range tests {
		v := w.NewFile()
		defer func() {
			v.SetScratch(true)
			v.Close()
		}()

		e := v.BeginEdit()
		v.Insert(e, 0, test.text)
		v.EndEdit(e)
		if res := v.ExpandByClass(test.start, test.classes); res != test.expect {
			t.Errorf("Test %d: Expected %v from view.ExpandByClass, but got %v", i, test.expect, res)
		}
	}
}

func TestSetBuffer(t *testing.T) {
	var w Window

	v := newView(&w)
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	b := text.NewBuffer()
	b.SetName("test")

	_ = v.setBuffer(b)

	if v.buffer.Name() != b.Name() {
		t.Errorf("Expected buffer called %s, but got %s", b.Name(), v.buffer.Name())
	}
}

func TestSetBufferTwice(t *testing.T) {
	var w Window

	v := newView(&w)
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	b1 := text.NewBuffer()
	b1.SetName("test1")

	_ = v.setBuffer(b1)

	b2 := text.NewBuffer()
	b2.SetName("test2")

	err := v.setBuffer(b2)

	if err == nil {
		t.Errorf("Expected setting the second buffer to cause an error, but it didn't.")
	}

	if v.buffer.Name() != b1.Name() {
		t.Errorf("Expected buffer called %s, but got %s", b1.Name(), v.buffer.Name())
	}
}

func TestWindow(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	if v.Window() != w {
		t.Errorf("Expected the set window to be the one that spawned the view, but it isn't.")
	}
}

func TestSetScratch(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	def := v.IsScratch()

	v.SetScratch(!def)

	if v.IsScratch() == def {
		t.Errorf("Expected the view to be scratch = %v, but it was %v", !def, v.IsScratch())
	}
}

func TestSetOverwriteStatus(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	def := v.OverwriteStatus()

	v.SetOverwriteStatus(!def)

	if v.OverwriteStatus() == def {
		t.Errorf("Expected the view to be overwrite = %v, but it was %v", !def, v.OverwriteStatus())
	}
}

func TestIsDirtyWhenScratch(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer v.Close()

	v.SetScratch(true)

	if v.IsDirty() {
		t.Errorf("Expected the view not to be marked as dirty, but it was")
	}
}

func TestIsDirtyWhenClean(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.OpenFile("testdata/Default.sublime-keymap", 0)
	defer v.Close()

	v.Save()

	if v.IsDirty() {
		t.Errorf("Expected the view not to be marked as dirty, but it was")
	}
}

func TestIsDirtyWhenDirty(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	v.SetScratch(false)
	v.buffer.Insert(0, "test")

	if !v.IsDirty() {
		t.Errorf("Expected the view to be marked as dirty, but it wasn't")
	}
}

func TestCloseView(t *testing.T) {
	w := GetEditor().NewWindow()
	defer w.Close()

	l := len(w.Views())

	v := w.OpenFile("testdata/Default.sublime-keymap", 0)

	v.Save()
	v.Close()

	if len(w.Views()) != l {
		t.Errorf("Expected %d views, but got %d", l, len(w.Views()))
	}
}

func TestCloseView2(t *testing.T) {
	const testfile = "testdata/Default.sublime-keymap"
	fe := GetEditor().Frontend()
	if dfe, ok := fe.(*DummyFrontend); ok {
		// Make it trigger a reload
		dfe.SetDefaultAction(true)
	}

	// Make sure a closed view isn't reloaded after it has been closed
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.OpenFile(testfile, 0)

	v.SetScratch(true)
	v.Close()

	if data, err := ioutil.ReadFile(testfile); err != nil {
		t.Errorf("Couldn't load file: %s", err)
		return
	} else if err = ioutil.WriteFile(testfile, data, 0644); err != nil {
		t.Errorf("Couldn't save file: %s", err)
		return
	}
}

func TestViewLoadSettings(t *testing.T) {
	GetEditor().loadSettings()

	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	if v.Settings().Get("translate_tabs_to_spaces", true).(bool) != false {
		t.Error("Expected `translate_tabs_to_spaces` be false for a new view but is true")
	}

	v.Settings().Set("syntax", "../packages/python.tmbundle/Syntaxes/Python.tmLanguage")
	if v.Settings().Get("translate_tabs_to_spaces", false).(bool) != true {
		t.Error("Expected `translate_tabs_to_spaces` be true for python syntax but is false")
	}
}

func BenchmarkScopeNameLinear(b *testing.B) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	const (
		in     = "textmate/language_test.go"
		syntax = "textmate/testdata/Go.tmLanguage"
	)
	b.StopTimer()
	v.Settings().Set("syntax", syntax)
	if d, err := ioutil.ReadFile(in); err != nil {
		b.Fatal(err)
	} else {
		e := v.BeginEdit()
		v.Insert(e, 0, string(d))
		v.EndEdit(e)
		b.StartTimer()
		for j := 0; j < b.N; j++ {
			for i := 0; i < v.buffer.Size(); i++ {
				v.ScopeName(i)
			}
		}
	}
}

func BenchmarkScopeNameRandom(b *testing.B) {
	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	const (
		in     = "textmate/language_test.go"
		syntax = "textmate/testdata/Go.tmLanguage"
	)
	b.StopTimer()
	v.Settings().Set("syntax", syntax)
	if d, err := ioutil.ReadFile(in); err != nil {
		b.Fatal(err)
	} else {
		e := v.BeginEdit()
		v.Insert(e, 0, string(d))
		v.EndEdit(e)
		p := rand.Perm(b.N)
		b.StartTimer()
		for _, i := range p {
			v.ScopeName(i)
		}
	}
}

func TestFind(t *testing.T) {
	in := "testing\nview.find\n[lite*r.al|ignoreCAsE]\n\tabra_kadabra\n\n"
	tests := []struct {
		pat   string
		pos   int
		flags int
		exp   text.Region
	}{
		{"view", 2, 0, text.Region{8, 12}},
		{"eof", 50, 0, text.Region{-1, -1}},
		{"caSE", 10, IGNORECASE, text.Region{35, 39}},
		{"[lite*r", 1, LITERAL, text.Region{18, 25}},
		{".Al", 1, LITERAL | IGNORECASE, text.Region{25, 28}},
		{"^\n", 4, 0, text.Region{55, 56}},
		{"[A-C]", 4, 0, text.Region{35, 36}},
		{"abra$", 4, 0, text.Region{50, 54}},
		{"i(nd|ng)", 4, 0, text.Region{4, 7}},
		{"p?aSe", 4, IGNORECASE, text.Region{36, 39}},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	e := v.BeginEdit()
	v.Insert(e, 0, in)
	v.EndEdit(e)

	for i, test := range tests {
		if ret := v.Find(test.pat, test.pos, test.flags); !reflect.DeepEqual(ret, test.exp) {
			t.Errorf("Test %d: Expected return region be %s, but got %s", i, test.exp, ret)
		}
	}
}

func TestSetStatus(t *testing.T) {
	tests := []struct {
		keys, vals []string
		exp        map[string]string
	}{
		{
			[]string{"a", "", "d"},
			[]string{"b", "c", ""},
			map[string]string{"a": "b", "": "c", "d": ""},
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	for i, test := range tests {
		for j, key := range test.keys {
			v.SetStatus(key, test.vals[j])
		}
		if !reflect.DeepEqual(v.status, test.exp) {
			t.Errorf("Test %d: Expected %v be equal to %v", i, v.status, test.exp)
		}
	}
}

func TestGetStatus(t *testing.T) {
	tests := []struct {
		st  map[string]string
		get map[string]string
	}{
		{
			map[string]string{"a": "b", "": "c", "d": ""},
			map[string]string{"a": "b", "": "c", "d": ""},
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	for i, test := range tests {
		v.status = test.st
		for key, exp := range test.get {
			if val := v.GetStatus(key); val != exp {
				t.Errorf("Test %d: Expected key %s value be %s, but got %s", i, key, exp, val)
			}
		}
	}
}

func TestEraseStatus(t *testing.T) {
	tests := []struct {
		st   map[string]string
		keys []string
		exp  map[string]string
	}{
		{
			map[string]string{"a": "b", "c": "d"},
			[]string{"a"},
			map[string]string{"c": "d"},
		},
		{
			map[string]string{"a": "b", "": "c", "d": ""},
			[]string{"", "d"},
			map[string]string{"a": "b"},
		},
	}

	w := GetEditor().NewWindow()
	defer w.Close()

	v := w.NewFile()
	defer func() {
		v.SetScratch(true)
		v.Close()
	}()

	for i, test := range tests {
		v.status = test.st
		for _, key := range test.keys {
			v.EraseStatus(key)
		}
		if !reflect.DeepEqual(v.status, test.exp) {
			t.Errorf("Test %d: Expected %v be equal to %v", i, v.status, test.exp)
		}
	}
}
