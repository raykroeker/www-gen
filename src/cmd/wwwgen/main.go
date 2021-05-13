package main

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
)

var (
	debugL           = log.New(ioutil.Discard, "DEBUG ", log.LstdFlags)
	errL             = log.New(os.Stderr, "ERROR ", log.LstdFlags)
	wd, _            = os.Getwd()
	defaultContent   = path.Clean(path.Join(wd, "..", "www-src", "content"))
	defaultMonitor   = path.Clean(path.Join(wd, "..", "www-src", "mon.json"))
	defaultSites     = path.Clean(path.Join(wd, "..", "wwww"))
	defaultTemplates = path.Clean(path.Join(wd, "..", "www-src", "templates"))
)

var flags struct {
	debug     bool
	config    string
	content   string
	monitor   string
	sites     string
	templates string
}

func main() {
	flag.BoolVar(&flags.debug, "debug", false, "enable debug logging")
	flag.StringVar(&flags.config, "config", "bin/sites.json", "site generator configuration")
	flag.StringVar(&flags.content, "content", defaultContent, "static content directory")
	flag.StringVar(&flags.monitor, "monitor", defaultMonitor, "monitor config file")
	flag.StringVar(&flags.sites, "sites", defaultSites, "sites root")
	flag.StringVar(&flags.templates, "templates", defaultTemplates, "templates directory")
	flag.Parse()
	if flags.debug {
		debugL.SetOutput(os.Stdout)
	}
	data, err := ioutil.ReadFile(flags.config)
	if err != nil {
		errL.Fatalf("Cannot read config filename=%s err=%v", flags.config, err)
	}
	var config *config
	err = json.Unmarshal(data, &config)
	if err != nil {
		errL.Fatalf("Cannot parse config filename=%s err=%v", flags.config, err)
	}
	debugL.Printf("config=%v", config)
	urls := map[string]struct{}{}
	mon := &monitor{}
	for _, site := range config.Sites {
		for _, domain := range site.Domains {
			for _, content := range site.Content {
				for _, path := range content.Paths {
					url := fmt.Sprintf("https://%s/%s", domain, path)
					if _, ok := urls[url]; ok {
						errL.Fatalf("Duplicate url=%s", url)
					}
					urls[url] = struct{}{}
				}
			}
			for _, page := range site.Pages {
				for _, path := range page.Paths {
					url := fmt.Sprintf("https://%s/%s", domain, path)
					if _, ok := urls[url]; ok {
						errL.Fatalf("Duplicate url=%s", url)
					}
					urls[url] = struct{}{}
				}
			}
			for _, content := range site.Content {
				debugL.Printf("content=%v", content)
				err = content.copy(mon, domain)
				if err != nil {
					errL.Fatalf("Cannot copy content=%v err=%v", content, err)
				}
			}
			for _, page := range site.Pages {
				err = page.generate(mon, domain)
				if err != nil {
					errL.Fatalf("Cannot generate page=%v err=%v", page, err)
				}
			}
		}
	}
	data, err = json.MarshalIndent(&mon, "", "    ")
	if err != nil {
		errL.Fatalf("Cannot marshal monitor config err=%v", err)
	}
	err = ioutil.WriteFile(flags.monitor, data, 0644)
	if err != nil {
		errL.Fatalf("Cannot write monitor file=%s err=%v", flags.monitor, err)
	}
}

type monitor struct {
	Endpoints []*endpoint `json:"endpoints"`
}

func (m *monitor) addEndpoint(url string, sha512 []byte) {
	m.Endpoints = append(m.Endpoints, &endpoint{
		URL:    url,
		SHA512: base64.StdEncoding.EncodeToString(sha512),
	})
}

type endpoint struct {
	URL    string `json:"url"`
	SHA512 string `json:"sha512"`
}

type config struct {
	Sites map[string]*site `json:"sites"`
}

func (c *config) String() string {
	return fmt.Sprintf("sites=%v", c.Sites)
}

type site struct {
	Content []*content `json:"content"`
	Domains []string   `json:"domains"`
	Pages   []*page    `json:"pages"`
}

func (s site) String() string {
	return fmt.Sprintf("content=%v domains=%q pages=%v", s.Content, s.Domains, s.Pages)
}

type content struct {
	Paths   []string `json:"paths"`
	Address string   `json:"address"`
}

func (c content) copy(mon *monitor, domain string) error {
	copy := func(dstFilename, srcFilename string) ([]byte, int64, error) {
		dst, err := os.OpenFile(dstFilename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if err != nil {
			return nil, 0, err
		}
		defer dst.Close()
		hash := sha512.New()
		w := io.MultiWriter(hash, dst)
		src, err := os.Open(srcFilename)
		if err != nil {
			return nil, 0, err
		}
		written, err := io.Copy(w, src)
		if err != nil {
			return nil, written, err
		}
		return hash.Sum(nil), written, err
	}
	tokens := strings.Split(c.Address, ":")
	srcFilename := path.Join(flags.content, tokens[0], tokens[1])
	if _, err := os.Stat(srcFilename); err != nil {
		return err
	}
	for _, p := range c.Paths {
		dstFilename := path.Join(flags.sites, domain, p)
		dir := path.Dir(dstFilename)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0700)
			if err != nil {
				return err
			}
		}
		hashSum, bytes, err := copy(dstFilename, srcFilename)
		if err != nil {
			return err
		}
		mon.addEndpoint(fmt.Sprintf("https://%s%s", domain, p), hashSum)
		debugL.Printf("src=%s dst=%s bytes=%d", srcFilename, dstFilename, bytes)
	}
	return nil
}

func (c content) String() string {
	return fmt.Sprintf("paths=%q address=%s", c.Paths, c.Address)
}

type page struct {
	Paths    []string `json:"paths"`
	Template string   `json:"template"`
}

func (p page) String() string {
	return fmt.Sprintf("paths=%q template=%s", p.Paths, p.Template)
}

func (p *page) generate(mon *monitor, domain string) error {
	text, err := ioutil.ReadFile(path.Join(flags.templates, fmt.Sprintf("%s.html", p.Template)))
	if err != nil {
		return err
	}
	template, err := template.New(p.Template).Parse(string(text))
	if err != nil {
		return err
	}
	execute := func(filename string) ([]byte, error) {
		dir := path.Dir(filename)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			err = os.MkdirAll(dir, 0755)
			if err != nil {
				return nil, err
			}
		}
		file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		hash := sha512.New()
		var w io.Writer
		if flags.debug {
			w = io.MultiWriter(hash, file, os.Stdout)
		} else {
			w = io.MultiWriter(hash, file)
		}
		err = template.Execute(w, nil)
		if err != nil {
			return nil, err
		}
		return hash.Sum(nil), nil
	}
	for _, p := range p.Paths {
		hashSum, err := execute(path.Join(flags.sites, domain, p))
		if err != nil {
			return err
		}
		mon.addEndpoint(fmt.Sprintf("https://%s%s", domain, p), hashSum)
	}
	return nil
}
