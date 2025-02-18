package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"
)

type Date time.Time

func (d Date) String() string {
	return time.Time(d).Format("2006-01-02T15:04:05Z")
}

func (d *Date) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	var v string
	dec.DecodeElement(&v, &start)
	t, err := time.Parse("2006-01-02T15:04:05.000-07:00", v)
	if err != nil {
		return err
	}
	*d = Date(t)
	return nil
}

type Draft bool

func (d *Draft) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	var v string
	dec.DecodeElement(&v, &start)
	switch v {
	case "yes":
		*d = true
		return nil
	case "no":
		*d = false
		return nil
	}
	return fmt.Errorf("Unknown value for draft boolean: %s", v)
}

type Reply struct {
	Rel    string `xml:"rel,attr"`
	Link   string `xml:"href,attr"`
	Source string `xml:"source,attr"`
}

type Image struct {
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
	Source string `xml:"src,attr"`
}

type Author struct {
	Name  string `xml:"name"`
	Uri   string `xml:"uri"`
	Image Image  `xml:"image"`
}

type Export struct {
	XMLName xml.Name `xml:"feed"`
	Entries []Entry  `xml:"entry"`
}

type Entry struct {
	ID        string  `xml:"id"`
	Published Date    `xml:"published"`
	Updated   Date    `xml:"updated"`
	Draft     Draft   `xml:"control>draft"`
	Title     string  `xml:"title"`
	Content   string  `xml:"content"`
	Tags      Tags    `xml:"category"`
	Author    Author  `xml:"author"`
	Source    Reply   `xml:"in-reply-to"`
	Links     []Reply `xml:"link"`
	Reply     uint64
	Children  []int
	Comments  []uint64
	Slug      string
	Extra     string
}

type Tag struct {
	Name   string `xml:"term,attr"`
	Scheme string `xml:"scheme,attr"`
}

type Tags []Tag
type EntrySet []int

func (t Tags) TomlString() string {
	names := []string{}
	for _, t := range t {
		if t.Scheme == "http://www.blogger.com/atom/ns#" {
			names = append(names, fmt.Sprintf("%q", t.Name))
		}
	}
	return strings.Join(names, ", ")
}

var tomlTempl = `+++
title = "{{ .Title }}"{{ if not (eq .Title .Slug) }}
slug = "{{ .Slug }}"{{end}}
date = {{ .Published }}
updated = {{ .Updated }}{{ with .Tags.TomlString }}
tags = [{{ . }}]{{ end }}{{ if .Draft }}
draft = true{{ end }}{{ if not (len .Comments | eq 0) }}
comments = [ {{range $i, $e := .Comments}}{{if $i}}, {{end}}{{$e}}{{end}} ]{{ end }}
blogimport = true {{ with .Extra }}
{{.}}{{ end }}
[author]
	name = "{{ .Author.Name }}"
	uri = "{{ .Author.Uri }}"
[author.image]
	source = "{{ .Author.Image.Source }}"
	width = "{{ .Author.Image.Width }}"
	height = "{{ .Author.Image.Height }}"

+++
{{ .Content }}
`

var yamlTempl = `---
title: "{{ .Title }}"
date: {{ .Published }}
updated: {{ .Updated }}{{ with .Tags.TomlString }}
tags: [{{ . }}]{{ end }}{{ if .Draft }}
draft: true{{ end }}
blogimport: true {{ with .Extra }}
{{.}}{{ end }}
author: "{{ .Author.Name }}"
---

{{ .Content }}
`

var t = template.Must(template.New("").Parse(yamlTempl))
var exp = Export{}

func (s EntrySet) Len() int {
	return len(s)
}
func (s EntrySet) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s EntrySet) Less(i, j int) bool {
	return time.Time(exp.Entries[s[i]].Published).Before(time.Time(exp.Entries[s[j]].Published))
}

func treeSort(i int) (list []int) {
	sort.Sort(EntrySet(exp.Entries[i].Children))
	for _, v := range exp.Entries[i].Children {
		list = append(list, v)
		list = append(list, treeSort(v)...)
	}
	return
}

