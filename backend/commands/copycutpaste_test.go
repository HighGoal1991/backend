// Copyright 2014 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package commands

import (
	. "github.com/limetext/lime/backend"
	"github.com/limetext/text"
	"testing"
)

type copyTest struct {
	buf     string
	clip    string
	regions []text.Region
	expClip string
	expBuf  string
}

var dummyClipboard string

func runCopyTest(command string, tests *[]copyTest, t *testing.T) {
	ed := GetEditor()
	ed.SetClipboardFuncs(func(n string) (err error) {
		dummyClipboard = n
		return nil
	}, func() (string, error) {
		return dummyClipboard, nil
	})
	defer ed.Init()

	w := ed.NewWindow()
	defer w.Close()

	for i, test := range *tests {
		v := w.NewFile()
		defer func() {
			v.SetScratch(true)
			v.Close()
		}()

		v.Buffer().Insert(0, test.buf)
		v.Sel().Clear()

		ed.SetClipboard(test.clip)

		for _, r := range test.regions {
			v.Sel().Add(r)
		}

		ed.CommandHandler().RunTextCommand(v, command, nil)

		if ed.GetClipboard() != test.expClip {
			t.Errorf("Test %d: Expected clipboard to be %q, but got %q", i, test.expClip, ed.GetClipboard())
		}

		b := v.Buffer().Substr(text.Region{A: 0, B: v.Buffer().Size()})

		if b != test.expBuf {
			t.Errorf("Test %d: Expected buffer to be %q, but got %q", i, test.expBuf, b)
		}
	}
}

func TestCopy(t *testing.T) {
	tests := []copyTest{
		{
			"test string",
			"",
			[]text.Region{{1, 3}},
			"es",
			"test string",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{3, 6}},
			"t\ns",
			"test\nstring",
		},
		{
			"test string",
			"",
			[]text.Region{{3, 3}},
			"test string\n",
			"test string",
		},
		{
			"test string",
			"",
			[]text.Region{{1, 3}, {5, 6}},
			"es\ns",
			"test string",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{1, 3}, {5, 6}},
			"es\ns",
			"test\nstring",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{1, 1}, {7, 7}},
			"test\n\nstring\n",
			"test\nstring",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{3, 6}, {9, 10}},
			"t\ns\nn",
			"test\nstring",
		},
		{
			"test string",
			"",
			[]text.Region{{5, 6}, {1, 3}},
			"es\ns",
			"test string",
		},
		{
			"test string",
			"",
			[]text.Region{{1, 1}, {6, 7}},
			"t",
			"test string",
		},
	}

	runCopyTest("copy", &tests, t)
}

func TestCut(t *testing.T) {
	tests := []copyTest{
		{
			"test string",
			"",
			[]text.Region{{1, 3}},
			"es",
			"tt string",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{3, 6}},
			"t\ns",
			"testring",
		},
		{
			"test string",
			"",
			[]text.Region{{3, 3}},
			"test string\n",
			"",
		},
		{
			"test string",
			"",
			[]text.Region{{5, 6}, {1, 3}},
			"es\ns",
			"tt tring",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{1, 3}, {5, 6}},
			"es\ns",
			"tt\ntring",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{1, 1}, {7, 7}},
			"test\n\nstring\n",
			"",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{3, 6}, {9, 10}},
			"t\ns\nn",
			"testrig",
		},
		{
			"test string",
			"",
			[]text.Region{{5, 6}, {1, 3}},
			"es\ns",
			"tt tring",
		},
		{
			"test string",
			"",
			[]text.Region{{6, 7}, {1, 1}},
			"t",
			"",
		},
		{
			"test\nstring",
			"",
			[]text.Region{{1, 1}, {6, 7}},
			"t",
			"sring",
		},
	}

	runCopyTest("cut", &tests, t)
}

func TestPaste(t *testing.T) {
	tests := []copyTest{
		{
			"test string",
			"test",
			[]text.Region{{1, 1}},
			"test",
			"ttestest string",
		},
		{
			"test string",
			"test",
			[]text.Region{{1, 3}},
			"test",
			"ttestt string",
		},
		{
			"test\nstring",
			"test",
			[]text.Region{{3, 6}},
			"test",
			"testesttring",
		},
		{
			"test string",
			"test",
			[]text.Region{{1, 3}, {5, 6}},
			"test",
			"ttestt testtring",
		},
		{
			"test\nstring",
			"test",
			[]text.Region{{1, 3}, {5, 6}},
			"test",
			"ttestt\ntesttring",
		},
		{
			"test\nstring",
			"test",
			[]text.Region{{3, 6}, {9, 10}},
			"test",
			"testesttritestg",
		},
		{
			"test\nstring",
			"test",
			[]text.Region{{9, 10}, {3, 6}},
			"test",
			"testesttritestg",
		},
	}

	runCopyTest("paste", &tests, t)
}
