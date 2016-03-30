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

// Package integrationtest is used to run the leaderelection library
// through a series tests with a real zookeeper instance that is
// killed, stop, restarted, etc.

package goint

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"testing"
	"time"
)

// This is the list of all the test cases we would like to run
var zkTestCases []Worker

var (
	Info *log.Logger
)

func createLoggers(errorHandle io.Writer) {
	Info = log.New(errorHandle,
		"Info: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

// TestMain kicks off all the tests
func TestGoInt(t *testing.T) {
	createLoggers(os.Stderr)
	rootDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Error getting wd: %v", err)
	}

	var gointTest = &Comp{
		Name: "Top",
		SubN: []Worker{
			&Comp{
				Name: "Exec/Wait test", // or some other meaningful name given the context of test (e.g., Compile)
				SubN: []Worker{
					&Exec{
						Name:      "ls -al",
						Token:     "ls-al",
						Directory: rootDir,
						Command: func() *exec.Cmd {
							return exec.Command("ls", "-al")
						},
						Stdout: TailReportOn,
						Stderr: TailReportOnError,
					},
					&Wait{
						Name:  "Wait for ls -al to finish",
						Token: "ls-al",
					},
				},
			},
			&Comp{
				Name: "Exec/Kill test", // or some other meaningful name given the context of test (e.g., Compile)
				SubN: []Worker{
					&Exec{
						Name:      "sleep 5",
						Token:     "sleep-5",
						Directory: rootDir,
						Command: func() *exec.Cmd {
							return exec.Command("sleep", "5")
						},
						Stdout: TailReportOn,
						Stderr: TailReportOnError,
					},
					&Kill{
						Name:  "Kill sleep 5",
						Token: "sleep-5",
					},
				},
			},
			&Delay{
				Name:     "Delay test",
				Duration: time.Second * 1,
			},
			&DelayHealthCheck{
				Name:         "DelayHealthCheck test - xfinity.com",
				PollInterval: time.Second,
				NPolls:       5,
				URL:          "http://www.xfinity.com",
			},
			&GoFunc{
				Name: "GoFunc test",
				Func: func(w *GoFunc) {
					Info.Print("Successful GoFunc test") // Can access variables via a closure
				},
			},
			&Curl{
				Name: "Curl test",
				URL:  "http://www.xfinity.com", // This could also tickle a REST endpoint with parameters. Any URL works.
			},
			// TODO: add later to demo this pattern?
			//&Comp{
			//	Name: "TestCases",
			//	SubN: zkTestCases,
			//},
		},
	}

	fmt.Printf("Welcome to Zookeeper integration test\n")

	gointTest.Start()
	if err := gointTest.Report(0); err != nil {
		t.Errorf("Error occured: %v", err)
	}
}
