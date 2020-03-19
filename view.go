// Copyright 2013 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package backend

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/limetext/backend/log"
	"github.com/limetext/backend/packages"
	"github.com/limetext/backend/parser"
	"github.com/limetext/backend/render"
	"github.com/limetext/rubex"
	"github.com/limetext/text"
	"github.com/limetext/util"
)

type (
	// A View provides a view into a specific underlying buffer
	// with its own set of selections, settings, viewport, etc.
	// Multiple Views can share the same underlying data buffer.
	View struct {
		text.HasSettings
		text.HasId
		window           *Window
		buffer           text.Buffer
		selection        text.RegionSet
		undoStack        UndoStack
		scratch          bool
		overwrite        bool
		cursyntax        string
		syntax           parser.SyntaxHighlighter
		regions          render.ViewRegionMap
		editstack        []*Edit
		lock             sync.Mutex
		reparseChan      chan parseReq
		status           map[string]string
		defaultSettings  *text.HasSettings
		platformSettings *text.HasSettings
		userSettings     *text.HasSettings
	}
	parseReq struct {
		forced bool
	}
)

func newView(w *Window) *View {
	v := &View{
		window:           w,
		regions:          make(render.ViewRegionMap),
		status:           make(map[string]string),
		reparseChan:      make(chan parseReq, 32),
		defaultSettings:  new(text.HasSettings),
		platformSettings: new(text.HasSettings),
		userSettings:     new(text.HasSettings),
	}
	v.Sel().AddOnChange("selection modified", func() {
		OnSelectionModified.Call(v)
	})
	// Initializing keybidings hierarchy
	// project <- syntax default <- syntax platform <- syntax user <- buffer
	v.defaultSettings.Settings().SetParent(v.window.Project())
	v.platformSettings.Settings().SetParent(v.defaultSettings)
	v.userSettings.Settings().SetParent(v.platformSettings)
	v.Settings().SetParent(v.userSettings)

	v.loadSettings()
	v.Settings().AddOnChange("backend.view.syntax", func(name string) {
		if name != "syntax" {
			return
		}
		v.lock.Lock()
		defer v.lock.Unlock()

		if syn := v.Settings().String("syntax", ""); syn != v.cursyntax {
			v.cursyntax = syn
			v.loadSettings()
			v.reparse(true)
		}
	})
	go v.parsethread()
	v.Settings().Set("is_widget", false)

	return v
}

// implement the fmt.Stringer interface
func (v *View) String() string {
	return fmt.Sprintf("View{id:%d, buffer: %s}", v.Id(), v.buffer)
}

func (v *View) setBuffer(b text.Buffer) error {
	if v.buffer != nil {
		return fmt.Errorf("There is already a buffer set")
	}
	v.buffer = b
	// TODO(q): Dynamically load the correct syntax file
	b.AddObserver(v)
	return nil
}

// BufferObserver

func (v *View) Erased(changed_buffer text.Buffer, region_removed text.Region, data_removed []rune) {
	v.flush(region_removed.B, region_removed.A-region_removed.B)
}

func (v *View) Inserted(changed_buffer text.Buffer, region_inserted text.Region, data_inserted []rune) {
	v.flush(region_inserted.A, region_inserted.B-region_inserted.A)
}

// End of Buffer Observer

// Flush is called every time the underlying buffer is changed.
// It calls Adjust() on all the regions associated with this view,
// triggers the "OnModified" event, and adds a reparse request
// to the parse go-routine.
func (v *View) flush(position, delta int) {
	v.Sel().Adjust(position, delta)
	func() {
		v.lock.Lock()
		defer v.lock.Unlock()

		e := util.Prof.Enter("view.flush")
		defer e.Exit()
		if v.syntax != nil {
			v.syntax.Adjust(position, delta)
		}
		for k, v2 := range v.regions {
			v2.Regions.Adjust(position, delta)
			v.regions[k] = v2
		}
	}()
	OnModified.Call(v)
	v.reparse(false)
}

