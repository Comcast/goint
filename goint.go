// Copyright 2016 Comcast Cable Communications Management, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// End Copyright

package goint

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

const buflen = 4096
const defaultBufferLimit = 4096

// These types are used to implement the integration test spec
type (
	// Worker interface is an interface that all nodes in the integration test
	// spec must implement.
	Worker interface {
		Report(int) error   // Prints Name and iterates over subnodes
		Go(*sync.WaitGroup) // Performs function of node
	}
	// Comp is a composite node. It groups multiple nodes together
	Comp struct {
		// Name of this step
		Name       string // Name of the step. Documentation only
		Sequential bool   // The nodes in SubN should be done serially
		SubN       []Worker
		Err        error
	}
	// Curl is used to do an http request a la curl
	Curl struct {
		Name          string // Name of the step. Documentation only
		URL           string
		PrintBodyFlag bool
		Resp          *http.Response
		Err           error
	}

	Delay struct {
		Name     string // Name of the step. Documentation only
		Duration time.Duration
		Err      error
	}
	DelayHealthCheck struct {
		Name         string // Name of the step. Documentation only
		URL          string
		Resp         *http.Response
		PollInterval time.Duration
		NPolls       int
		Err          error
	}
	Wait struct {
		Name  string // Name of the step. Documentation only
		Token string
		Err   error
	}
	Kill struct {
		Name  string // Name of the step. Documentation only
		Token string
		Err   error
	}
	// GoFunc is used to embed
	GoFunc struct {
		Name string    // Name of the step. Documentation only
		Func GoRoutine // Function to invoke
		Skip bool      // Flag to skip a test case, this will be reported SKIPPED in test report
		Err  error
	}
	// Type of functions in GoFunc
	GoRoutine func(*GoFunc)
	// Exec is used to start a process (ForkExec)
	// token is used to refer to that process (e.g. in Kill)

	stream struct {
		stream      io.ReadCloser
		firstBuf    []byte // Print the first buffer, even in TailMode
		data        chan []byte
		BufferLimit int
		TailMode    bool
		Report      int // 0 = normal, 1 = on error, 2 = off
		Discards    int
	}
	Exec struct {
		Name             string // Name of the step. Documentation only
		Token            string
		Directory        string
		Precommand       string
		Command          func() *exec.Cmd
		Args             []string
		Sequential       bool // DEPRECATED, remove all references
		SubN             []Worker
		Cmd              *exec.Cmd
		Pid              int // Pid of process
		Err              error
		stdwg            sync.WaitGroup
		KilledExplicitly bool // Whether killed explicitly by Kill worker

		Stdout stream
		Stderr stream
	}
)

var (
	TailReportOnError = stream{
		Report:   1,
		TailMode: true,
	}
	TailReportOn = stream{
		TailMode: true,
	}
	ReportOn = stream{}
)

var (
	Execs map[string]*Exec
)

func init() {
	Execs = make(map[string]*Exec)
}

// ---------------- Delay Methods
func (w *Delay) Go(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("%s\n", w.Name)
	time.Sleep(w.Duration)
}

func printName(dep int, name string) {
	name += "                                                  "
	fmt.Printf("%s%s: ", indent(dep), name[:50-dep*8])
}

func ReportNode(dep int, name string, err error) {
	printName(dep, name)
	if err == nil {
		fmt.Printf("OK\n")
	} else {
		fmt.Printf("FAILED: %s\n", err)
	}
}

func ReportNodeSkipped(dep int, name string) {
	printName(dep, name)
	fmt.Printf("SKIPPED\n")
}

func (w *Delay) Report(dep int) error {
	ReportNode(dep, w.Name, w.Err)
	return w.Err
}

// ---------------- DelayHealthCheck Methods
func (w *DelayHealthCheck) Go(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("%s\n", w.Name)

	// Wait 1 PollInterval to avoid clutter in the logs
	time.Sleep(w.PollInterval)
	for i := 0; i < w.NPolls; i++ {
		w.Resp, w.Err = http.Get(w.URL)
		if w.Err != nil {
			fmt.Printf("HealthCheckDelay-ing\n")
			fmt.Printf("bad read, err = %s\n", w.Err)
			time.Sleep(w.PollInterval)
			continue
		}
		defer w.Resp.Body.Close()
		if w.Resp.StatusCode == 200 {
			buf := make([]byte, 4096)

			defer w.Resp.Body.Close()
			_, err := w.Resp.Body.Read(buf)
			if err != nil && err != io.EOF {
				fmt.Printf("DelayHealthCheck: error reading body %s\n", err)
			} else {
				l := w.Resp.ContentLength
				if l > 0 {
					fmt.Printf("%s\n", string(buf[:l]))
				} else {
					fmt.Printf("%s\n", "No content in http response.")
				}
			}
			break
		}
		time.Sleep(w.PollInterval)
	}
	fmt.Printf("HealthCheckDelay Finished(%s)\n", w.Name)
}

