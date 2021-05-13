package main

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"sync"
	"time"
)

var (
	debugL        = log.New(ioutil.Discard, "DEBUG ", log.LstdFlags)
	errL          = log.New(os.Stderr, "ERROR ", log.LstdFlags)
	wd, _         = os.Getwd()
	defaultConfig = path.Join(wd, "monitor.json")
)

var flags struct {
	debug    bool
	config   string
	hash     bool
	parallel int
}

func main() {
	flag.BoolVar(&flags.debug, "debug", false, "enable debug logging")
	flag.StringVar(&flags.config, "config", defaultConfig, "site configuration")
	flag.BoolVar(&flags.hash, "hash", false, "check the hash of the url")
	flag.IntVar(&flags.parallel, "parallel", 16, "parallel checks")
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
	checkCh := make(chan check, flags.parallel*2)
	var wg sync.WaitGroup
	for i := 0; i < flags.parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for check := range checkCh {
				err := check.execute()
				if err != nil {
					errL.Printf("Cannot execute check=%v err=%v", check, err)
				}
			}
		}()
	}
	client := &http.Client{
		Timeout: time.Millisecond * 750,
	}
	for _, endpoint := range config.Endpoints {
		checkCh <- &endpointCheck{
			client:   client,
			endpoint: endpoint,
		}
	}
	close(checkCh)
	wg.Wait()
}

type check interface {
	execute() error
}

type endpointCheck struct {
	client   *http.Client
	endpoint *endpoint
}

func (ec *endpointCheck) execute() error {
	res, err := ec.client.Get(ec.endpoint.URL)
	if err != nil {
		return fmt.Errorf("Cannot execute GET url=%s err=%v", ec.endpoint.URL, err)
	} else {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("Cannot read result body url=%s err=%v", ec.endpoint.URL, err)
		}
		if res.StatusCode == http.StatusOK {
			if flags.hash {
				hash := sha512.New()
				sha512 := hash.Sum(body)
				sha512Str := base64.StdEncoding.EncodeToString(sha512)
				if sha512Str == ec.endpoint.SHA512 {
					return nil
				}
				return fmt.Errorf("HTTP result body incorrect endpoint=%v sha512=%s", ec.endpoint, sha512Str)
			}
			return nil
		} else {
			return fmt.Errorf("HTTP status not OK url=%s status=%s", ec.endpoint.URL, res.Status)
		}
	}
	return nil
}

type config struct {
	Endpoints []*endpoint `json:"endpoints"`
}

func (c *config) String() string {
	return fmt.Sprintf("endpoints=%v", c.Endpoints)
}

type endpoint struct {
	URL    string `json:"url"`
	SHA512 string `json:"sha512"`
}

func (e endpoint) String() string {
	return fmt.Sprintf("url=%s sha512=%s", e.URL, e.SHA512)
}