// parsethread() would be the go-routine used for dealing with reparsing the
// current buffer when it has been modified. Each opened view has its own
// go-routine parsethread() which sits idle and waits for requests to be sent
// on this view's reparseChan.
//
// The Buffer's ChangeCount, as well as the parse request's "forced" attribute
// is used to determined if a parse actually needs to happen or not.
//
// If it is decided that a reparse should take place, a snapshot of the Buffer is
// taken and a parse is performed. Upon completion of this parse operation,
// and if the snapshot of the buffer has not already become outdated,
// then the regions of the view associated with syntax highlighting is updated.
//
// Changes made to the Buffer during the time when there is no accurate
// parse of the buffer is a monkey-patched version of the old syntax highlighting
// regions, which in most instances will be accurate.
//
// See package backend/parser for more details.
func (v *View) parsethread() {
	pc := 0
	lastParse := -1
	doparse := func() (ret bool) {
		p := util.Prof.Enter("syntax.parse")
		defer p.Exit()
		defer func() {
			if r := recover(); r != nil {
				log.Error("Panic in parse thread: %v\n%s", r, string(debug.Stack()))
				if pc > 0 {
					panic(r)
				}
				pc++
			}
		}()

		data := v.Substr(text.Region{0, v.Size()})
		syntax := v.Settings().String("syntax", "")
		sh := syntaxHighlighter(syntax, data)

		// Only set if it isn't invalid already, otherwise the
		// current syntax highlighting will be more accurate
		// as it will have had incremental adjustments done to it
		if v.ChangeCount() != lastParse {
			return
		}

		v.lock.Lock()
		defer v.lock.Unlock()

		v.syntax = sh
		for k := range v.regions {
			if strings.HasPrefix(k, "lime.syntax") {
				delete(v.regions, k)
			}
		}

		for k, v2 := range sh.Flatten() {
			if v2.Regions.HasNonEmpty() {
				v.regions[k] = v2
			}
		}

		return true
	}

	v.lock.Lock()
	ch := v.reparseChan
	v.lock.Unlock()
	defer v.cleanup()
	if ch == nil {
		return
	}

	for pr := range ch {
		if cc := v.ChangeCount(); lastParse != cc || pr.forced {
			lastParse = cc
			if doparse() {
				v.Settings().Set("lime.syntax.updated", lastParse)
			}
		}
	}
}

// Send a reparse request via the reparse channel.
// If "forced" is set to true, then a reparse will be made
// even if the Buffer appears to not have changed.
//
// The actual parsing is done in a separate go-routine, for which the
// "lime.syntax.updated" setting will be set once it has finished.
//
// Note that it's presumed that the function calling this function
// has locked the view!
func (v *View) reparse(forced bool) {
	if v.isClosed() {
		// No point in issuing a re-parse if the view has been closed
		return
	}
	if len(v.reparseChan) < cap(v.reparseChan) || forced {
		v.reparseChan <- parseReq{forced}
	}
}

// Will load view settings respect to current syntax
// e.g if current syntax is Python settings order will be:
// Packages/Python/Python.sublime-settings
// Packages/Python/Python (windows).sublime-settings
// Packages/User/Python.sublime-settings
// <Buffer Specific Settings>
func (v *View) loadSettings() {
	syntax := v.Settings().String("syntax", "")
	ed := GetEditor()
	if r, err := rubex.Compile(`([A-Za-z]+?)\.(?:[^.]+)$`); err != nil {
		log.Error(err)
		// TODO: should we match syntax file name or the syntax name
	} else if s := r.FindStringSubmatch(syntax); len(s) > 1 {
		// TODO: the syntax folder should be the package path and name
		p := path.Join(ed.PackagesPath(), s[1], s[1]+".sublime-settings")
		log.Fine("Loading %s for view", p)
		packages.LoadJSON(p, v.defaultSettings.Settings())

		p = path.Join(ed.PackagesPath(), s[1], s[1]+" ("+ed.Plat()+").sublime-settings")
		log.Fine("Loading %s for view", p)
		packages.LoadJSON(p, v.platformSettings.Settings())

		p = path.Join(ed.UserPath(), s[1]+".sublime-settings")
		log.Fine("Loading %s for view", p)
		packages.LoadJSON(p, v.userSettings.Settings())
	}
}