func (w *DelayHealthCheck) Report(dep int) error {
	ReportNode(dep, w.Name, w.Err)
	return w.Err
}

// ---------------- GoFunc Methods
func (w *GoFunc) Go(wg *sync.WaitGroup) {
	fmt.Printf("GoFunc  %s\n", w.Name)
	defer wg.Done()

	// The Skip flag can be used to skip a test case from running.
	// For these cases the status will be set to SKIPPED in the report.
	if w.Skip == true {
		fmt.Printf("GoFunc %s skipping", w.Name)
		return
	}

	if w.Func == nil {
		w.Err = errors.New("GoFunc without a func attached")
		return
	}
	w.Func(w)
}

func (w *GoFunc) Report(dep int) error {
	if w.Skip == true {
		ReportNodeSkipped(dep, w.Name)
	} else {
		ReportNode(dep, w.Name, w.Err)
	}

	return w.Err
}

// ---------------- Curl Methods
func (w *Curl) Go(wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("%s\n", w.Name)

	w.Resp, w.Err = http.Get(w.URL)
	if w.Err != nil {
		fmt.Printf("Curl.Go: bad http.Get, err = %s\n", w.Err)
		return
	}
	defer w.Resp.Body.Close()
	if w.PrintBodyFlag {
		// If it's -1, we have an unknown content length. Possibly have
		// chunked Transfer Encoding.
		if w.Resp.ContentLength == -1 {
			//Verify that it's chunked.
			isChunked := false
			for _, xfer := range w.Resp.TransferEncoding {
				if xfer == "chunked" {
					isChunked = true
					break
				}
			}
			if isChunked {
				if resp, err := ioutil.ReadAll(w.Resp.Body); err != nil {
					fmt.Printf("Curl:PrintBodyFlag: error reported %s\n", err)
				} else {
					fmt.Printf("%s\n", string(resp))
				}

			} else {
				fmt.Printf("Unknown Transfer encoding present.\n")
			}
		} else {
			buf := make([]byte, 4096)
			_, err := w.Resp.Body.Read(buf)
			if err != nil && err != io.EOF {
				fmt.Printf("Curl:PrintBodyFlag: error reported %s\n", err)
			} else {
				l := w.Resp.ContentLength
				if l < 0 {
					fmt.Printf("w.Resp.ContentLength negative response\n")
				} else {
					fmt.Printf("%s\n", string(buf[:l]))
				}
			}
		}
	}
}

func (w *Curl) Report(dep int) error {
	ReportNode(dep, w.Name, w.Err)
	return w.Err
}

// ---------------- Exec Methods
func (w *Exec) Go(wg *sync.WaitGroup) {
	defer wg.Done()
	Execs[w.Token] = w

	fmt.Printf("Exec.Go(%s)\n", w.Token)

	// Set some default values (16M each buffer)
	if w.Stdout.BufferLimit == 0 {
		w.Stdout.BufferLimit = (defaultBufferLimit)
	}
	if w.Stderr.BufferLimit == 0 {
		w.Stderr.BufferLimit = (defaultBufferLimit)
	}
	w.Err = os.Chdir(w.Directory)
	if w.Err != nil {
		fmt.Printf("Exec.Go(%s): Chdir failed %s\n", w.Token, w.Err)
		return
	}
	w.Cmd = w.Command()

	w.Stdout.stream, w.Err = w.Cmd.StdoutPipe()
	if w.Err != nil {
		fmt.Printf("Exec.Go(%s): w.Cmd.StdoutPipe() failed%s\n", w.Token, w.Err)
		return
	}
	w.Stderr.stream, w.Err = w.Cmd.StderrPipe()
	if w.Err != nil {
		fmt.Printf("Exec.Go(%s): w.Cmd.StderrPipe() failed%s\n", w.Token, w.Err)
		return
	}
	w.Err = w.Cmd.Start()
	if w.Err != nil {
		fmt.Printf("Exec.Go(%s): w.Cmd.Start() failed %s\n", w.Token, w.Err)
		return
	}

	// Start 2 go routines to collect stdout and stderr
	collector := func(tag string, w *Exec, s *stream) {
		defer w.stdwg.Done()
		s.data = make(chan []byte, s.BufferLimit)
		buffer := make([]byte, buflen)
		for {
			// The read will block until there's data to read OR the pipe is
			// closed by calling .Wait()
			errlen, err := io.ReadAtLeast(s.stream, buffer, buflen)
			if errlen > 0 {
				if s.firstBuf == nil {
					s.firstBuf = buffer[0:errlen]
					buffer = make([]byte, buflen)
					continue
				}
				if s.TailMode {
					if len(s.data) == s.BufferLimit {
						<-s.data // discard
						s.Discards++
					}
					s.data <- buffer[0:errlen] // append
				} else {
					if len(s.data) == s.BufferLimit {
						s.Discards++
						continue
					}
					s.data <- buffer[0:errlen] // append
				}
			}
			if err != nil {
				break
			}
			buffer = make([]byte, buflen)
		}
	}
	w.stdwg.Add(2)
	go collector("stdout", w, &w.Stdout)
	go collector("stderr", w, &w.Stderr)

	if w.Err != nil {
		fmt.Printf("Exec.Go(%s):Start failed %s\n", w.Token, w.Err)
		return
	}
}

