package netobserv

import (
	"flag"
	"testing"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	filePath "path/filepath"
	exutil "github.com/openshift/origin/test/extended/util"
	// compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
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

func TestBackend(t *testing.T) {
	// Enable test execution (sets testsStarted flag)
	exutil.WithCleanup(func() {
		RegisterFailHandler(Fail)

		suiteConfig, reporterConfig := GinkgoConfiguration()

		if len(suiteConfig.FocusStrings) > 0 {
			combinedFocus := make([]string, len(suiteConfig.FocusStrings))
			for i, userFocus := range suiteConfig.FocusStrings {
				combinedFocus[i] = "sig-netobserv.*" + userFocus
			}
			suiteConfig.FocusStrings = combinedFocus
		} else {
			suiteConfig.FocusStrings = []string{"sig-netobserv"}
		}

		RunSpecs(t, "Backend Suite", suiteConfig, reporterConfig)
	})
}