// Returns the full concatenated nested scope name at point.
// See package backend/parser for details.
func (v *View) ScopeName(point int) string {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.syntax != nil {
		return v.syntax.ScopeName(point)
	}
	return ""
}

// Returns the Region of the innermost scope that contains "point".
// See package backend/parser for details.
func (v *View) ExtractScope(point int) text.Region {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.syntax != nil {
		return v.syntax.ScopeExtent(point)
	}
	return text.Region{}
}

// ScoreSelector() takes a point and a selector string and returns a score
// as to how good that specific selector matches the scope name at
// that point.
func (v *View) ScoreSelector(point int, selector string) int {
	// TODO(.): The algorithm to determine the score has not been compared
	// to what ST3 actually does. Not sure if any plugin I personally
	// care about uses this functionality, and if it does if it requires
	// specific scores to be returned.
	//
	// The overall implementation should be fine as a higher score is
	// returned the more specific a selector is due to the innermost
	// scope name being concatenated last in the string returned by ScopeName
	if sn := v.ScopeName(point); len(sn) > 0 {
		return 1 + strings.Index(sn, selector)
	}
	return 0
}

// Sel() returns a pointer to the RegionSet used by this View
// to mark possibly multiple cursor positions and selection
// regions.
//
// Some quick notes:
// The actual cursor position is always in Region.B.
// Region{0,0} is a cursor at the start of the text (before any characters in the text).
//
// Region{0,1} has the cursor at position 1 (after the first character),
// but also selects/highlights the first character. In this instance Region.A = 0, Region.B = 1,
// Region.Start() returns 0 and Region.End() returns 1.
//
// Region{1,0} has the cursor at position 0 (before the first character),
// but also selects/highlights the first character. Think holding shift and pressing left on your keyboard.
// In this instance Region.A = 1, Region.B = 0, Region.Start() returns 0 and Region.End() returns 1.
//
func (v *View) Sel() *text.RegionSet {
	// BUG(.): Sometimes Sel becomes empty. There should always be at a minimum 1 valid cursor.
	return &v.selection
}

// Returns the window this View belongs to.
func (v *View) Window() *Window {
	return v.window
}

// Inserts text at the given position in the provided edit object.
// Tabs are (sometimes, depending on the View's settings) translated to spaces.
// The return value is the length of the string that was inserted.
func (v *View) Insert(edit *Edit, point int, value string) int {
	if t := v.Settings().Bool("translate_tabs_to_spaces", false); t && strings.Contains(value, "\t") {
		tab_size := v.Settings().Int("tab_size", 4)
		lines := strings.Split(value, "\n")
		for i, li := range lines {
			for {
				idx := strings.Index(li, "\t")
				if idx == -1 {
					break
				}
				ai := idx
				if i == 0 {
					_, col := v.RowCol(point)
					ai = col + 1
				}
				add := 1 + ((ai + (tab_size - 1)) &^ (tab_size - 1))
				spaces := ""
				for j := ai; j < add; j++ {
					spaces += " "
				}
				li = li[:idx] + spaces + li[idx+1:]
			}
			lines[i] = li
		}
		value = strings.Join(lines, "\n")
	}
	edit.composite.AddExec(text.NewInsertAction(v.buffer, point, value))
	// TODO(.): I think this should rather be the number of runes inserted?
	// The spec states that len() of a string returns the number of bytes,
	// which isn't very useful as all other buffer values are IIRC in runes.
	// http://golang.org/ref/spec#Length_and_capacity
	return len(value)
}

// Adds an Erase action of the given Region to the provided Edit object.
func (v *View) Erase(edit *Edit, r text.Region) {
	edit.composite.AddExec(text.NewEraseAction(v.buffer, r))
}

// Adds a Replace action of the given Region to the provided Edit object.
func (v *View) Replace(edit *Edit, r text.Region, value string) {
	edit.composite.AddExec(text.NewReplaceAction(v.buffer, r, value))
}

