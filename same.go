package main

import (
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"io"
	"io/ioutil"
	"os"
	"path"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("You must specify at least one directory")
		return
	}
	if len(os.Args) == 2 {
		dirname := path.Clean(os.Args[1])
		fileinfo, err := os.Stat(dirname)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return
		}
		handle(dirname, fileinfo, 1)
		analyze(dirname+"/", false)
	} else {
		for i := 1; i < len(os.Args); i++ {
			dirname := path.Clean(os.Args[i])
			fileinfo, err := os.Stat(dirname)
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				return
			}
			handle(dirname, fileinfo, i)
		}
		analyze("", true)
	}
}

var levels []map[string]Item

type Item struct {
	Aliases []*File
}

type File struct {
	Lvl      int
	Path     string
	Filename string
	Size     int64
	Children []*File
	Group    int

	slowHash string
	slowErr  error
	fastHash string
}

func (f *File) SlowHash() (string, error) {
	if f.slowHash == "" && f.slowErr == nil {
		if f.Size >= 0 {
			hash, err := hashFile(f.Path)
			f.slowHash = string(hash)
			f.slowErr = err
		} else {
			h := sha512.New()
			for _, child := range f.Children {
				childHash, err := child.SlowHash()
				if err != nil {
					return "", err
				}
				binary.Write(h, binary.BigEndian, len(child.Filename))
				h.Write([]byte(child.Filename))
				h.Write([]byte(childHash))
			}
			f.slowHash = string(h.Sum([]byte{2}))
			f.slowErr = nil
		}
	}
	return f.slowHash, f.slowErr
}

func (f *File) FastHash() string {
	if f.fastHash == "" {
		h := fnv.New128()
		binary.Write(h, binary.BigEndian, f.Size)
		for _, child := range f.Children {
			binary.Write(h, binary.BigEndian, len(child.Filename))
			h.Write([]byte(child.Filename))
			h.Write([]byte(child.FastHash()))
		}
		f.fastHash = string(h.Sum(nil))
	}
	return f.fastHash
}

func hashFile(name string) ([]byte, error) {
	h := sha512.New()
	f, err := os.Open(name)
	defer f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return nil, err
	}
	_, err = io.Copy(h, f)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return nil, err
	}
	return h.Sum([]byte{1}), nil
}

func handle(name string, fileinfo os.FileInfo, group int) *File {
	filename := fileinfo.Name()
	if fileinfo.IsDir() {
		lvl, files, err := scan(name, group)
		file := &File{
			Lvl:      lvl,
			Path:     name + "/",
			Filename: filename,
			Children: files,
			Size:     -1,
			slowErr:  err,
			Group:    group,
		}
		insertFile(file)
		return file
	}
	file := &File{
		Lvl:      0,
		Path:     name,
		Filename: filename,
		Size:     fileinfo.Size(),
		Group:    group,
	}
	insertFile(file)
	return file
}

func scan(dirname string, group int) (int, []*File, error) {
	fileinfos, err := ioutil.ReadDir(dirname)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 0, nil, err
	}
	files := make([]*File, 0)
	maxlvl := 1
	for _, fileinfo := range fileinfos {
		name := path.Join(dirname, fileinfo.Name())
		file := handle(name, fileinfo, group)
		files = append(files, file)
		if file.Lvl+1 > maxlvl {
			maxlvl = file.Lvl + 1
		}
	}
	return maxlvl, files, nil
}

func analyze(dirname string, multi bool) {
	for i := len(levels) - 1; i >= 0; i-- {
		for _, item := range levels[i] {
			if len(item.Aliases) < 2 {
				continue
			}
			if multi {
				g := make(map[int]bool)
				for _, alias := range item.Aliases {
					g[alias.Group] = true
				}
				if len(g) < 2 {
					continue
				}
			}
			m := make(map[string]Item, len(item.Aliases))
			for _, file := range item.Aliases {
				hash2, err := file.SlowHash()
				if err != nil {
					continue
				}
				item2 := m[hash2]
				item2.Aliases = append(item2.Aliases, file)
				m[hash2] = item2
			}
			for _, item2 := range m {
				if len(item2.Aliases) < 2 {
					continue
				}
				if multi {
					g := make(map[int]bool)
					for _, alias := range item2.Aliases {
						g[alias.Group] = true
					}
					if len(g) < 2 {
						continue
					}
				}
				for _, file := range item2.Aliases {
					fmt.Printf("%s\n", file.Path[len(dirname):])
					removeFile(file)
				}
				fmt.Printf("\n")
			}
		}
	}
}

func insertFile(file *File) {
	// Empty directory?
	if file.Size == -1 && len(file.Children) == 0 {
		return
	}
	// Empty file?
	if file.Size == 0 {
		return
	}
	for len(levels) <= file.Lvl {
		levels = append(levels, make(map[string]Item))
	}
	item := levels[file.Lvl][file.FastHash()]
	item.Aliases = append(item.Aliases, file)
	levels[file.Lvl][file.FastHash()] = item
}

func removeFile(file *File) {
	for _, child := range file.Children {
		removeFile(child)
	}
	item := levels[file.Lvl][file.FastHash()]
	for i, alias := range item.Aliases {
		if file == alias {
			item.Aliases = append(item.Aliases[:i], item.Aliases[i+1:]...)
			break
		}
	}
	levels[file.Lvl][file.FastHash()] = item
}
