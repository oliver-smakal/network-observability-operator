package netobserv

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	exutil "github.com/openshift/origin/test/extended/util"
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