// Creates a new Edit object. Think of it a bit like starting an SQL transaction.
// Another Edit object should not be created before ending the previous one.
//
// TODO(.): Is nesting edits ever valid? Perhaps a nil edit should be returned if the previous wasn't ended?
// What if it will never be ended? Leaving the buffer in a broken state where no more changes can be made to
// it is obviously not good and is the reason why ST3 removed the ability to manually create Edit objects
// to stop people from breaking the undo stack.
func (v *View) BeginEdit() *Edit {
	e := newEdit(v)
	v.editstack = append(v.editstack, e)
	return e
}

// Ends the given Edit object.
func (v *View) EndEdit(edit *Edit) {
	if edit.invalid {
		// This happens when nesting Edits and the child Edit ends after the parent edit.
		log.Fine("This edit has already been invalidated: %v, %v", edit, v.editstack)
		return
	}

	// Find the position of this Edit object in this View's Edit stack.
	// If plugins, commands, etc are well-behaved the ended edit should be
	// last in the stack, but shit happens and we cannot count on this being the case.
	i := len(v.editstack) - 1
	for i := len(v.editstack) - 1; i >= 0; i-- {
		if v.editstack[i] == edit {
			break
		}
	}
	if i == -1 {
		// TODO(.): Under what instances does this happen again?
		log.Error("This edit isn't even in the stack... where did it come from? %v, %v", edit, v.editstack)
		return
	}

	if l := len(v.editstack) - 1; i != l {
		// TODO(.): See TODO in BeginEdit
		log.Error("This edit wasn't last in the stack... %d !=  %d: %v, %v", i, l, edit, v.editstack)
	}

	// Invalidate all Edits "below" and including this Edit.
	for j := len(v.editstack) - 1; j >= i; j-- {
		current_edit := v.editstack[j]
		current_edit.invalid = true
		sel_same := reflect.DeepEqual(*v.Sel(), current_edit.savedSel)
		buf_same := v.ChangeCount() == current_edit.savedCount
		eq := (sel_same && buf_same && current_edit.composite.Len() == 0)
		if v.IsScratch() || current_edit.bypassUndo || eq {
			continue
		}
		switch {
		case i == 0:
			// Well-behaved, no nested edits!
			fallthrough
		case j != i:
			// BOO! Someone began another Edit without finishing the first one.
			// In this instance, the parent Edit ended before the child.
			// TODO(.): What would be the correct way to handle this?
			v.undoStack.Add(edit)
		default:
			// BOO! Also poorly-behaved. This Edit object began after the parent began,
			// but was finished before the parent finished.
			//
			// Add it as a child of the parent Edit so that undoing the parent
			// will undo this edit as well.
			v.editstack[i-1].composite.Add(current_edit)
		}
	}
	// Pop this Edit and all the children off the Edit stack.
	v.editstack = v.editstack[:i]
}

// Sets the scratch property of the view.
// TODO(.): Couldn't this just be a value in the View's Settings?
func (v *View) SetScratch(s bool) {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.scratch = s
}

// Checks the scratch property of the view.
// TODO(.): Couldn't this just be a value in the View's Settings?
func (v *View) IsScratch() bool {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.scratch
}

// Sets the overwrite status property of the view.
// TODO(.): Couldn't this just be a value in the View's Settings?
func (v *View) OverwriteStatus() bool {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.overwrite
}

// Checks the overwrite status property of the view.
// TODO(.): Couldn't this just be a value in the View's Settings?
func (v *View) SetOverwriteStatus(s bool) {
	v.lock.Lock()
	defer v.lock.Unlock()
	v.overwrite = s
}

// Returns whether the underlying Buffer has any unsaved modifications.
// Note that Scratch buffers are never considered dirty.
func (v *View) IsDirty() bool {
	if v.IsScratch() {
		return false
	}
	lastSave := v.Settings().Int("lime.last_save_change_count", -1)
	return v.ChangeCount() != lastSave
}

