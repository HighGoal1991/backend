// Copyright 2013 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package sublime

import (
	"bytes"
	"fmt"
	"github.com/limetext/gopy/lib"
	"github.com/limetext/lime/backend"
	_ "github.com/limetext/lime/backend/commands"
	"github.com/limetext/lime/backend/log"
	"github.com/limetext/lime/backend/packages"
	"github.com/limetext/lime/backend/util"
	"github.com/limetext/text"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var dummyClipboard string

type consoleObserver struct {
	T *testing.T
}

func (o *consoleObserver) Erased(changed_buffer text.Buffer, region_removed text.Region, data_removed []rune) {
	// do nothing
}

func (o *consoleObserver) Inserted(changed_buffer text.Buffer, region_inserted text.Region, data_inserted []rune) {
	o.T.Logf("%s", string(data_inserted))
}

func TestSublime(t *testing.T) {
	ed := backend.GetEditor()
	ed.SetClipboardFuncs(func(n string) (err error) {
		dummyClipboard = n
		return nil
	}, func() (string, error) {
		return dummyClipboard, nil
	})
	ed.Init()

	ed.Console().Buffer().AddObserver(&consoleObserver{T: t})
	w := ed.NewWindow()
	Init()
	l := py.NewLock()
	py.AddToPath("testdata")
	py.AddToPath("testdata/plugins")
	if m, err := py.Import("sublime_plugin"); err != nil {
		t.Fatal(err)
	} else {
		plugins := packages.ScanPlugins("testdata/", ".py")
		for _, p := range plugins {
			newPlugin(p, m)
		}
	}

	subl, err := py.Import("sublime")
	if err != nil {
		t.Fatal(err)
	}

	if w, err := _windowClass.Alloc(1); err != nil {
		t.Fatal(err)
	} else {
		(w.(*Window)).data = &backend.Window{}
		subl.AddObject("test_window", w)
	}

	// Testing plugin reload
	data := []byte(`import sublime, sublime_plugin

class TestToxt(sublime_plugin.TextCommand):
    def run(self, edit):
        print("my view's id is: %d" % self.view.id())
        self.view.insert(edit, 0, "Tada")
		`)
	if err := ioutil.WriteFile("testdata/plugins/reload.py", data, 0644); err != nil {
		t.Fatalf("Couldn't write testdata/plugins/reload.py: %s", err)
	}
	defer os.Remove("testdata/plugins/reload.py")
	time.Sleep(time.Millisecond * 50)

	if dir, err := os.Open("testdata"); err != nil {
		t.Error(err)
	} else if files, err := dir.Readdirnames(0); err != nil {
		t.Error(err)
	} else {
		for _, fn := range files {
			if filepath.Ext(fn) == ".py" {
				log.Debug("Running %s", fn)
				if _, err := py.Import(fn[:len(fn)-3]); err != nil {
					log.Error(err)
					t.Error(err)
				} else {
					log.Debug("Ran %s", fn)
				}
			}
		}
	}

	var f func(indent string, v py.Object, buf *bytes.Buffer)
	f = func(indent string, v py.Object, buf *bytes.Buffer) {
		b := v.Base()
		if dir, err := b.Dir(); err != nil {
			t.Error(err)
		} else {
			if l, ok := dir.(*py.List); ok {
				sl := l.Slice()

				if indent == "" {
					for _, v2 := range sl {
						if item, err := b.GetAttr(v2); err != nil {
							t.Error(err)
						} else {
							ty := item.Type()
							line := fmt.Sprintf("%s%s\n", indent, v2)
							buf.WriteString(line)
							if ty == py.TypeType {
								f(indent+"\t", item, buf)
							}
							item.Decref()
						}
					}
				} else {
					for _, v2 := range sl {
						buf.WriteString(fmt.Sprintf("%s%s\n", indent, v2))
					}
				}

			} else {
				ty := dir.Type()
				t.Error("Unexpected type:", ty)
			}
			dir.Decref()
		}
	}
	buf := bytes.NewBuffer(nil)
	f("", subl, buf)

	l.Unlock()

	const expfile = "testdata/api.txt"
	if d, err := ioutil.ReadFile(expfile); err != nil {
		if err := ioutil.WriteFile(expfile, buf.Bytes(), 0644); err != nil {
			t.Error(err)
		}
	} else if diff := util.Diff(string(d), buf.String()); diff != "" {
		t.Error(diff)
	}
	ed.LogCommands(true)
	tests := []string{
		"state",
		"registers",
		"settings",
		"constants",
		"registers",
		"cmd_data",
		"marks",
	}

	for _, test := range tests {
		ed.CommandHandler().RunWindowCommand(w, "vintage_ex_run_data_file_based_tests", backend.Args{"suite_name": test})
	}
	for _, w := range ed.Windows() {
		for _, v := range w.Views() {
			if strings.HasSuffix(v.Buffer().FileName(), "sample.txt") {
				continue
			}
			if strings.Index(v.Buffer().Substr(text.Region{A: 0, B: v.Buffer().Size()}), "FAILED") != -1 {
				t.Error(v.Buffer())
			}
		}
	}

	var v *backend.View
	for _, v2 := range w.Views() {
		if v == nil || v2.Buffer().Size() > v.Buffer().Size() {
			v = v2
		}
	}
}
