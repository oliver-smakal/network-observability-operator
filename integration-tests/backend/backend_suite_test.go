package netobserv

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	exutil "github.com/openshift/origin/test/extended/util"
	filePath "path/filepath"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
)

func init() {
	// Initialize framework flags - must be done before flag.Parse()
	exutil.InitStandardFlags()
}

var _ = BeforeSuite(func() {
	// Parse flags
	flag.Parse()

	// Set up provider config after parsing flags
	e2eframework.AfterReadingAllFlags(exutil.TestContext)

	// Initialize test
	Expect(exutil.InitTest(false)).NotTo(HaveOccurred())

	// example of before suite working - check the noo exists 
	if os.Getenv("CHECK_NOO_EXISTS") == "TRUE" {
		baseDir, _ := filePath.Abs("./testdata")
		subscriptionDir := filePath.Join(baseDir,"subscription")
		oc := exutil.NewCLIForMonitorTest("netobserv")

		NOcatSrc := Resource{"catsrc", "netobserv-konflux-fbc", netobservNS}
		NOSource := CatalogSourceObjects{"stable", NOcatSrc.Name, NOcatSrc.Namespace}


		NO := SubscriptionObjects{
			OperatorName:  "netobserv-operator",
			Namespace:     netobservNS,
			PackageName:   NOPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: &NOSource,
		}

		// NetObserv-specific checks only if operator was just deployed
		NOexisting, err := CheckOperatorStatus(oc, NO.Namespace, NO.PackageName)
		Expect(err).NotTo(HaveOccurred())
		Expect(NOexisting).To(BeTrue())
		// Verify FlowCollector API exists
		flowcollectorAPIExists, err := isFlowCollectorAPIExists(oc)
		Expect(flowcollectorAPIExists).To(BeTrue())
		Expect(err).NotTo(HaveOccurred())
	}

})

type testSpec struct {
	name string
	spec types.TestSpec
}

type testResults struct {
	passed  int
	failed  int
	skipped int
	total   int
}

// applyFocusFilter combines user focus strings with the sig-netobserv prefix
func applyFocusFilter(suiteConfig types.SuiteConfig) []string {
	if len(suiteConfig.FocusStrings) > 0 {
		combinedFocus := make([]string, len(suiteConfig.FocusStrings))
		for i, userFocus := range suiteConfig.FocusStrings {
			combinedFocus[i] = "sig-netobserv.*" + userFocus
		}
		return combinedFocus
	}
	return []string{"sig-netobserv"}
}

// collectMatchingTests walks the test tree and collects tests matching focus patterns
func collectMatchingTests(focusStrings []string) []testSpec {
	if !GetSuite().InPhaseBuildTree() {
		_ = GetSuite().BuildTree()
	}

	var tests []testSpec
	GetSuite().WalkTests(func(name string, spec types.TestSpec) {
		for _, focus := range focusStrings {
			if regexp.MustCompile(focus).MatchString(name) {
				tests = append(tests, testSpec{name: name, spec: spec})
				break
			}
		}
	})
	return tests
}

// runTestsIndividually executes each test separately and tracks results
func runTestsIndividually(tests []testSpec, suiteConfig types.SuiteConfig, reporterConfig types.ReporterConfig) testResults {
	cwd, _ := os.Getwd()
	results := testResults{total: len(tests)}

	for _, test := range tests {
		config := suiteConfig
		config.FocusStrings = []string{"^" + regexp.QuoteMeta(test.name) + "$"}

		GetSuite().RunSpec(
			test.spec,
			Labels{},
			"Backend Suite",
			cwd,
			GetFailer(),
			GetWriter(),
			config,
			reporterConfig,
		)

		report := GetSuite().GetReport()
		if len(report.SpecReports) > 0 {
			specReport := report.SpecReports[len(report.SpecReports)-1]
			switch specReport.State {
			case types.SpecStatePassed:
				results.passed++
			case types.SpecStateFailed, types.SpecStatePanicked, types.SpecStateInterrupted:
				results.failed++
			case types.SpecStateSkipped:
				results.skipped++
			}
		}
	}

	return results
}

// printTestSummary outputs the test execution summary
func printTestSummary(results testResults) {
	fmt.Printf("\n")
	fmt.Printf("Ran %d tests\n", results.total)
	fmt.Printf("Passed: %d, Failed: %d, Skipped: %d\n", results.passed, results.failed, results.skipped)
}

func TestBackend(t *testing.T) {
	exutil.WithCleanup(func() {
		RegisterFailHandler(Fail)

		suiteConfig, reporterConfig := GinkgoConfiguration()

		focusStrings := applyFocusFilter(suiteConfig)
		suiteConfig.FocusStrings = focusStrings

		suiteConfig.EmitSpecProgress = true
		suiteConfig.OutputInterceptorMode = "none"
		reporterConfig.NoColor = true
		reporterConfig.Succinct = true
		SetReporterConfig(reporterConfig)

		tests := collectMatchingTests(focusStrings)
		results := runTestsIndividually(tests, suiteConfig, reporterConfig)

		printTestSummary(results)
		if results.failed > 0 {
			t.Fail()
		}
	})
}
