// Copyright 2014 The lime Authors.
// Use of this source code is governed by a 2-clause
// BSD-style license that can be found in the LICENSE file.

package packages

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestPlugin(t *testing.T) {
	tests := []struct {
		path   string
		suffix string
		files  []string
	}{
		{
			"testdata/Vintage",
			".py",
			[]string{"action_cmds.py", "state.py", "transformers.py"},
		},
	}
	for i, test := range tests {
		p := NewPlugin(test.path, test.suffix)
		p.Reload()
		if p.Name() != test.path {
			t.Errorf("Test %d: Expected plugin name %s but, got %s", i, test.path, p.Name())
		}
		for _, f := range test.files {
			found := false
			for _, fi := range p.Get().([]os.FileInfo) {
				if f == fi.Name() {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Test %d: Expected to find %s in %s plugin", i, f, p.Name())
			}
		}
	}
}

func TestPluginReload(t *testing.T) {
	p := NewPlugin("testdata/Closetag", ".vim")
	if err := ioutil.WriteFile("testdata/Closetag/test.vim", []byte("testing"), 0644); err != nil {
		t.Fatalf("Couldn't write file: %s", err)
	}
	p.Reload()
	fi := p.Get().([]os.FileInfo)
	found := false
	for _, f := range fi {
		if f.Name() == "test.vim" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find test.vim file in %s", p.Name())
	}
	os.Remove("testdata/Closetag/test.vim")
}

func TestScanPlugins(t *testing.T) {
	tests := []struct {
		path   string
		suffix string
		expect []string
	}{
		{
			"testdata/",
			".py",
			[]string{
				"testdata/Vintage",
			},
		},
		{
			"testdata/",
			".vim",
			[]string{
				"testdata/Closetag",
			},
		},
	}
	for i, test := range tests {
		plugins := ScanPlugins(test.path, test.suffix)
		for _, f := range test.expect {
			found := false
			for _, p := range plugins {
				if f == p.Name() {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Test %d: Expected ScanPlugins find %s plugin", i, f)
			}
		}
	}
}