func (v *View) FileChanged(filename string) {
	log.Finest("Reloading %s", filename)

	if saving := v.Settings().Bool("lime.saving", false); saving {
		// This reload was triggered by ourselves saving to this file, so don't reload it
		return
	}
	if !GetEditor().Frontend().OkCancelDialog("File was changed by another program, reload?", "reload") {
		return
	}

	if d, err := ioutil.ReadFile(filename); err != nil {
		log.Error("Could not read file: %s\n. Error was: %v", filename, err)
	} else {
		edit := v.BeginEdit()
		end := v.Size()
		v.Replace(edit, text.Region{0, end}, string(d))
		v.EndEdit(edit)
	}
}

// Saves the file
func (v *View) Save() error {
	return v.SaveAs(v.FileName())
}

// Saves the file to the specified filename
func (v *View) SaveAs(name string) (err error) {
	log.Fine("SaveAs(%s)", name)
	v.Settings().Set("lime.saving", true)
	defer v.Settings().Erase("lime.saving")
	OnPreSave.Call(v)
	if atomic := v.Settings().Bool("atomic_save", true); v.FileName() == "" || !atomic {
		if err := v.nonAtomicSave(name); err != nil {
			return err
		}
	} else {
		n, err := ioutil.TempDir(path.Dir(v.FileName()), "lime")
		if err != nil {
			return err
		}
		tmpf := path.Join(n, "tmp")
		if err := v.nonAtomicSave(tmpf); err != nil {
			return err
		}
		if err := os.Rename(tmpf, name); err != nil {
			// When we want to save as a file in another directory
			// we can't go with os.Rename so we need to force
			// not atomic saving sometimes as 4th test in TestSaveAsOpenFile
			if err := v.nonAtomicSave(name); err != nil {
				return err
			}
		}
		if err := os.RemoveAll(n); err != nil {
			return err
		}
	}

	ed := GetEditor()
	if fn := v.FileName(); fn != name {
		v.SetFileName(name)
		if fn != "" {
			ed.UnWatch(fn, v)
		}
		ed.Watch(name, v)
	}

	v.Settings().Set("lime.last_save_change_count", v.ChangeCount())
	OnPostSave.Call(v)
	return nil
}

func (v *View) nonAtomicSave(name string) error {
	data := []byte(v.Substr(text.Region{0, v.Size()}))
	if err := ioutil.WriteFile(name, data, 0644); err != nil {
		return err
	}
	return nil
}

// Returns the CommandHistory entry at the given relative index.
//
// When "modifying_only" is set to true, only commands that actually changed
// the Buffer in some way (as opposed to just moving the cursor around) are counted as
// an index. That would be a "hard" command as it is referred to in UndoStack.Undo.
func (v *View) CommandHistory(idx int, modifying_only bool) (name string, args Args, count int) {
	// TODO(.): merge history when possible
	if i := v.undoStack.index(idx, modifying_only); i != -1 {
		e := v.undoStack.actions[i]
		return e.command, e.args, 1
	}
	return "", nil, 0
}

func (v *View) runCommand(cmd TextCommand, name string) error {
	e := v.BeginEdit()
	e.command = name
	//	e.args = args
	e.bypassUndo = cmd.BypassUndo()

	defer func() {
		v.EndEdit(e)
		if r := recover(); r != nil {
			log.Error("Paniced while running text command %s %v: %v\n%s", name, cmd, r, string(debug.Stack()))
		}
	}()
	p := util.Prof.Enter("view.cmd." + name)
	defer p.Exit()
	return cmd.Run(v, e)
}

// AddRegions lets users mark text regions in a view with a scope name, gutter icon and ViewRegionflags
// which are then optionally used to alter the display of those regions.
//
// Typical uses would be to draw squiggly lines under misspelled words, show an icon in the gutter to
// indicate a breakpoint, keeping track of snippet or auto-completion fields, highlight code compilation
// warnings, etc.
//
// The regions will be automatically adjusted as appropriate when the underlying buffer is changed.
func (v *View) AddRegions(key string, regions []text.Region, scope, icon string, flags render.ViewRegionFlags) {
	vr := render.ViewRegions{Scope: scope, Icon: icon, Flags: flags}
	vr.Regions.AddAll(regions)

	v.lock.Lock()
	defer v.lock.Unlock()
	v.regions[key] = vr
}

