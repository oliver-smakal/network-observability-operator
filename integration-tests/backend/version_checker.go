package e2etests

import (
    "context"
    "fmt"
    "strconv"
    "strings"
    
    . "github.com/onsi/ginkgo/v2"
    configv1 "github.com/openshift/api/config/v1"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

type OCPVersion struct {
    Major int
    Minor int
}

var clusterVersion *OCPVersion

func GetOCPVersion(ctx context.Context, k8sClient client.Client) (*OCPVersion, error) {
    if clusterVersion != nil {
        return clusterVersion, nil
    }
    
    cv := &configv1.ClusterVersion{}
    err := k8sClient.Get(ctx, types.NamespacedName{Name: "version"}, cv)
    if err != nil {
        return nil, err
    }
    
    version := cv.Status.Desired.Version
    parts := strings.Split(version, ".")
    if len(parts) < 2 {
        return nil, fmt.Errorf("invalid version: %s", version)
    }
    
    major, _ := strconv.Atoi(parts[0])
    minor, _ := strconv.Atoi(parts[1])
    
    clusterVersion = &OCPVersion{Major: major, Minor: minor}
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
