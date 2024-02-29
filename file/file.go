package file

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/GannettDigital/vend/cli"
	"github.com/GannettDigital/vend/output"
)

// VendorDir represents a vendor directory.
type VendorDir struct {
	basePath       string
	modFileContent []byte
	mod            GoMod
	deps           []Dep
	filter         *regexp.Regexp
	debug          bool
	quiet          bool
}

// InitVendorDir creates a new vendor directory.
func InitVendorDir(options cli.Options) VendorDir {
	var filterRegexp *regexp.Regexp
	if options.Filter != "" {
		filterR, err := regexp.Compile(options.Filter)
		output.OnError(err, "Error compiling filter regular expression")
		filterRegexp = filterR
	}

	wd, err := os.Getwd()
	output.OnError(err, "Error getting the current directory")

	basePath, err := filepath.Abs(options.BasePath)
	output.OnError(err, "Error making absolute base directory")

	if basePath == wd || !strings.HasPrefix(basePath, wd) {
		output.Fatal("Output path (%q) must be a subdirectory of the current directory (%q).", basePath, wd)
	}

	return VendorDir{
		basePath: basePath,
		mod:      ParseModJSON(cli.ReadModJSON()),
		deps:     ParseDownloadJSON(cli.ReadDownloadJSON()),
		filter:   filterRegexp,
		debug:    options.Debug,
		quiet:    options.Quiet,
	}
}

// CopyDependencies copies remote module level dependencies transitively.
func (v *VendorDir) CopyDependencies() {
	// Check that go.mod is newer than the vendored directory, to know if work is to be done.
	// This cannot know if replaced modules have changed so skips the check if one is present.
	// Also skips check if using "vendor" directory since "go mod vendor" will have made changes.
	if !strings.HasSuffix(v.basePath, "/vendor") && v.mod.Replace == nil {
		goModPath := "go.mod"
		if envGoModPath := os.Getenv("GOMOD"); envGoModPath != "" {
			goModPath = envGoModPath
		}
		statGoMod, err := os.Stat(goModPath)
		output.OnError(err, "Error stat'ing go.mod file")
		statBasePath, err := os.Stat(v.basePath)
		if err != nil {
			if !os.IsNotExist(err) {
				output.OnError(err, "Error stat'ing base directory")
			}
		} else if statBasePath.ModTime().After(statGoMod.ModTime()) {
			if !v.quiet {
				fmt.Println("vend: no module changes since last run")
			}
			return
		}
	}

	v.clear()

	for _, d := range v.deps {
		if !v.quiet {
			fmt.Printf("vend: copying %s (%s)\n", d.Path, d.Version)
		}
		copied := v.copy(d.Dir, v.vendPath(d.Path))
		if !copied && v.filter != nil {
			// This ignores errors because some parts of the path (especially the
			// hostname) might be shared between multiple, un-related packages.
			for p := d.Path; p != "."; p = filepath.Dir(p) {
				if v.debug {
					fmt.Fprintf(os.Stderr, "pruning: %s\n", p)
				}
				err := v.remove(v.vendPath(p))
				if err != nil {
					if os.IsNotExist(err) {
						continue
					} else if errors.Is(err, syscall.ENOTEMPTY) {
						break
					}
					output.OnError(err, "Error removing path")
				}
			}
		}
	}

	for _, r := range v.mod.Replace {
		if r.Old.Path != r.New.Path {
			if !v.quiet {
				fmt.Printf("vend: replacing %s with %s\n", r.Old.Path, r.New.Path)
			}
			newPath := v.vendPath(r.New.Path)
			oldPath := v.vendPath(r.Old.Path)
			// If the directory is in the vendor folder it was copied from the
			// module cache so we can just rename it. Otherwise it's a local
			// directory located somewhere else that needs copying in.
			if v.exists(newPath) {
				v.copy(newPath, oldPath)
				v.removeAll(newPath)
			} else {
				v.copy(r.New.Path, oldPath)
			}
		}
	}
}

// exists checks if a file exists.
func (v *VendorDir) exists(file string) bool {
	_, err := os.Stat(file)
	return !os.IsNotExist(err)
}

// removeAll recursively removes a path.
func (v *VendorDir) removeAll(p string) {
	if len(p) < len(v.basePath) {
		output.Fatal("cannot remove path that is ancestor of base path: %s vs. %s", p, v.basePath)
	}
	if v.debug {
		fmt.Fprintf(os.Stderr, "removing all: %s\n", p)
	}
	err := os.RemoveAll(p)
	output.OnError(err, "Error removing path")
}

// remove removes a path.
func (v *VendorDir) remove(p string) error {
	if v.debug {
		fmt.Fprintf(os.Stderr, "removing: %s\n", p)
	}
	return os.Remove(p)
}

// vendPath creates a vendor directory path.
func (v *VendorDir) vendPath(p string) string {
	return path.Join(v.basePath, p)
}

// copyModFile internally copies and saves the modules.txt file.
func (v *VendorDir) copyModFile() {
	var err error
	v.modFileContent, err = ioutil.ReadFile(v.vendPath("modules.txt"))
	output.OnError(err, "Error reading modules.txt")
}

// writeModFile writes the modules.txt file into the vendor directory.
func (v *VendorDir) writeModFile() {
	err := ioutil.WriteFile(v.vendPath("modules.txt"), v.modFileContent, 0644)
	output.OnError(err, "Error saving modules.txt")
}

// clear removes all dependencies from the vendor directory.
func (v *VendorDir) clear() {
	haveModFile := v.exists(v.vendPath("modules.txt"))

	if haveModFile {
		v.copyModFile()
	}
	v.removeAll(v.basePath)

	err := os.MkdirAll(v.basePath, 0755)
	output.OnError(err, "Error creating vendor directory")

	if haveModFile {
		v.writeModFile()
	}
}

// copy will copy files and directories.
func (v *VendorDir) copy(src string, dest string) bool {
	info, err := os.Lstat(src)
	output.OnError(err, "Error getting information about source")

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return false // Completely ignore symlinks.
	case info.IsDir():
		return v.copyDirectory(src, dest)
	case v.filter == nil || v.filter.MatchString(dest):
		return v.copyFile(src, dest)
	default:
		return false
	}
}

// copyDirectory will copy directories.
func (v *VendorDir) copyDirectory(src string, dest string) bool {
	err := os.MkdirAll(dest, 0755)
	output.OnError(err, "Error creating directories")

	contents, err := ioutil.ReadDir(src)
	output.OnError(err, "Error reading source directory")

	var copied bool
	for _, content := range contents {
		s := filepath.Join(src, content.Name())
		d := filepath.Join(dest, content.Name())
		if v.copy(s, d) {
			copied = true
		}
	}

	if !copied && v.filter != nil {
		err := v.remove(dest)
		output.OnError(err, "Error removing path")
	}
	return copied
}

// copyFile will copy files.
func (v *VendorDir) copyFile(src string, dest string) bool {
	err := os.MkdirAll(filepath.Dir(dest), 0755)
	output.OnError(err, "Error creating directories")

	d, err := os.Create(dest)
	output.OnError(err, "Error creating file")
	defer d.Close()

	s, err := os.Open(src)
	output.OnError(err, "Error opening file")
	defer s.Close()

	_, err = io.Copy(d, s)
	output.OnError(err, "Error copying file")

	return true
}
