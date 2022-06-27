package build

import (
	"crypto/sha256"
	"encoding/base64"
	"io/ioutil"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

func CodeHash(root string) (string, error) {
	path, err := filepath.Abs(root)
	log.Infof("generating code hash %s", path)
	codeHash := sha256.New()

	data, err := ioutil.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		log.Debugf("could not read go.mod %s", err)
		return "", err
	}

	f, err := modfile.ParseLax(filepath.Join(root, "go.mod"), data, nil)
	if err != nil {
		log.Debugf("could parse read go.mod %s", err)
		return "", err
	}

	if f != nil {
		modules := make(map[module.Version]bool)
		for _, require := range f.Require {
			modules[require.Mod] = true
		}
		for _, replace := range f.Replace {
			delete(modules, replace.Old)
			modules[replace.New] = true
		}

		//do we have a guaranteed order here?
		for version := range modules {
			codeHash.Write([]byte(version.String()))
		}
	}

	files := make(map[string]struct{})
	hashAllGoFiles(".", files)

	for fname := range files {
		data, err := ioutil.ReadFile(fname)
		if err != nil {
			return "", err
		}
		codeHash.Write(data)
	}

	codeHashDigest := base64.StdEncoding.EncodeToString(codeHash.Sum(nil))

	return codeHashDigest, err

}

func hashAllGoFiles(fname string, paths map[string]struct{}) {
	files, err := ioutil.ReadDir(fname)
	if err == nil {
		for _, file := range files {
			if file.IsDir() {
				hashAllGoFiles(filepath.Join(fname, file.Name()), paths)
			} else {
				if strings.HasSuffix(file.Name(), ".go") {
					file := filepath.Join(fname, file.Name())
					paths[file] = struct{}{}
				}
			}
		}
	}
}
