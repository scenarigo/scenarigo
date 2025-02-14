package reporter

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// GenerateTestReport generates a test result report from r.
func GenerateTestReport(r Reporter) (*TestReport, error) {
	if r == nil {
		return nil, errors.New("reporter is nil")
	}
	if !r.isRoot() {
		return nil, errors.New("must be a root reporter")
	}
	report := &TestReport{
		Name:   r.getName(),
		Result: testResult(r),
	}
	for _, file := range r.getChildren() {
		fileReport := ScenarioFileReport{
			Name:     file.getName(),
			Result:   testResult(file),
			Duration: TestDuration(file.getDuration()),
		}
		for _, scenario := range file.getChildren() {
			scenarioReport := ScenarioReport{
				Name:     scenario.getName(),
				File:     file.getName(),
				Result:   testResult(scenario),
				Duration: TestDuration(scenario.getDuration()),
			}
			for _, step := range scenario.getChildren() {
				logs := step.getLogs()
				stepReport := StepReport{
					Name:     step.getName(),
					Result:   testResult(step),
					Duration: TestDuration(step.getDuration()),
					Logs: ReportLogs{
						Info:  logs.infoLogs(),
						Error: logs.errorLogs(),
						Skip:  logs.skipLog(),
					},
					SubSteps: generateSubStepReports(step),
				}
				scenarioReport.Steps = append(scenarioReport.Steps, stepReport)
			}
			fileReport.Scenarios = append(fileReport.Scenarios, scenarioReport)
		}
		report.Files = append(report.Files, fileReport)
	}
	return report, nil
}

func generateSubStepReports(r Reporter) []SubStepReport {
	children := r.getChildren()
	if len(children) == 0 {
		return nil
	}
	reports := make([]SubStepReport, len(children))
	for i, child := range children {
		logs := child.getLogs()
		reports[i] = SubStepReport{
			Name:     child.getName(),
			Result:   testResult(child),
			Duration: TestDuration(child.getDuration()),
			Logs: ReportLogs{
				Info:  logs.infoLogs(),
				Error: logs.errorLogs(),
				Skip:  logs.skipLog(),
			},
			SubSteps: generateSubStepReports(child),
		}
	}
	return reports
}

func TestResultString(r Reporter) string {
	return testResult(r).String()
}

func testResult(r Reporter) TestResult {
	if r.Failed() {
		return TestResultFailed
	}
	if r.Skipped() {
		return TestResultSkipped
	}
	return TestResultPassed
}

/*
TestReport represents a test result report.
This struct can be marshalled as JUnit-like format XML.

	b, err := xml.MarshalIndent(test.report, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(b)
*/
type TestReport struct {
	XMLName xml.Name             `json:"-"              xml:"testsuites"`
	Name    string               `json:"name,omitempty" xml:"name,attr,omitempty"`
	Result  TestResult           `json:"result"         xml:"-"`
	Files   []ScenarioFileReport `json:"files"          xml:"testsuite"`
}

// ScenarioFileReport represents a result report of a test scenario file.
type ScenarioFileReport struct {
	Name      string           `json:"name"      xml:"name,attr,omitempty"`
	Result    TestResult       `json:"result"    xml:"-"`
	Duration  TestDuration     `json:"duration"  xml:"time,attr"`
	Scenarios []ScenarioReport `json:"scenarios" xml:"testcase"`
}

type xmlScenarioFileReport ScenarioFileReport

// MarshalXML implements xml.Marshaler interface.
func (r ScenarioFileReport) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	var failures int
	for _, scenario := range r.Scenarios {
		if scenario.Result == TestResultFailed {
			failures++
		}
	}
	start.Attr = append(start.Attr,
		xml.Attr{
			Name: xml.Name{
				Local: "tests",
			},
			Value: fmt.Sprint(len(r.Scenarios)),
		},
		xml.Attr{
			Name: xml.Name{
				Local: "failures",
			},
			Value: fmt.Sprint(failures),
		},
	)
	return e.EncodeElement(xmlScenarioFileReport(r), start)
}

// ScenarioReport represents a result report of a test scenario.
type ScenarioReport struct {
	Name     string       `json:"name"`
	File     string       `json:"-"`
	Result   TestResult   `json:"result"`
	Duration TestDuration `json:"duration"`
	Steps    []StepReport `json:"steps"`
}

type xmlScenarioReport struct {
	Name      string                   `xml:"name,attr,omitempty"`
	File      string                   `xml:"file,attr,omitempty"`
	Duration  TestDuration             `xml:"time,attr"`
	Failure   *xmlScenarioReportDetail `xml:"failure,omitempty"`
	Skipped   *xmlScenarioReportDetail `xml:"skipped,omitempty"`
	SystemOut *xmlCDATA                `xml:"system-out,omitempty"`
}

