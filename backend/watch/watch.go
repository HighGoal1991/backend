package watch

import (
	"code.google.com/p/log4go"
	"github.com/howeyc/fsnotify"
	"os"
	"path/filepath"
	"sync"
)

type Watcher struct {
	wchr     *fsnotify.Watcher
	watched  map[string][]func() // All watched paths including their actions
	watchers []string            // helper variable for paths we created watcher on
	dirs     []string            // helper variable for dirs we are watching
	lock     sync.Mutex
}

func NewWatcher() *Watcher {
	wchr, err := fsnotify.NewWatcher()
	if err != nil {
		log4go.Error("Could not create watcher due to: %s", err)
		return nil
	}
	watched := make(map[string][]func())
	watchers := make([]string, 0)
	dirs := make([]string, 0)

	return &Watcher{wchr: wchr, watched: watched, watchers: watchers, dirs: dirs}
}

func (w *Watcher) Watch(path string, action func()) {
	fi, err := os.Stat(path)
	dir := err == nil && fi.IsDir()
	// If the file doesn't exist currently we will add watcher for file
	// directory and look for create event inside the directory
	if !dir && os.IsNotExist(err) {
		w.Watch(filepath.Dir(path), nil)
	}
	// If the path points to a file and there is no action
	// Don't watch
	if !dir && action == nil {
		log4go.Error("No action for watching the file")
		return
	}
	w.lock.Lock()
	defer w.lock.Unlock()
	// If exists in watchers we are already watching the path
	// no need to watch again just adding the action
	if exist(w.watchers, path) {
		if action != nil {
			w.watched[path] = append(w.watched[path], action)
		}
		return
	}
	// If the file is under one of watched dirs
	// no need to create watcher
	if !dir && exist(w.dirs, filepath.Dir(path)) {
		w.watched[path] = append(w.watched[path], action)
		return
	}
	if err := w.wchr.Watch(path); err != nil {
		log4go.Error("Could not watch: %s", err)
		return
	}
	w.watchers = append(w.watchers, path)
	w.watched[path] = append(w.watched[path], action)
	if dir {
		w.dirs = append(w.dirs, path)
		for _, p := range w.watchers {
			if filepath.Dir(p) != path {
				continue
			}
			if err := w.wchr.RemoveWatch(p); err != nil {
				log4go.Error("Couldn't unwatch file: %s", err)
				continue
			}
			w.watchers = remove(w.watchers, p)
		}
	}
}

func (w *Watcher) UnWatch(path string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	if !exist(w.watchers, path) {
		return
	}
	if exist(w.dirs, path) {
		for p, _ := range w.watched {
			if filepath.Dir(p) == path && !exist(w.watchers, p) {
				if err := w.wchr.Watch(p); err != nil {
					log4go.Error("Could not watch: %s", err)
					return
				}
				w.watchers = append(w.watchers, p)
			}
		}
	}
	if err := w.wchr.RemoveWatch(path); err != nil {
		log4go.Error("Couldn't unwatch file: %s", err)
		return
	}
	w.watchers = remove(w.watchers, path)
	w.dirs = remove(w.dirs, path)
	delete(w.watched, path)
}

func (w *Watcher) Observe() {
	for {
		select {
		case ev := <-w.wchr.Event:
			func() {
				// The watcher will be removed if the file is deleted
				// so we need to watch the parent directory for when the
				// file is created again
				if ev.IsDelete() {
					w.watchers = remove(w.watchers, ev.Name)
					w.Watch(filepath.Dir(ev.Name), nil)
				}
				w.lock.Lock()
				defer w.lock.Unlock()
				actions, exist := w.watched[ev.Name]
				if !exist {
					return
				}
				for _, action := range actions {
					if action != nil {
						action()
					}
				}
				if !exist(w.dirs, ev.Name) {
					return
				}
				for p, actions := range w.watched {
					if filepath.Dir(p) == ev.Name && !exist(w.watchers, p) {
						for _, action := range actions {
							action()
						}
					}
				}
			}()
		case err := <-w.wchr.Error:
			log4go.Error("Watcher error: %s", err)
		}
	}
}

// Helper function checking an element exists in a slice
func exist(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

// Helper function for removing an element from slice
func remove(slice []string, path string) []string {
	for i, el := range slice {
		if el == path {
			slice[i], slice = slice[len(slice)-1], slice[:len(slice)-1]
			break
		}
	}
	return slice
}
