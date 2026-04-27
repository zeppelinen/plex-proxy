//go:build e2ehelper

package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	results := []testResult{
		runCheck("HTTP proxy forwards Plex requests", checkHTTP),
		runCheck("GDM proxy forwards discovery", checkGDM),
	}

	junitOutput := os.Getenv("JUNIT_OUTPUT")
	if err := writeJUnit(junitOutput, results); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if junitOutput != "" {
			results = append(results, testResult{
				Name:    "Write JUnit report",
				Failure: err.Error(),
			})
		}
	}

	for _, result := range results {
		if result.Failure != "" {
			fmt.Fprintln(os.Stderr, result.Failure)
			os.Exit(1)
		}
	}
}

type testResult struct {
	Name    string
	Time    time.Duration
	Failure string
}

func runCheck(name string, check func() error) testResult {
	start := time.Now()
	deadline := start.Add(90 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if err := check(); err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		return testResult{Name: name, Time: time.Since(start)}
	}
	if last == nil {
		last = fmt.Errorf("timed out")
	}
	return testResult{Name: name, Time: time.Since(start), Failure: last.Error()}
}

func checkHTTP() error {
	resp, err := http.Get("http://proxy:32400/")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	if body["ok"] != "true" {
		return fmt.Errorf("unexpected body: %v", body)
	}
	if body["host"] != "plex:32400" {
		return fmt.Errorf("unexpected host: %v", body)
	}
	if body["x-forwarded-proto"] != "http" {
		return fmt.Errorf("missing forwarded proto: %v", body)
	}
	return nil
}

type junitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	ClassName string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitMessage `xml:"failure,omitempty"`
}

type junitMessage struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

func writeJUnit(path string, results []testResult) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var total time.Duration
	suite := junitTestSuite{Name: "e2e", Tests: len(results)}
	for _, result := range results {
		total += result.Time
		testCase := junitTestCase{
			ClassName: "e2e",
			Name:      result.Name,
			Time:      seconds(result.Time),
		}
		if result.Failure != "" {
			suite.Failures++
			testCase.Failure = &junitMessage{Message: result.Failure, Text: result.Failure}
		}
		suite.Cases = append(suite.Cases, testCase)
	}
	suite.Time = seconds(total)

	report := junitTestSuites{
		Tests:    suite.Tests,
		Failures: suite.Failures,
		Time:     suite.Time,
		Suites:   []junitTestSuite{suite},
	}
	data, err := xml.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	data = append([]byte(xml.Header), data...)
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func seconds(d time.Duration) string {
	return fmt.Sprintf("%.3f", d.Seconds())
}

func checkGDM() error {
	addr, err := net.ResolveUDPAddr("udp4", "proxy:32410")
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("M-SEARCH * HTTP/1.1\r\n\r\n")); err != nil {
		return err
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	resp := string(buf[:n])
	for _, want := range []string{"Content-Type: plex/media-server", "Name: E2E Plex", "Port: 32400", "Host: proxy"} {
		if !strings.Contains(resp, want) {
			return fmt.Errorf("gdm response missing %q: %s", want, resp)
		}
	}
	return nil
}