type xmlScenarioReportDetail struct {
	Message string `xml:"message,attr,omitempty"`
	Logs    string `xml:",chardata"`
}

type xmlCDATA struct {
	CDATA string `xml:",cdata"`
}

// MarshalXML implements xml.Marshaler interface.
func (r ScenarioReport) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	xr := &xmlScenarioReport{
		Name:     r.Name,
		File:     r.File,
		Duration: r.Duration,
	}
	switch r.Result {
	case TestResultFailed:
		for _, step := range r.Steps {
			if step.Result == TestResultFailed {
				if len(step.Logs.Info) > 0 {
					xr.SystemOut = &xmlCDATA{
						CDATA: strings.Join(step.Logs.Info, "\n"),
					}
				}
				xr.Failure = &xmlScenarioReportDetail{
					Message: step.Name,
					Logs:    strings.Join(step.Logs.Error, "\n"),
				}
				break
			}
		}
	case TestResultSkipped:
		for _, step := range r.Steps {
			if step.Result == TestResultSkipped {
				xr.Skipped = &xmlScenarioReportDetail{
					Message: step.Name,
				}
				if step.Logs.Skip != nil {
					xr.Skipped.Logs = *step.Logs.Skip
				}
				break
			}
		}
	default:
	}
	return e.EncodeElement(xr, start)
}

// StepReport represents a result report of a test scenario step.
type StepReport struct {
	Name     string          `json:"name"`
	Result   TestResult      `json:"result"`
	Duration TestDuration    `json:"duration"`
	Logs     ReportLogs      `json:"logs"`
	SubSteps []SubStepReport `json:"subSteps,omitempty"`
}

type ReportLogs struct {
	Info  []string `json:"info,omitempty"`
	Error []string `json:"error,omitempty"`
	Skip  *string  `json:"skip,omitempty"`
}

// SubStepReport represents a result report of a test scenario sub step.
type SubStepReport struct {
	Name     string          `json:"name"`
	Result   TestResult      `json:"result"`
	Duration TestDuration    `json:"duration"`
	Logs     ReportLogs      `json:"logs"`
	SubSteps []SubStepReport `json:"subSteps,omitempty"`
}

// TestResult represents a test result.
type TestResult int

const (
	TestResultUndefined TestResult = iota
	TestResultPassed
	TestResultFailed
	TestResultSkipped

	testResultUndefinedString = "undefined"
	testResultPassedString    = "passed"
	testResultFailedString    = "failed"
	testResultSkippedString   = "skipped"
)

// String returns r as a string.
func (r TestResult) String() string {
	switch r {
	case TestResultPassed:
		return testResultPassedString
	case TestResultFailed:
		return testResultFailedString
	case TestResultSkipped:
		return testResultSkippedString
	default:
		return testResultUndefinedString
	}
}

// MarshalJSON implements json.Marshaler interface.
func (r TestResult) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", r.String())), nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (r *TestResult) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case testResultPassedString:
		*r = TestResultPassed
	case testResultFailedString:
		*r = TestResultFailed
	case testResultSkippedString:
		*r = TestResultSkipped
	case testResultUndefinedString:
		*r = TestResultUndefined
	default:
		return fmt.Errorf("invalid test result %s", s)
	}
	return nil
}

// MarshalYAML implements yaml.Marshaler interface.
func (r TestResult) MarshalYAML() ([]byte, error) {
	return []byte(r.String()), nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (r *TestResult) UnmarshalYAML(b []byte) error {
	var s string
	if err := yaml.Unmarshal(b, &s); err != nil {
		return err
	}
	switch s {
	case testResultPassedString:
		*r = TestResultPassed
	case testResultFailedString:
		*r = TestResultFailed
	case testResultSkippedString:
		*r = TestResultSkipped
	case testResultUndefinedString:
		*r = TestResultUndefined
	default:
		return fmt.Errorf("invalid test result %s", s)
	}
	return nil
}

// TestDuration represents an elapsed time of a test.
type TestDuration time.Duration

// MarshalJSON implements json.Marshaler interface.
func (d TestDuration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%q", time.Duration(d).String())), nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (d *TestDuration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	td, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = TestDuration(td)
	return nil
}

// MarshalYAML implements yaml.Marshaler interface.
func (d TestDuration) MarshalYAML() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface.
func (d *TestDuration) UnmarshalYAML(b []byte) error {
	var s string
	if err := yaml.Unmarshal(b, &s); err != nil {
		return err
	}
	td, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = TestDuration(td)
	return nil
}

// MarshalYAML implements xml.Marshaler interface.
func (d TestDuration) MarshalXMLAttr(name xml.Name) (xml.Attr, error) {
	return xml.Attr{
		Name:  name,
		Value: fmt.Sprintf("%f", time.Duration(d).Seconds()),
	}, nil
}
