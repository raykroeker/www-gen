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
	"sort"
	"strings"
	"sync"
	"time"
	"www"
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
	parallel int
}

func main() {
	flag.BoolVar(&flags.debug, "debug", false, "enable debug logging")
	flag.StringVar(&flags.config, "config", defaultConfig, "monitor configuration")
	flag.IntVar(&flags.parallel, "parallel", 16, "parallel checks")
	flag.Parse()
	if flags.debug {
		debugL.SetOutput(os.Stdout)
	}
	data, err := ioutil.ReadFile(flags.config)
	if err != nil {
		errL.Fatalf("Cannot read config filename=%s err=%v", flags.config, err)
	}
	var mon *www.Monitor
	err = json.Unmarshal(data, &mon)
	if err != nil {
		errL.Fatalf("Cannot parse config filename=%s err=%v", flags.config, err)
	}
	debugL.Printf("mon=%v", mon)
	checkCh := make(chan check, flags.parallel*2)
	checkResultCh := make(chan result, flags.parallel*2)
	done := make(chan int)
	go func() {
		all := []result{}
		for res := range checkResultCh {
			all = append(all, res)
		}
		sort.SliceStable(all, func(i, j int) bool {
			for k := 0; k < len(all[i].context); k++ {
				if all[i].context[k] < all[j].context[k] {
					return true
				}
			}
			return false
		})
		exit := 0
		for _, res := range all {
			if res.pass {
				fmt.Printf("%v\n", res)
			} else {
				exit = 1
				fmt.Printf("\n%v\n\n", res)
			}
		}
		done <- exit
		close(done)
	}()
	var wg sync.WaitGroup
	for i := 0; i < flags.parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for check := range checkCh {
				res, err := check.execute()
				if err != nil {
					errL.Fatalf("Cannot execute check=%v err=%v", check, err)
				}
				checkResultCh <- *res

			}
		}()
	}
	client := &http.Client{
		Timeout: time.Millisecond * 750,
	}
	for _, endpoint := range mon.Endpoints {
		checkCh <- &endpointCheck{
			client:   client,
			endpoint: endpoint,
		}
	}
	close(checkCh)
	wg.Wait()
	close(checkResultCh)
	os.Exit(<-done)
}

// check defines a check execution.
type check interface {

	// execute a check and return a result. An error is only returned in the
	// fatal case, not the failing case.
	execute() (*result, error)
}

// endpointCheck uses the client to issue a request to the endpoint and compare
// the body hash.
type endpointCheck struct {
	client   *http.Client
	endpoint *www.Endpoint
}

// context returns a result context including the method and url.
func (ec endpointCheck) context() []string {
	return []string{ec.endpoint.Method, ec.endpoint.URL}
}

// fail reutrns a failed result using the error string.
func (ec endpointCheck) fail(err error) *result {
	return &result{
		pass:    false,
		context: ec.context(),
		message: err.Error(),
	}
}

// failf returns a formatted failed result.
func (ec endpointCheck) failf(format string, arg ...interface{}) *result {
	return &result{
		pass:    false,
		context: ec.context(),
		message: fmt.Sprintf(format, arg...),
	}
}

// execute implements check.
func (ec *endpointCheck) execute() (*result, error) {
	req, err := http.NewRequest(ec.endpoint.Method, ec.endpoint.URL, nil)
	if err != nil {
		return nil, err
	}
	res, err := ec.client.Do(req)
	if err != nil {
		return ec.fail(err), nil
	} else {
		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("Cannot read result body url=%s err=%v", ec.endpoint.URL, err)
		}
		if res.StatusCode == ec.endpoint.ExpectedStatusCode {
			hash := sha512.New()
			_, err = hash.Write(body)
			if err != nil {
				return nil, err
			}
			bodyHash := base64.StdEncoding.EncodeToString(hash.Sum(nil))
			if bodyHash == ec.endpoint.ExpectedBodyHash {
				return &result{
					pass:    true,
					context: []string{ec.endpoint.Method, ec.endpoint.URL},
					message: "",
				}, nil
			}
			return ec.failf("res-hash: %s<>%s", bodyHash, ec.endpoint.ExpectedBodyHash), nil
		} else {
			return ec.failf("http-status: %d<>%d", res.StatusCode, ec.endpoint.ExpectedStatusCode), nil
		}
	}
}

// result of a check execution.
type result struct {
	pass    bool
	context []string
	message string
}

// String implements Stringer.
func (r result) String() string {
	str := "FAIL"
	if r.pass {
		str = "PASS"
	}
	buf := strings.Builder{}
	_, _ = fmt.Fprintf(&buf, "%s", str)
	if len(r.context) > 0 {
		_, _ = fmt.Fprintf(&buf, ": %s", strings.Join(r.context, " | "))
	}
	if len(r.message) > 0 {
		_, _ = fmt.Fprintf(&buf, ": %s", r.message)
	}
	return buf.String()
}
