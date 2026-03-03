# copy test files.

```
~/Repos/openshift-tests-private/test/extended$ cp netobserv/* ~/Repos/network-observability-operator/integration-tests/backend/
```


# copy dependencies until compilation succesful

copy the files
```
~/Repos/openshift-tests-private/test/extended$ cp -r util ~/Repos/network-observability-operator/integration-tests/backend/
~/Repos/openshift-tests-private/test/extended$ cp -r scheme ~/Repos/network-observability-operator/integration-tests/backend/
~/Repos/openshift-tests-private/test/extended$ cp -r testdata ~/Repos/network-observability-operator/integration-tests/backend/
```
add to go.mod replace commands, use the origin go.mod as a teamplate
```go
replace (
	github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20240806135314-3946b2b7b2a8
	bitbucket.org/ww/goautoneg => github.com/munnerz/goautoneg v0.0.0-20120707110453-a547fc61f48d
	github.com/jteeuwen/go-bindata => github.com/jteeuwen/go-bindata v3.0.8-0.20151023091102-a0ff2567cfb7+incompatible
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc => go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.53.0
#and more...
)
```

When executing go mod tidy `export GOPRIVATE="github.com/openshift/*"` is requied.

The following addition to go.mod:
```go
exclude (
  // Exclude old unified containerd to avoid conflicts with split API module
  github.com/containerd/containerd v1.7.18
)
```
fixes `go mod tidy` issues.


# adjust the execution of tests to ignore the kubernetes tests

As k8s.io/kubernetes/test/e2e/framework is imported, it triggters tests inside of that package -> the test suite need to be adjusted to ignore these by default.
