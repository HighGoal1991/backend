// Copyright 2016 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package packages

import (
	"io/ioutil"
	"path"

	"github.com/limetext/backend/log"
)

// A helper struct to implement File*Callback interfaces and
// watching all scaned directories for new packages
type scanDir struct {
	path string
}

// TODO: are we checking new folders to?
func (p *scanDir) FileCreated(name string) {
	record(name)
}

// watches scaned directory
func watchDir(dir string) {
	log.Finest("Watching scaned dir: %s", dir)
	sd := &scanDir{dir}
	if err := watcher.Watch(sd.path, sd); err != nil {
		log.Error("Couldn't watch %s: %s", sd.path, err)
	}
}

var loaded = make(map[string]Package)

func Scan(dir string) {
	log.Debug("Scanning %s for packages", dir)
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Error("Error while scanning %s: %s", dir, err)
	}
	watchDir(dir)

	var pkgs []Package
	for _, fi := range fis {
		pkgPath := path.Join(dir, fi.Name())
		if _, ok := loaded[pkgPath]; ok {
			continue
		}
		if pkg := record(pkgPath); pkg != nil {
			pkgs = append(pkgs, pkg)
		}
	}
	// TODO: we cant run this in a go routine because currently there is
	// no way to frontends to know when for example the color scheme
	// is ready
	func() {
		for _, pkg := range pkgs {
			load(pkg)
			loaded[pkg.Path()] = pkg
		}
	}()
}