// Returns the Regions associated with the given key.
func (v *View) GetRegions(key string) (ret []text.Region) {
	v.lock.Lock()
	defer v.lock.Unlock()
	vr := v.regions[key]
	rs := vr.Regions.Regions()
	ret = make([]text.Region, len(rs))
	copy(ret, rs)
	return
}

// Removes the Regions associated with the given key from the view.
func (v *View) EraseRegions(key string) {
	v.lock.Lock()
	defer v.lock.Unlock()
	delete(v.regions, key)
}

// Returns the UndoStack of this view. Tread lightly.
func (v *View) UndoStack() *UndoStack {
	return &v.undoStack
}

// Transform() takes a viewport, gets a colour scheme from editor and
// returns a Recipe suitable for rendering the contents of this View
// that is visible in that viewport.
func (v *View) Transform(viewport text.Region) render.Recipe {
	pe := util.Prof.Enter("view.Transform")
	defer pe.Exit()
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.syntax == nil {
		return nil
	}
	cs := v.Settings().String("color_scheme", "")
	scheme := ed.GetColorScheme(cs)
	rr := make(render.ViewRegionMap)
	for k, v := range v.regions {
		rr[k] = *v.Clone()
	}
	rs := render.ViewRegions{Flags: render.SELECTION}
	rs.Regions.AddAll(v.Sel().Regions())
	rr["lime.selection"] = rs
	return render.Transform(scheme, rr, viewport)
}

func (v *View) cleanup() {
	v.lock.Lock()
	defer v.lock.Unlock()

	// TODO(.): There can be multiple views into a single Buffer,
	// need to do some reference counting to see when it should be
	// closed
	v.buffer.Close()
	v.buffer = nil
}

func (v *View) isClosed() bool {
	return v.reparseChan == nil
}

// Initiate the "close" operation of this view.
// Returns "true" if the view was closed. Otherwise returns "false".
func (v *View) Close() bool {
	OnPreClose.Call(v)
	if v.IsDirty() {
		close_anyway := GetEditor().Frontend().OkCancelDialog("File has been modified since last save, close anyway?", "Yes")
		if !close_anyway {
			return false
		}
	}
	if n := v.FileName(); n != "" {
		GetEditor().UnWatch(n, v)
	}

	// Call the event first while there's still access possible to the underlying
	// buffer
	OnClose.Call(v)

	v.window.remove(v)

	// Closing the reparseChan, and setting to nil will eventually clean up other resources
	// when the parseThread exits
	v.lock.Lock()
	defer v.lock.Unlock()
	close(v.reparseChan)
	v.reparseChan = nil

	return true
}

const (
	CLASS_WORD_START = 1 << iota
	CLASS_WORD_END
	CLASS_PUNCTUATION_START
	CLASS_PUNCTUATION_END
	CLASS_SUB_WORD_START
	CLASS_SUB_WORD_END
	CLASS_LINE_START
	CLASS_LINE_END
	CLASS_EMPTY_LINE
	CLASS_MIDDLE_WORD
	CLASS_WORD_START_WITH_PUNCTUATION
	CLASS_WORD_END_WITH_PUNCTUATION
	CLASS_OPENING_PARENTHESIS
	CLASS_CLOSING_PARENTHESIS

	DEFAULT_SEPARATORS = "[!\"#$%&'()*+,\\-./:;<=>?@\\[\\\\\\]^`{|}~]"
)