func (w *Exec) Report(dep int) error {
	ReportNode(dep, w.Name, w.Err)
	return w.Err
}

// ---------------- Comp Methods

func (w *Comp) Go(wg *sync.WaitGroup) {
	fmt.Printf("Go  %s\n", w.Name)
	defer wg.Done()

	if len(w.SubN) > 0 {
		var wg2 sync.WaitGroup
		for _, sub := range w.SubN {
			wg2.Add(1)
			if w.Sequential {
				sub.Go(&wg2)
			} else {
				// TODO put back the 'go'
				sub.Go(&wg2)
			}
		}
		wg2.Wait()
	}
	fmt.Printf("End %s\n", w.Name)
}

func (w *Comp) Report(dep int) error {
	var err error
	ReportNode(dep, w.Name, w.Err)
	for _, sub := range w.SubN {
		e1 := sub.Report(dep + 1)
		if e1 != nil {
			err = e1
		}
	}
	if w.Err != nil {
		return w.Err
	} else {
		return err
	}
}

func indent(d int) (tabs string) {
	for i := 0; i < d; i++ {
		tabs += "\t"
	}
	return
}

func (w *Comp) Start() {
	var wg sync.WaitGroup

	wg.Add(1)
	w.Go(&wg)
	if w.Err != nil {
		return
	}
	wg.Wait()
}

// ----------- Wait Methods

func (w *Wait) Go(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("Go %s, token = %s\n", w.Name, w.Token)

	runner := Execs[w.Token]
	if runner == nil {
		fmt.Printf("Wait.Go: Token not found\n")
		return
	}
	runner.stdwg.Wait()

	err := runner.Cmd.Wait()
	if err != nil {
		if runner.KilledExplicitly {
			fmt.Printf("Wait.Go: Cmd.Wait() returns error %s. Ignore error; killed explicitly", err)
		} else {
			w.Err = err
		}
	}
	strfmt := "\n++++++++ start of %s ++++++++++++++++++++++++++++++++++++++\n"
	onefmt := "\n+++++++++++ Here is the first buffer (%s)++++++++++++++++++\n"
	tailfmt := "\n+++++++++++ TailMode Discarded %d %s buffers +++++++++++++\n"
	headfmt := "\n+++++++++++ HeadMode Discarded %d %s buffers +++++++++++++\n"
	endfmt := "\n++++++++ end of %s ++++++++++++++++++++++++++++++++++++++++\n"

	reporter := func(tag string, s *stream) {
		if s.Report == 0 ||
			((s.Report == 1) && (err != nil)) {

			fmt.Printf(strfmt, tag)
			if s.firstBuf != nil {
				fmt.Printf(onefmt, tag)
				fmt.Print(string(s.firstBuf))
			}
			if s.TailMode && s.Discards > 0 {
				fmt.Printf(tailfmt, s.Discards, tag)
			}
			for len(s.data) > 0 {
				buf := <-s.data
				fmt.Print(string(buf))
			}
			if !s.TailMode && s.Discards > 0 {
				fmt.Printf(headfmt, s.Discards, tag)
			}
			fmt.Printf(endfmt, tag)
		}
	}
	reporter("stdout", &runner.Stdout)
	reporter("stderr", &runner.Stderr)
}

func (w *Wait) Report(dep int) error {
	ReportNode(dep, w.Name, w.Err)
	return w.Err
}

// ----------- Kill Methods
func (w *Kill) Go(wg *sync.WaitGroup) {
	defer wg.Done()
	fmt.Printf("Go %s, token = %s\n", w.Name, w.Token)
	runner := Execs[w.Token]
	if runner == nil {
		fmt.Printf("Kill.Go: Token not found\n")
		return
	}
	w.Err = runner.Cmd.Process.Kill()
	if w.Err != nil {
		fmt.Printf("Kill.Go: syscall.Kill returned %s\n", w.Err)
	} else {
		runner.KilledExplicitly = true
	}
	return // implied
}
func (w *Kill) Report(dep int) error {
	ReportNode(dep, w.Name, w.Err)
	return w.Err
}

// --------------
