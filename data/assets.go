// Code generated by go-bindata.
// sources:
// data.go
// indexOf.mustache
// DO NOT EDIT!

package data

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func bindataRead(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, gz)
	clErr := gz.Close()

	if err != nil {
		return nil, fmt.Errorf("Read %q: %v", name, err)
	}
	if clErr != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

type asset struct {
	bytes []byte
	info  os.FileInfo
}

type bindataFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
}

func (fi bindataFileInfo) Name() string {
	return fi.name
}
func (fi bindataFileInfo) Size() int64 {
	return fi.size
}
func (fi bindataFileInfo) Mode() os.FileMode {
	return fi.mode
}
func (fi bindataFileInfo) ModTime() time.Time {
	return fi.modTime
}
func (fi bindataFileInfo) IsDir() bool {
	return false
}
func (fi bindataFileInfo) Sys() interface{} {
	return nil
}

var _dataGo = []byte("\x1f\x8b\x08\x00\x00\x09\x6e\x88\x00\xff\x2a\x48\x4c\xce\x4e\x4c\x4f\x55\x48\x49\x2c\x49\xe4\xe2\xd2\xd7\x4f\xcf\xb7\x4a\x4f\xcd\x4b\x2d\x4a\x2c\x49\x55\x48\xcf\xd7\x4d\xca\xcc\x03\xc9\x28\xe8\x16\x64\xa7\x2b\xa8\xb8\xfb\x07\x38\x3a\x7b\x3b\xba\xbb\x2a\xe8\xe6\x2b\x24\x16\x17\xa7\x96\x14\xeb\xa5\xe7\x2b\xe8\x71\x01\x02\x00\x00\xff\xff\x54\x60\x2c\x9e\x46\x00\x00\x00")

func dataGoBytes() ([]byte, error) {
	return bindataRead(
		_dataGo,
		"data.go",
	)
}

func dataGo() (*asset, error) {
	bytes, err := dataGoBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "data.go", size: 70, mode: os.FileMode(420), modTime: time.Unix(1473403985, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

var _indexofMustache = []byte("\x1f\x8b\x08\x00\x00\x09\x6e\x88\x00\xff\x6c\x8f\xc1\x6a\x86\x30\x10\x84\xcf\xe6\x29\xd2\xff\x7f\x80\xe5\xbf\x6f\x73\x12\xc1\x4b\xe9\xa1\xe7\x42\xda\x44\x22\x68\x15\xdd\x43\x65\xc8\xbb\x97\xd4\x44\x2d\xf4\xb6\x93\x9d\xf9\x76\xc2\x4f\x6e\xfa\x94\x6d\xf6\x3a\xc8\x38\x18\xc5\xc1\x5b\x67\x54\xc5\xd2\xcb\xe0\x4d\xfb\xe5\xfc\xb7\x9e\x3a\x0d\xbc\x5a\x09\x31\x32\xed\x0b\xc5\xb4\x3b\x39\x3c\xfe\x73\x85\x87\x51\x2c\xf6\x23\x59\x2b\x96\x4c\xad\x58\x9c\x79\xdb\x66\xcf\x24\x87\x7e\xb1\xe3\x1f\xbd\x06\x9b\x25\x53\x09\x02\xf7\xa6\x1f\xfc\x1a\x63\xa2\x2d\xc5\x0a\xdc\xdb\xb5\xee\x97\x18\x6b\x80\xf2\x08\xbc\xe7\xa9\x39\x1f\xaf\x07\xd8\xea\xb0\xf8\xee\xf9\x06\xa4\xdb\x29\x51\x30\x74\x26\x6e\xa6\xac\x99\xac\xb9\xe6\x81\xb6\x3e\x88\x4c\xbf\x75\x00\x2a\x05\x99\xf2\xbf\x7f\x02\x00\x00\xff\xff\x99\xd1\xbc\xa5\x5c\x01\x00\x00")

func indexofMustacheBytes() ([]byte, error) {
	return bindataRead(
		_indexofMustache,
		"indexOf.mustache",
	)
}

func indexofMustache() (*asset, error) {
	bytes, err := indexofMustacheBytes()
	if err != nil {
		return nil, err
	}

	info := bindataFileInfo{name: "indexOf.mustache", size: 348, mode: os.FileMode(420), modTime: time.Unix(1473403263, 0)}
	a := &asset{bytes: bytes, info: info}
	return a, nil
}

// Asset loads and returns the asset for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func Asset(name string) ([]byte, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("Asset %s can't read by error: %v", name, err)
		}
		return a.bytes, nil
	}
	return nil, fmt.Errorf("Asset %s not found", name)
}

