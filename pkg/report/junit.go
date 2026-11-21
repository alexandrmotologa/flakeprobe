package report

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
)

type jUnitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []jUnitTestSuite `xml:"testsuite"`
}

type jUnitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Time      float64         `xml:"time,attr"`
	Timestamp string          `xml:"timestamp,attr"`
	TestCases []jUnitTestCase `xml:"testcase"`
}

type jUnitTestCase struct {
	Classname string        `xml:"classname,attr"`
	Name      string        `xml:"name,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *jUnitFailure `xml:"failure,omitempty"`
}

type jUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// ExportJUnitXML writes standard JUnit XML for CI test gate integration.
func ExportJUnitXML(filePath string, analysis *entropy.FlakinessAnalysis, targetDesc string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	suite := jUnitTestSuite{
		Name:      fmt.Sprintf("flakeprobe.%s", targetDesc),
		Tests:     3,
		Failures:  0,
		Errors:    0,
		Time:      0.05,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// Test 1: Flakiness Risk Index Gate
	tcFRI := jUnitTestCase{
		Classname: "flakeprobe.reliability",
		Name:      "test_flakiness_risk_index_gate",
		Time:      0.01,
	}
	if analysis.IsUnstable {
		suite.Failures++
		tcFRI.Failure = &jUnitFailure{
			Message: fmt.Sprintf("FRI %.4f exceeded maximum allowed threshold of %.4f", analysis.FRI, analysis.Threshold),
			Type:    "FlakinessGateViolation",
			Body:    fmt.Sprintf("Archetype: %s\nMean Trajectory Distance: %.4f\nMax Entropy: %.4f", analysis.Archetype, analysis.DistanceProfile.MeanNormalizedDistance, analysis.EntropyProfile.MaxNodeEntropy),
		}
	}
	suite.TestCases = append(suite.TestCases, tcFRI)

	// Test 2: Node Bifurcation Entropy
	tcEntropy := jUnitTestCase{
		Classname: "flakeprobe.entropy",
		Name:      "test_decision_node_entropy",
		Time:      0.01,
	}
	if analysis.EntropyProfile.MaxNodeEntropy >= 1.0 {
		suite.Failures++
		tcEntropy.Failure = &jUnitFailure{
			Message: fmt.Sprintf("Critical decision bifurcation observed (H = %.4f bits)", analysis.EntropyProfile.MaxNodeEntropy),
			Type:    "BifurcationViolation",
			Body:    "Agent branched unpredictably into disparate tool branches.",
		}
	}
	suite.TestCases = append(suite.TestCases, tcEntropy)

	// Test 3: Outcome Consistency
	tcOutcome := jUnitTestCase{
		Classname: "flakeprobe.consistency",
		Name:      "test_outcome_consistency",
		Time:      0.01,
	}
	if analysis.SuccessRate > 0 && analysis.SuccessRate < 1.0 {
		suite.Failures++
		tcOutcome.Failure = &jUnitFailure{
			Message: fmt.Sprintf("Discordant execution outcomes: success rate is %.1f%%", analysis.SuccessRate*100.0),
			Type:    "DiscordantOutcome",
			Body:    "Identical inputs produced mixed successes and failures across runs.",
		}
	}
	suite.TestCases = append(suite.TestCases, tcOutcome)

	suites := jUnitTestSuites{
		Suites: []jUnitTestSuite{suite},
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create junit file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(xml.Header); err != nil {
		return err
	}

	enc := xml.NewEncoder(file)
	enc.Indent("", "  ")
	return enc.Encode(suites)
}