func main() {
	log.SetFlags(0)

	var extra = flag.String("extra", "", "additional metadata to set in frontmatter")
	flag.Parse()

	args := flag.Args()

	if len(args) != 2 {
		log.Printf("Usage: %s [options] <xmlfile> <targetdir>", os.Args[0])
		log.Println("options:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	dir := args[1]

	info, err := os.Stat(dir)

	if os.IsNotExist(err) {
		err = os.MkdirAll(path.Join(dir, "comments"), 0755)
	}
	if err != nil {
		log.Fatal(err)
	}

	info, err = os.Stat(dir)
	if err != nil || !info.IsDir() {
		log.Fatal("Second argument is not a directory.")
	}

	b, err := ioutil.ReadFile(args[0])
	if err != nil {
		log.Fatal(err)
	}

	err = xml.Unmarshal(b, &exp)
	if err != nil {
		log.Fatal(err)
	}

	if len(exp.Entries) < 1 {
		log.Fatal("No blog entries found!")
	}

	postmap := make(map[uint64]int)

	// Go through and create a map of all entries so we can refer to them later by ID number
	for k := range exp.Entries {
		isTemplate := false
		for _, tag := range exp.Entries[k].Tags {
			if tag.Scheme == "http://schemas.google.com/g/2005#kind" {
				switch tag.Name {
				case "http://schemas.google.com/blogger/2008/kind#comment":
					fallthrough
				case "http://schemas.google.com/blogger/2008/kind#post":
				default:
					isTemplate = true
				}
				break
			}
		}
		if isTemplate {
			continue
		}
		if index := strings.LastIndex(exp.Entries[k].ID, "post-"); index >= 0 {
			exp.Entries[k].ID = exp.Entries[k].ID[index+5:]

			if id, err := strconv.ParseUint(exp.Entries[k].ID, 10, 64); err == nil {
				postmap[id] = k
			} else {
				fmt.Println("Can't parse " + exp.Entries[k].ID)
			}
		}
		for _, link := range exp.Entries[k].Links {
			switch strings.ToLower(link.Rel) {
			case "related":
				exp.Entries[k].Reply, _ = strconv.ParseUint(path.Base(link.Link), 10, 64)
			case "alternate":
			case "replies":
				exp.Entries[k].Slug = strings.Replace(path.Base(link.Link), path.Ext(link.Link), "", -1)
			}
		}
	}

	// Build comment heirarchy
	for k, entry := range exp.Entries {
		for _, tag := range entry.Tags {
			if tag.Name == "http://schemas.google.com/blogger/2008/kind#comment" &&
				tag.Scheme == "http://schemas.google.com/g/2005#kind" {
				parent := entry.Reply
				if parent == 0 {
					parent, _ = strconv.ParseUint(path.Base(entry.Source.Source), 10, 64)
				}
				if parent == 0 {
					fmt.Println("Skipping deleted comment " + entry.ID)
					break
				}
				if i, ok := postmap[parent]; ok {
					exp.Entries[i].Children = append(exp.Entries[i].Children, k)
				} else {
					panic(strconv.Itoa(k) + " entry did not exist")
				}
				writeComment(entry, dir)
				break
			}
		}
	}

	count := 0
	drafts := 0
	for k, entry := range exp.Entries {
		isPost := false
		for _, tag := range entry.Tags {
			if tag.Name == "http://schemas.google.com/blogger/2008/kind#post" &&
				tag.Scheme == "http://schemas.google.com/g/2005#kind" {
				isPost = true
				break
			}
		}
		if !isPost {
			continue
		}
		// Sort and flatten all top level comment chains
		entry.Children = treeSort(k)
		for _, v := range entry.Children {
			if id, err := strconv.ParseUint(exp.Entries[v].ID, 10, 64); err == nil {
				entry.Comments = append(entry.Comments, id)
			}
		}
		if extra != nil {
			entry.Extra = *extra
		}
		if err := writeEntry(entry, dir); err != nil {
			log.Fatalf("Failed writing post %q to disk:\n%s", entry.Title, err)
		}
		if entry.Draft {
			drafts++
		} else {
			count++
		}
	}
	log.Printf("Wrote %d published posts to disk.", count)
	log.Printf("Wrote %d drafts to disk.", drafts)
}

var delim = []byte("+++\n")

func writeEntry(e Entry, dir string) error {
	slug := makePath(e.Published, e.Title)
	filename := filepath.Join(dir, slug+".md")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, e)
}

func writeComment(e Entry, dir string) error {
	e.Title = strings.Replace(strings.Replace(e.Title, "\n", "", -1), "\r", "", -1)
	filename := filepath.Join(path.Join(dir, "comments"), "c"+e.ID+".toml")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return t.Execute(f, e)
}

// Take a string with any characters and replace it so the string could be used in a path.
// E.g. Social Media -> social-media
func makePath(d Date, s string) string {
	return fmt.Sprintf("%v-%s", d.String()[:10], unicodeSanitize(strings.ToLower(strings.Replace(strings.TrimSpace(s), " ", "-", -1))))
}

func unicodeSanitize(s string) string {
	source := []rune(s)
	target := make([]rune, 0, len(source))

	for _, r := range source {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			target = append(target, r)
		}
	}
	return string(target)
}