// Classifies point, returning a bitwise OR of zero or more of defined flags
func (v *View) Classify(point int) (res int) {
	var a, b string = "", ""
	ws := v.Settings().String("word_separators", DEFAULT_SEPARATORS)
	if point > 0 {
		a = v.Substr(text.Region{point - 1, point})
	}
	if point < v.Size() {
		b = v.Substr(text.Region{point, point + 1})
	}

	// Out of range
	if v.Size() == 0 || point < 0 || point > v.Size() {
		res = 3520
		return
	}

	// If before and after the point are separators return 0
	if re, err := rubex.Compile(ws); err != nil {
		log.Error(err)
	} else if a == b && re.MatchString(a) {
		res = 0
		return
	}

	// SubWord start & end
	if re, err := rubex.Compile("[A-Z]"); err != nil {
		log.Error(err)
	} else {
		if re.MatchString(b) && !re.MatchString(a) {
			res |= CLASS_SUB_WORD_START
			res |= CLASS_SUB_WORD_END
		}
	}
	if a == "_" && b != "_" {
		res |= CLASS_SUB_WORD_START
	}
	if b == "_" && a != "_" {
		res |= CLASS_SUB_WORD_END
	}

	// Punc start & end
	if re, err := rubex.Compile(ws); err != nil {
		log.Error(err)
	} else {
		// Why ws != ""? See https://github.com/limetext/rubex/issues/2
		if ((re.MatchString(b) && ws != "") || b == "") && !(re.MatchString(a) && ws != "") {
			res |= CLASS_PUNCTUATION_START
		}
		if ((re.MatchString(a) && ws != "") || a == "") && !(re.MatchString(b) && ws != "") {
			res |= CLASS_PUNCTUATION_END
		}
		// Word start & end
		if re1, err := rubex.Compile("\\w"); err != nil {
			log.Error(err)
		} else if re2, err := rubex.Compile("\\s"); err != nil {
			log.Error(err)
		} else {
			if re1.MatchString(b) && ((re.MatchString(a) && ws != "") || re2.MatchString(a) || a == "") {
				res |= CLASS_WORD_START
			}
			if re1.MatchString(a) && ((re.MatchString(b) && ws != "") || re2.MatchString(b) || b == "") {
				res |= CLASS_WORD_END
			}
		}
	}

	// Line start & end
	if a == "\n" || a == "" {
		res |= CLASS_LINE_START
	}
	if b == "\n" || b == "" {
		res |= CLASS_LINE_END
		if ws == "" {
			res |= CLASS_WORD_END
		}
	}

	// Empty line
	if (a == "\n" && b == "\n") || (a == "" && b == "") {
		res |= CLASS_EMPTY_LINE
	}
	// Middle word
	if re, err := rubex.Compile("\\w"); err != nil {
		log.Error(err)
	} else {
		if re.MatchString(a) && re.MatchString(b) {
			res |= CLASS_MIDDLE_WORD
		}
	}

	// Word start & end with punc
	if re, err := rubex.Compile("\\s"); err != nil {
		log.Error(err)
	} else {
		if (res&CLASS_PUNCTUATION_START != 0) && (re.MatchString(a) || a == "") {
			res |= CLASS_WORD_START_WITH_PUNCTUATION
		}
		if (res&CLASS_PUNCTUATION_END != 0) && (re.MatchString(b) || b == "") {
			res |= CLASS_WORD_END_WITH_PUNCTUATION
		}
	}

	// Openning & closing parentheses
	if re, err := rubex.Compile("[(\\[{]"); err != nil {
		log.Error(err)
	} else {
		if re.MatchString(a) || re.MatchString(b) {
			res |= CLASS_OPENING_PARENTHESIS
		}
	}
	if re, err := rubex.Compile("[)\\]}]"); err != nil {
		log.Error(err)
	} else {
		if re.MatchString(a) || re.MatchString(b) {
			res |= CLASS_CLOSING_PARENTHESIS
		}
	}
	// TODO: isn't this a bug? what's the relation between
	// ',' and parentheses
	if a == "," {
		res |= CLASS_OPENING_PARENTHESIS
	}
	if b == "," {
		res |= CLASS_CLOSING_PARENTHESIS
	}

	return
}

// Finds the next location after point that matches the given classes
// Searches backward if forward is false
func (v *View) FindByClass(point int, forward bool, classes int) int {
	i := -1
	if forward {
		i = 1
	}
	size := v.Size()
	// Sublime doesn't consider initial point even if it matches.
	for p := point + i; ; p += i {
		if p <= 0 {
			return 0
		}
		if p >= size {
			return size
		}
		if v.Classify(p)&classes != 0 {
			return p
		}
	}
}

