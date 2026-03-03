package netobserv

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBackend(t *testing.T) {
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
}
