package e2etests

import (
	"flag"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	exutil "github.com/openshift/origin/test/extended/util"
	e2eframework "k8s.io/kubernetes/test/e2e/framework"
	filePath "path/filepath"
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
		subscriptionDir := filePath.Join(baseDir, "subscription")
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

func TestBackend(t *testing.T) {
	exutil.WithCleanup(func() {
		RegisterFailHandler(Fail)

		suiteConfig, reporterConfig := GinkgoConfiguration()

		// Apply focus filter

		if len(suiteConfig.FocusStrings) > 0 {
			combinedFocus := make([]string, len(suiteConfig.FocusStrings))
			for i, userFocus := range suiteConfig.FocusStrings {
				combinedFocus[i] = "sig-netobserv.*" + userFocus
			}
			suiteConfig.FocusStrings = combinedFocus
		} else {
			suiteConfig.FocusStrings = []string{"sig-netobserv"}
		}

		// Configure reporter - suppress default verbose output
		suiteConfig.EmitSpecProgress = true
		suiteConfig.OutputInterceptorMode = "none"
		reporterConfig.SilenceSkips = true // Hide the "S" characters for skipped tests
		reporterConfig.NoColor = true
		reporterConfig.Succinct = true
		reporterConfig.Verbose = false

		// Standard Ginkgo run with custom reporting via hooks
		RunSpecs(t, "Backend Suite", suiteConfig, reporterConfig)
	})
}

// Custom reporting hooks

var _ = ReportBeforeSuite(func(report Report) {
	fmt.Println("\n--------------------REPORT_BEFORE_SUITE_START--------------------")
	defer fmt.Println("\n--------------------REPORT_BEFORE_SUITE_END--------------------")

	fmt.Printf("Running Suite: %s - %s\n", report.SuiteDescription, report.SuitePath)
	fmt.Printf("==========================================================================================================\n")
	fmt.Printf("Random Seed: %d\n\n", report.SuiteConfig.RandomSeed)
	fmt.Printf("Will run %d specs\n", report.PreRunStats.SpecsThatWillRun)
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Only report on specs that actually ran (not skipped on via focused filter)
	if report.State == types.SpecStateSkipped && report.Failure.Message == "" && report.RunTime <= 0{
		return
	}

	if report.LeafNodeType != types.NodeTypeIt {
		return
	}

	fmt.Println("--------------------REPORT_AFTER_EACH_START--------------------")
	defer fmt.Println("--------------------REPORT_AFTER_EACH_END--------------------")

	// Print spec progress
	fmt.Printf("%s\n", report.FullText())
	fmt.Printf("%s\n", report.LeafNodeLocation.String())

	// Print result
	switch report.State {
	case types.SpecStatePassed:
		fmt.Printf("• PASSED [%.3f seconds]\n", report.RunTime.Seconds())
	case types.SpecStateSkipped:
		fmt.Printf("• SKIPPED [%.3f seconds]\n", report.RunTime.Seconds())
		if report.Failure.Message != "" {
			fmt.Printf("\n%s\n", report.Failure.Message)
			fmt.Printf("%s\n", report.Failure.Location.String())
		}
	case types.SpecStateFailed, types.SpecStatePanicked, types.SpecStateInterrupted:
		fmt.Printf("• FAILED [%.3f seconds]\n", report.RunTime.Seconds())
		if report.Failure.Message != "" {
			fmt.Printf("\n%s\n", report.Failure.Message)
			fmt.Printf("%s\n", report.Failure.Location.String())
		}
	}
})

var _ = ReportAfterSuite("NetObserv Summary", func(report Report) {
	fmt.Println("\n--------------------REPORT_AFTER_SUITE_START--------------------")
	defer fmt.Println("\n--------------------REPORT_AFTER_SUITE_END--------------------")

	passed := 0
	failed := 0
	skipped := 0
	ranTests := 0

	// Get only the test specs (not setup/teardown)
	specs := report.SpecReports.WithLeafNodeType(types.NodeTypeIt)

	for _, specReport := range specs {
		switch specReport.State {
		case types.SpecStatePassed:
			passed++
			ranTests++
		case types.SpecStateFailed, types.SpecStatePanicked, types.SpecStateInterrupted:
			failed++
			ranTests++
		case types.SpecStateSkipped:
			// Skip filtered-out specs: they have State==Skipped but RunTime==0 and no failure info
			// Explicitly skipped specs (via Skip()) have State==Skipped but were actually evaluated
			if specReport.Failure.Message != "" || specReport.RunTime > 0 {
				// Explicitly skipped - test body was evaluated
				skipped++
			}
		}
	}

	// Total specs evaluated (passed + failed + explicitly skipped)
	totalEvaluated := ranTests + skipped

	fmt.Printf("------------------------------\n")
	if report.SuiteSucceeded {
		fmt.Printf("\nBackend Suite - %d/%d specs • SUCCESS! [%.3f seconds]\n",
			totalEvaluated, totalEvaluated, report.RunTime.Seconds())
	} else {
		fmt.Printf("\nBackend Suite - %d/%d specs • FAILURE! [%.3f seconds]\n",
			totalEvaluated, totalEvaluated, report.RunTime.Seconds())
	}

	fmt.Printf("\nRan %d tests\n", ranTests)
	fmt.Printf("Passed: %d, Failed: %d, Skipped: %d\n", passed, failed, skipped)
})
