package e2etests

import (
    "fmt"
    "strconv"
    "strings"
    
    . "github.com/onsi/ginkgo/v2"
     exutil "github.com/openshift/origin/test/extended/util"
)

type OCPVersion struct {
    Major int
    Minor int
}

var clusterVersion *OCPVersion

// Will run 1 specs
// vesionstring:  '4.20.0-0.nightly-2026-04-22-115050'
// parts:  ['4 20 0-0 nightly-2026-04-22-115050']
// Detected OCP 0.20

func GetOCPVersion(oc *exutil.CLI,) (*OCPVersion, error) {

    if clusterVersion != nil {
        return clusterVersion, nil
    }

    version, err := oc.AsAdmin().WithoutNamespace().Run("get").Args("clusterversion", "-o=jsonpath={.items[0].status.desired.version}").Output()
    if err != nil {
        return nil, err
    }

    fmt.Println("vesionstring: ", version)
    parts := strings.Split(version, ".")
    fmt.Println("parts: ", parts)
    if len(parts) < 2 {
        return nil, fmt.Errorf("invalid version: %s", version)
    }
    
    major, _ := strconv.Atoi(parts[0])
    fmt.Println("converted ", parts[0], "to",  major)
    minor, _ := strconv.Atoi(parts[1])
    fmt.Println("converted ", parts[1], "to",  minor)
    
    clusterVersion = &OCPVersion{Major: major, Minor: minor}
    fmt.Println("Detected OCP", clusterVersion )
    return clusterVersion, nil
}

func (v *OCPVersion) AtLeast(major, minor int) bool {
    if v.Major > major {
        return true
    }
    return v.Major == major && v.Minor >= minor
}

func (v *OCPVersion) String() string {
    return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// SkipIfOCPBelow skips test if cluster version is below requirement
func SkipIfOCPBelow(major, minor int) {
    if clusterVersion == nil {
        Fail("Cluster version not initialized")
    }
    if !clusterVersion.AtLeast(major, minor) {
        Skip(fmt.Sprintf("Requires OCP %d.%d+, cluster is %s", major, minor, clusterVersion))
    }
}
