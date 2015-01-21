// Copyright 2013 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package backend

import (
	"github.com/limetext/lime/backend/log"
	"github.com/limetext/text"
	"io/ioutil"
	"path/filepath"
	"runtime/debug"
	"sync"
)

type Window struct {
	text.HasId
	text.HasSettings
	views       []*View
	active_view *View
	lock        sync.Mutex
}

func (w *Window) NewFile() *View {
	w.lock.Lock()
	defer w.lock.Unlock()

	w.views = append(w.views, newView(w))
	v := w.views[len(w.views)-1]

	v.Settings().SetParent(w)
	v.setBuffer(text.NewBuffer())
	v.selection.Clear()
	v.selection.Add(text.Region{A: 0, B: 0})
	v.Settings().Set("lime.last_save_change_count", v.buffer.ChangeCount())

	OnNew.Call(v)
	w.SetActiveView(v)

	return v
}

func (w *Window) Views() []*View {
	w.lock.Lock()
	defer w.lock.Unlock()
	ret := make([]*View, len(w.views))
	copy(ret, w.views)
	return ret
}

func (w *Window) remove(v *View) {
	w.lock.Lock()
	defer w.lock.Unlock()
	for i, vv := range w.views {
		if v == vv {
			end := len(w.views) - 1
			if i != end {
				copy(w.views[i:], w.views[i+1:])
			}
			w.views = w.views[:end]
			return
		}
	}
	log.Errorf("Wanted to remove view %+v, but it doesn't appear to be a child of this window", v)
}

func (w *Window) OpenFile(filename string, flags int) *View {
	v := w.NewFile()

	v.SetScratch(true)
	e := v.BeginEdit()
	if fn, err := filepath.Abs(filename); err != nil {
		v.Buffer().SetFileName(filename)
	} else {
		v.Buffer().SetFileName(fn)
	}
	if d, err := ioutil.ReadFile(filename); err != nil {
		log.Errorf("Couldn't load file %s: %s", filename, err)
	} else {
		v.Insert(e, 0, string(d))
	}
	v.EndEdit(e)
	v.selection.Clear()
	v.selection.Add(text.Region{A: 0, B: 0})
	v.Settings().Set("lime.last_save_change_count", v.buffer.ChangeCount())
	v.SetScratch(false)

	OnLoad.Call(v)
	w.SetActiveView(v)

	return v
}

func (w *Window) SetActiveView(v *View) {
	if w.active_view != nil {
		OnDeactivated.Call(w.active_view)
	}
	w.active_view = v
	if w.active_view != nil {
		OnActivated.Call(w.active_view)
	}
}

func (w *Window) ActiveView() *View {
	return w.active_view
}

// Closes the Window and all its Views.
// Returns "true" if the Window closed successfully. Otherwise returns "false".
func (w *Window) Close() bool {
	if !w.CloseAllViews() {
		return false
	}
	GetEditor().remove(w)

	return true
}

// Closes all of the Window's Views.
// Returns "true" if all the Views closed successfully. Otherwise returns "false".
func (w *Window) CloseAllViews() bool {
	for len(w.views) > 0 {
		if !w.views[0].Close() {
			return false
		}
	}

	return true
}

func (w *Window) runCommand(c WindowCommand, name string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("Paniced while running window command %s %v: %v\n%s", name, c, r, string(debug.Stack()))
		}
	}()
	return c.Run(w)
}