// MustAsset is like Asset but panics when Asset would return an error.
// It simplifies safe initialization of global variables.
func MustAsset(name string) []byte {
	a, err := Asset(name)
	if err != nil {
		panic("asset: Asset(" + name + "): " + err.Error())
	}

	return a
}

// AssetInfo loads and returns the asset info for the given name.
// It returns an error if the asset could not be found or
// could not be loaded.
func AssetInfo(name string) (os.FileInfo, error) {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	if f, ok := _bindata[cannonicalName]; ok {
		a, err := f()
		if err != nil {
			return nil, fmt.Errorf("AssetInfo %s can't read by error: %v", name, err)
		}
		return a.info, nil
	}
	return nil, fmt.Errorf("AssetInfo %s not found", name)
}

// AssetNames returns the names of the assets.
func AssetNames() []string {
	names := make([]string, 0, len(_bindata))
	for name := range _bindata {
		names = append(names, name)
	}
	return names
}

// _bindata is a table, holding each asset generator, mapped to its name.
var _bindata = map[string]func() (*asset, error){
	"data.go":          dataGo,
	"indexOf.mustache": indexofMustache,
}

// AssetDir returns the file names below a certain
// directory embedded in the file by go-bindata.
// For example if you run go-bindata on data/... and data contains the
// following hierarchy:
//     data/
//       foo.txt
//       img/
//         a.png
//         b.png
// then AssetDir("data") would return []string{"foo.txt", "img"}
// AssetDir("data/img") would return []string{"a.png", "b.png"}
// AssetDir("foo.txt") and AssetDir("notexist") would return an error
// AssetDir("") will return []string{"data"}.
func AssetDir(name string) ([]string, error) {
	node := _bintree
	if len(name) != 0 {
		cannonicalName := strings.Replace(name, "\\", "/", -1)
		pathList := strings.Split(cannonicalName, "/")
		for _, p := range pathList {
			node = node.Children[p]
			if node == nil {
				return nil, fmt.Errorf("Asset %s not found", name)
			}
		}
	}
	if node.Func != nil {
		return nil, fmt.Errorf("Asset %s not found", name)
	}
	rv := make([]string, 0, len(node.Children))
	for childName := range node.Children {
		rv = append(rv, childName)
	}
	return rv, nil
}

type bintree struct {
	Func     func() (*asset, error)
	Children map[string]*bintree
}

var _bintree = &bintree{nil, map[string]*bintree{
	"data.go":          &bintree{dataGo, map[string]*bintree{}},
	"indexOf.mustache": &bintree{indexofMustache, map[string]*bintree{}},
}}

// RestoreAsset restores an asset under the given directory
func RestoreAsset(dir, name string) error {
	data, err := Asset(name)
	if err != nil {
		return err
	}
	info, err := AssetInfo(name)
	if err != nil {
		return err
	}
	err = os.MkdirAll(_filePath(dir, filepath.Dir(name)), os.FileMode(0755))
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(_filePath(dir, name), data, info.Mode())
	if err != nil {
		return err
	}
	err = os.Chtimes(_filePath(dir, name), info.ModTime(), info.ModTime())
	if err != nil {
		return err
	}
	return nil
}

// RestoreAssets restores an asset under the given directory recursively
func RestoreAssets(dir, name string) error {
	children, err := AssetDir(name)
	// File
	if err != nil {
		return RestoreAsset(dir, name)
	}
	// Dir
	for _, child := range children {
		err = RestoreAssets(dir, filepath.Join(name, child))
		if err != nil {
			return err
		}
	}
	return nil
}

func _filePath(dir, name string) string {
	cannonicalName := strings.Replace(name, "\\", "/", -1)
	return filepath.Join(append([]string{dir}, strings.Split(cannonicalName, "/")...)...)
}
