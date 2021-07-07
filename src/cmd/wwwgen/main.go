// Command wwwgen uses the site configuration to generate a static web site. The
// tempates are read from the
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
	"net/http"
	"os"
	"path"
	"strings"
	"www"

	"github.com/russross/blackfriday"
)

var (
	debugL           = log.New(ioutil.Discard, "DEBUG ", log.LstdFlags)
	errL             = log.New(os.Stderr, "ERROR ", log.LstdFlags)
	wd, _            = os.Getwd()
	defaultContent   = path.Clean(path.Join(wd, "..", "www-src", "content"))
	defaultMonitor   = path.Clean(path.Join(wd, "..", "www-src", "mon.json"))
	defaultSites     = path.Clean(path.Join(wd, "..", "www"))
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
	mon := &www.Monitor{}
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

type config struct {
	Sites map[string]*site `json:"sites"`
}

func (c *config) String() string {
	return fmt.Sprintf("sites=%v", c.Sites)
}

type site struct {
	Content []*content `json:"content"`
	Domains []string   `json:"domains"`
	Pages   []*pages   `json:"pages"`
}

func (s site) String() string {
	return fmt.Sprintf("content=%v domains=%q pages=%v", s.Content, s.Domains, s.Pages)
}

type content struct {
	Paths   []string `json:"paths"`
	Address string   `json:"address"`
}

func (c content) copy(mon *www.Monitor, domain string) error {
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
		dstFilename := localFilePath(domain, p)
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
		mon.Endpoints = append(mon.Endpoints, &www.Endpoint{
			Method:             http.MethodGet,
			URL:                fmt.Sprintf("https://%s%s", domain, p),
			ExpectedStatusCode: 200,
			ExpectedBodyHash:   base64.StdEncoding.EncodeToString(hashSum),
		})
		debugL.Printf("src=%s dst=%s bytes=%d", srcFilename, dstFilename, bytes)
	}
	return nil
}

func (c content) String() string {
	return fmt.Sprintf("paths=%q address=%s", c.Paths, c.Address)
}

// pages uses a single go html template to generate multiple static pages. A
// page is generated per path.
type pages struct {
	// Data for the template.
	Data map[string]interface{} `json:"data"`
	// Paths at which the pages exist.
	Paths []string `json:"paths"`
	// Template used to render the pages.
	Template string `json:"template"`
}

// String imlements Stringer.
func (p pages) String() string {
	return fmt.Sprintf("paths=%q template=%s", p.Paths, p.Template)
}

// funcMap returns a template function map.
func (p *pages) funcMap() template.FuncMap {
	return template.FuncMap{
		"markdownToHTML": func(templateFilename string) template.HTML {
			filename := path.Join(flags.templates, fmt.Sprintf("%s.md", templateFilename))
			data, err := ioutil.ReadFile(filename)
			if err != nil {
				errL.Fatalf("Cannot read markdown filename=%s", filename)
			}
			return template.HTML(string(blackfriday.Run(data)))
		},
	}
}

// generate pages for the domain. The template is executed for each path (sub
// optimal) and written into the domain root.
func (p *pages) generate(mon *www.Monitor, domain string) error {
	text, err := ioutil.ReadFile(path.Join(flags.templates, fmt.Sprintf("%s.html", p.Template)))
	if err != nil {
		return err
	}
	template, err := template.New(p.Template).Funcs(p.funcMap()).Parse(string(text))
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
		err = template.Execute(w, p.Data)
		if err != nil {
			return nil, err
		}
		return hash.Sum(nil), nil
	}
	for _, p := range p.Paths {
		hashSum, err := execute(localFilePath(domain, p))
		if err != nil {
			return err
		}
		mon.Endpoints = append(mon.Endpoints, &www.Endpoint{
			Method:             http.MethodGet,
			URL:                fmt.Sprintf("https://%s%s", domain, p),
			ExpectedStatusCode: http.StatusOK,
			ExpectedBodyHash:   base64.StdEncoding.EncodeToString(hashSum),
		})
	}
	return nil
}

// localFilePath builds a local filesystem path at to write to.
func localFilePath(domain, leaf string) string {
	// www.raykroeker.com -> com.raykroeker.www
	tokens := strings.Split(domain, ".")
	for left, right := 0, len(tokens)-1; left < right; left, right = left+1, right-1 {
		tokens[left], tokens[right] = tokens[right], tokens[left]
	}
	return path.Join(flags.sites, strings.Join(tokens, "."), leaf)
}
