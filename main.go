package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/gogo/protobuf/proto"
	"v2ray.com/core/app/router"
)

type Entry struct {
	Type  string
	Value string
}

type List struct {
	Name  string
	Entry []Entry
}

type ParsedList struct {
	Name      string
	Inclusion map[string]bool
	Entry     []Entry
}

func (l *ParsedList) toProto() (*router.GeoSite, error) {
	site := &router.GeoSite{
		CountryCode: l.Name,
	}
	for _, entry := range l.Entry {
		switch entry.Type {
		case "domain":
			site.Domain = append(site.Domain, &router.Domain{
				Type:  router.Domain_Domain,
				Value: entry.Value,
			})
		case "regex":
			site.Domain = append(site.Domain, &router.Domain{
				Type:  router.Domain_Regex,
				Value: entry.Value,
			})
		case "keyword":
			site.Domain = append(site.Domain, &router.Domain{
				Type:  router.Domain_Plain,
				Value: entry.Value,
			})
		case "full":
			site.Domain = append(site.Domain, &router.Domain{
				Type:  router.Domain_Full,
				Value: entry.Value,
			})
		default:
			return nil, errors.New("unknown domain type: " + entry.Type)
		}
	}
	return site, nil
}

func removeComment(line string) string {
	idx := strings.Index(line, "#")
	if idx == -1 {
		return line
	}
	return strings.TrimSpace(line[:idx])
}

func parseEntry(line string) (Entry, error) {
	kv := strings.Split(line, ":")
	if len(kv) == 1 {
		return Entry{
			Type:  "domain",
			Value: kv[0],
		}, nil
	}
	if len(kv) == 2 {
		return Entry{
			Type:  strings.ToLower(kv[0]),
			Value: strings.ToLower(kv[1]),
		}, nil
	}
	return Entry{}, errors.New("Invalid format: " + line)
}

func DetectPath(path string) (string, error) {
	arrPath := strings.Split(path, string(filepath.ListSeparator))
	for _, content := range arrPath {
		fullPath := filepath.Join(content, "src", "github.com", "v2ray", "domain-list-community", "data")
		_, err := os.Stat(fullPath)
		if err == nil || os.IsExist(err) {
			return fullPath, nil
		}
	}
	err := errors.New("No file found in GOPATH")
	return "", err
}

func Load(path string) (*List, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	list := &List{
		Name: strings.ToUpper(filepath.Base(path)),
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		line = removeComment(line)
		if len(line) == 0 {
			continue
		}
		entry, err := parseEntry(line)
		if err != nil {
			return nil, err
		}
		list.Entry = append(list.Entry, entry)
	}

	return list, nil
}

func ParseList(list *List, ref map[string]*List) (*ParsedList, error) {
	pl := &ParsedList{
		Name:      list.Name,
		Inclusion: make(map[string]bool),
	}
	entryList := list.Entry
	for {
		newEntryList := make([]Entry, 0, len(entryList))
		hasInclude := false
		for _, entry := range entryList {
			if entry.Type == "include" {
				if pl.Inclusion[entry.Value] {
					continue
				}
				refName := strings.ToUpper(entry.Value)
				pl.Inclusion[refName] = true
				r := ref[refName]
				if r == nil {
					return nil, errors.New(entry.Value + " not found.")
				}
				newEntryList = append(newEntryList, r.Entry...)
				hasInclude = true
			} else {
				newEntryList = append(newEntryList, entry)
			}
		}
		entryList = newEntryList
		if !hasInclude {
			break
		}
	}
	pl.Entry = entryList

	return pl, nil
}

func main() {
	dir, err := DetectPath(os.Getenv("GOPATH"))
	if err != nil {
		fmt.Println("Failed: ", err)
		return
	}
	ref := make(map[string]*List)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		list, err := Load(path)
		if err != nil {
			return err
		}
		ref[list.Name] = list
		return nil
	})
	if err != nil {
		fmt.Println("Failed: ", err)
		return
	}
	protoList := new(router.GeoSiteList)
	for _, list := range ref {
		pl, err := ParseList(list, ref)
		if err != nil {
			fmt.Println("Failed: ", err)
			return
		}
		site, err := pl.toProto()
		if err != nil {
			fmt.Println("Failed: ", err)
			return
		}
		protoList.Entry = append(protoList.Entry, site)
	}

	protoBytes, err := proto.Marshal(protoList)
	if err != nil {
		fmt.Println("Failed:", err)
		return
	}
	if err := ioutil.WriteFile("dlc.dat", protoBytes, 0777); err != nil {
		fmt.Println("Failed: ", err)
	}
}