// Expands the selection until the point on each side matches the given classes
func (v *View) ExpandByClass(r text.Region, classes int) text.Region {
	// Sublime doesn't consider the points the region starts on.
	// If not already on edge of buffer, expand by 1 in both directions.
	a := r.A
	if a > 0 {
		a -= 1
	} else if a < 0 {
		a = 0
	}

	b := r.B
	size := v.Size()
	if b < size {
		b += 1
	} else if b > size {
		b = size
	}

	for ; a > 0 && (v.Classify(a)&classes == 0); a -= 1 {
	}
	for ; b < size && (v.Classify(b)&classes == 0); b += 1 {
	}
	return text.Region{a, b}
}

const (
	LITERAL = 1 << iota
	IGNORECASE
)

func (v *View) Find(pat string, pos int, flags int) text.Region {
	r := text.Region{pos, v.Size()}
	s := v.Substr(r)

	if flags&LITERAL != 0 {
		pat = "\\Q" + pat
	}
	if flags&IGNORECASE != 0 {
		pat = "(?im)" + pat
	} else {
		pat = "(?m)" + pat
	}
	// Using regexp instead of rubex because rubex doesn't
	// support flag for treating pattern as a literal text
	if re, err := regexp.Compile(pat); err != nil {
		log.Error(err)
	} else if loc := re.FindStringIndex(s); loc != nil {
		return text.Region{pos + loc[0], pos + loc[1]}
	}
	return text.Region{-1, -1}
}

func (v *View) Status() map[string]string {
	m := make(map[string]string)
	v.lock.Lock()
	defer v.lock.Unlock()

	for k, v := range v.status {
		m[k] = v
	}
	return m
}

func (v *View) SetStatus(key, val string) {
	v.lock.Lock()
	v.status[key] = val
	v.lock.Unlock()
	OnStatusChanged.Call(v)
}

func (v *View) GetStatus(key string) string {
	v.lock.Lock()
	defer v.lock.Unlock()
	return v.status[key]
}

func (v *View) EraseStatus(key string) {
	v.lock.Lock()
	delete(v.status, key)
	v.lock.Unlock()
	OnStatusChanged.Call(v)
}

func (v *View) SetSyntaxFile(file string) {
	v.Settings().Set("syntax", file)
}

func (v *View) ChangeCount() int {
	return v.buffer.ChangeCount()
}

func (v *View) FileName() string {
	return v.buffer.FileName()
}

func (v *View) Substr(r text.Region) string {
	return v.buffer.Substr(r)
}

func (v *View) SubstrR(r text.Region) []rune {
	return v.buffer.SubstrR(r)
}

func (v *View) FullLine(off int) text.Region {
	return v.buffer.FullLine(off)
}

func (v *View) FullLineR(r text.Region) text.Region {
	return v.buffer.FullLineR(r)
}

func (v *View) BufferId() text.Id {
	return v.buffer.Id()
}

func (v *View) Line(off int) text.Region {
	return v.buffer.Line(off)
}

func (v *View) LineR(r text.Region) text.Region {
	return v.buffer.LineR(r)
}

func (v *View) Lines(r text.Region) []text.Region {
	return v.buffer.Lines(r)
}

func (v *View) SetFileName(n string) error {
	if err := v.buffer.SetFileName(n); err != nil {
		return err
	}
	if ext := path.Ext(n); ext != "" {
		if file := GetEditor().fileTypeSyntax(ext[1:]); file != "" {
			v.SetSyntaxFile(file)
		}
	}
	return nil
}

func (v *View) Name() string {
	return v.buffer.Name()
}

func (v *View) SetName(n string) error {
	return v.buffer.SetName(n)
}

func (v *View) RowCol(point int) (int, int) {
	return v.buffer.RowCol(point)
}

func (v *View) TextPoint(row, col int) int {
	return v.buffer.TextPoint(row, col)
}

func (v *View) Size() int {
	return v.buffer.Size()
}

func (v *View) Word(off int) text.Region {
	return v.buffer.Word(off)
}

func (v *View) WordR(r text.Region) text.Region {
	return v.buffer.WordR(r)
}

func (v *View) AddObserver(ob text.BufferObserver) error {
	return v.buffer.AddObserver(ob)
}
