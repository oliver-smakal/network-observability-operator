package netobserv

import (
	"encoding/json"
	"os"
	"time"

	filePath "path/filepath"
	"strings"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	exutil "github.com/network-observability-operator/integration-tests/backend/util"
	e2e "k8s.io/kubernetes/test/e2e/framework"
	e2eoutput "k8s.io/kubernetes/test/e2e/framework/pod/output"
)

var _ = g.Describe("[sig-netobserv] Network_Observability", func() {

	defer g.GinkgoRecover()
	var (
		oc = exutil.NewCLI("netobserv", exutil.KubeConfigPath())
		// NetObserv Operator variables
		NOcatSrc = Resource{"catsrc", "netobserv-konflux-fbc", netobservNS}
		NOSource = CatalogSourceObjects{"stable", NOcatSrc.Name, NOcatSrc.Namespace}

		// Template directories
		baseDir              = exutil.FixturePath("testdata", "netobserv")
		subscriptionDir      = exutil.FixturePath("testdata", "netobserv", "subscription")
		flowFixturePath      = filePath.Join(baseDir, "flowcollector_v1beta2_template.yaml")
		flowSliceFixturePath = filePath.Join(baseDir, "flowcollectorSlice_v1alpha1_template.yaml")

		// Operator namespace object
		OperatorNS = OperatorNamespace{
			Name:              netobservNS,
			NamespaceTemplate: filePath.Join(subscriptionDir, "namespace.yaml"),
		}
		NO = SubscriptionObjects{
			OperatorName:  "netobserv-operator",
			Namespace:     netobservNS,
			PackageName:   NOPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: &NOSource,
		}
		imageDigest    = filePath.Join(subscriptionDir, "image-digest-mirror-set.yaml")
		catSrcTemplate = filePath.Join(subscriptionDir, "catalog-source.yaml")
		catalogSource  = os.Getenv("MULTISTAGE_PARAM_OVERRIDE_NETOBSERV_CS_IMAGE")

		kubeadminToken  string
		kubeAdminPasswd = os.Getenv("QE_KUBEADMIN_PASSWORD")
		namespace       string

		// Loki Operator variables
		lokiDir         = exutil.FixturePath("testdata", "netobserv", "loki")
		lokiPackageName = "loki-operator"
		lokiSource      CatalogSourceObjects
		ls              *lokiStack
		Lokiexisting    = false
		lokiStackNS     = "netobserv-loki"
		LO              = SubscriptionObjects{
			OperatorName:  "loki-operator-controller-manager",
			Namespace:     loNS,
			PackageName:   lokiPackageName,
			Subscription:  filePath.Join(subscriptionDir, "sub-template.yaml"),
			OperatorGroup: filePath.Join(subscriptionDir, "allnamespace-og.yaml"),
			CatalogSource: &lokiSource,
		}

		// LokiStack variables
		ipStackType       string
		lokiStackTemplate = filePath.Join(lokiDir, "lokistack-simple.yaml")
		lokiTenant        = "openshift-network"
	)

	g.BeforeEach(func() {
		if strings.Contains(os.Getenv("E2E_RUN_TAGS"), "disconnected") {
			g.Skip("Skipping tests for disconnected profiles")
		}
		namespace = oc.Namespace()

		g.By("Get kubeadmin token")
		if kubeAdminPasswd == "" {
			g.Skip("no kubeAdminPasswd is provided in this profile, set QE_KUBEADMIN_PASSWORD env var")
		}
		serverUrl, serverUrlErr := oc.AsAdmin().WithoutNamespace().Run("whoami").Args("--show-server").Output()
		o.Expect(serverUrlErr).NotTo(o.HaveOccurred())
		currentContext, currentContextErr := oc.WithoutNamespace().Run("config").Args("current-context").Output()
		o.Expect(currentContextErr).NotTo(o.HaveOccurred())
		defer func() {
			rollbackCtxErr := oc.WithoutNamespace().Run("config").Args("set", "current-context", currentContext).Execute()
			o.Expect(rollbackCtxErr).NotTo(o.HaveOccurred())
		}()

		kubeadminToken = getKubeAdminToken(oc, kubeAdminPasswd, serverUrl, currentContext)
		o.Expect(kubeadminToken).NotTo(o.BeEmpty())

		isHypershift := exutil.IsHypershiftHostedCluster(oc)

		// Deploy NetObserv operator
		OperatorNS.DeployOperatorNamespace(oc)
		deployedUpstreamCatalogSource, catSrcErr := setupCatalogSource(oc, NOcatSrc, catSrcTemplate, imageDigest, catalogSource, isHypershift, &NOSource, &NO)
		o.Expect(catSrcErr).NotTo(o.HaveOccurred())
		ensureNetObservOperatorDeployed(oc, NO, NOSource, deployedUpstreamCatalogSource)

		ipStackType = checkIPStackType(oc)

		g.By("Deploy loki operator")
		if !validateInfraAndResourcesForLoki(oc, "10Gi", "6") {
			g.Skip("Current platform does not have enough resources available for this test!")
		}

		// check if Loki Operator exists
		var err error
		Lokiexisting, err = CheckOperatorStatus(oc, LO.Namespace, LO.PackageName)
		o.Expect(err).NotTo(o.HaveOccurred())

		lokiChannel, err := getLokiChannel(oc, "redhat-operators")
		if err != nil || lokiChannel == "" {
			g.Skip("Loki channel not found, skip this case")
		}
		lokiSource = CatalogSourceObjects{lokiChannel, "redhat-operators", "openshift-marketplace"}

		// Don't delete if Loki Operator existed already before NetObserv
		//  unless it is not using the 'stable' operator
		// If Loki Operator was installed by NetObserv tests,
		//  it will install and uninstall after each spec/test.
		if !Lokiexisting {
			ensureOperatorDeployed(oc, LO, lokiSource, "name="+LO.OperatorName)
		} else {
			channelName, err := checkOperatorChannel(oc, LO.Namespace, LO.PackageName)
			o.Expect(err).NotTo(o.HaveOccurred())
			if channelName != lokiChannel {
				e2e.Logf("found %s channel for loki operator, removing and reinstalling with %s channel instead", channelName, lokiSource.Channel)
				LO.uninstallOperator(oc)
				ensureOperatorDeployed(oc, LO, lokiSource, "name="+LO.OperatorName)
				Lokiexisting = false
			}
		}

		g.By("Deploy lokiStack")
		// get storageClass Name
		sc, err := getStorageClassName(oc)
		if err != nil || len(sc) == 0 {
			g.Skip("StorageClass not found in cluster, skip this case")
		}

		objectStorageType := getStorageType(oc)
		if len(objectStorageType) == 0 && ipStackType != "ipv6single" {
			g.Skip("Current cluster doesn't have a proper object storage for this test!")
		}
		oc.CreateSpecifiedNamespaceAsAdmin(lokiStackNS)

		ls = &lokiStack{
			Name:          "lokistack",
			Namespace:     lokiStackNS,
			TSize:         "1x.demo",
			StorageType:   objectStorageType,
			StorageSecret: "objectstore-secret",
			StorageClass:  sc,
			BucketName:    "netobserv-loki-" + getInfrastructureName(oc),
			Tenant:        lokiTenant,
			Template:      lokiStackTemplate,
		}

		if ipStackType == "ipv6single" {
			e2e.Logf("running IPv6 test")
			ls.EnableIPV6 = "true"
		}

		err = ls.prepareResourcesForLokiStack(oc)
		if err != nil {
			g.Skip("Skipping test since LokiStack resources were not deployed")
		}

		err = ls.deployLokiStack(oc)
		if err != nil {
			g.Skip("Skipping test since LokiStack was not deployed")
		}

		lokiStackResource := Resource{"lokistack", ls.Name, ls.Namespace}
		err = lokiStackResource.WaitForResourceToAppear(oc)
		if err != nil {
			g.Skip("Skipping test since LokiStack did not become ready")
		}

		err = ls.waitForLokiStackToBeReady(oc)
		if err != nil {
			g.Skip("Skipping test since LokiStack is not ready")
		}
		ls.Route = "https://" + getRouteAddress(oc, ls.Namespace, ls.Name)
	})

	g.AfterEach(func() {
		ls.removeLokiStack(oc)
		ls.removeObjectStorage(oc)
		if !Lokiexisting {
			LO.uninstallOperator(oc)
		}
		oc.DeleteSpecifiedNamespaceAsAdmin(lokiStackNS)
	})

	g.It("Author:aramesha-Critical-86388-Verify flowCollectorSlice collectionMode: AlwaysCollect [Serial]", func() {
		// Test ping pods template variables
		pingPodsTemplate := filePath.Join(baseDir, "test-ping-pods_template.yaml")
		testPingPodsTemplate := TestPingPodsTemplate{
			ServerNS:    "test-ping-server-86388",
			ClientNS:    "test-ping-client-86388",
			PingTargets: "192.168.1.0 8.8.8.8",
			Template:    pingPodsTemplate,
		}

		subnetLabelsConfig := []map[string]interface{}{
			{
				"name": "external-api",
				"cidrs": []string{
					"8.8.8.8/32",
					"1.1.1.1/32",
				},
			},
			{
				"name": "internal-service",
				"cidrs": []string{
					"192.168.1.0/24",
				},
			},
		}

		config, err := json.Marshal(subnetLabelsConfig)
		o.Expect(err).ToNot(o.HaveOccurred())
		subnetLabels := string(config)

		g.By("Deploy FlowCollectorSlice")
		startTime := time.Now()
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testPingPodsTemplate.ClientNS)
		oc.CreateSpecifiedNamespaceAsAdmin(testPingPodsTemplate.ClientNS)
		flowSlice := FlowcollectorSlice{
			Name:         "subnet-label-slice",
			Namespace:    testPingPodsTemplate.ClientNS,
			SubnetLabels: subnetLabels,
			Template:     flowSliceFixturePath,
		}

		defer flowSlice.DeleteFlowcollectorSlice(oc)
		flowSlice.CreateFlowcollectorSlice(oc)

		g.By("Deploy FlowCollector with SlicesEnabled in AlwaysCollect mode")
		flow := Flowcollector{
			Namespace:      namespace,
			LokiNamespace:  lokiStackNS,
			CollectionMode: "AlwaysCollect",
			SlicesEnable:   "true",
			Template:       flowFixturePath,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)
		flowSlice.WaitForFlowcollectorSliceReady(oc)

		g.By("Deploy test ping server and client pods")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testPingPodsTemplate.ServerNS)
		err = testPingPodsTemplate.createPingPods(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertAllPodsToBeReady(oc, testPingPodsTemplate.ServerNS)
		exutil.AssertAllPodsToBeReady(oc, testPingPodsTemplate.ClientNS)

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		// Scenario1: Internal IP subnetLabel
		g.By("Verify flows with internal-service subnetLabel")
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testPingPodsTemplate.ClientNS,
		}
		parameters := []string{"DstAddr=\"192.168.1.0\""}

		flowRecords, err := lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows from client NS to internal-service > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.DstSubnetLabel).Should(o.ContainSubstring("internal-service"))
		}

		// Scenario2: External IP subnetLabel
		g.By("Verify flows with external-api subnetLabel")
		parameters = []string{"DstAddr=\"8.8.8.8\""}

		flowRecords, err = lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows from client NS to external-api > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.DstSubnetLabel).Should(o.ContainSubstring("external-api"))
		}

		// Scenario3: Flows are collected from namespaces without Slice deployed too
		g.By("Verify flows having no subnet label")
		lokilabels = Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testPingPodsTemplate.ServerNS,
		}

		flowRecords, err = lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows from server NS > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.DstSubnetLabel).Should(o.ContainSubstring("external-api"))
		}
	})

	g.It("Author:aramesha-Critical-86388-Verify flowCollectorSlice collectionMode: AllowList [Serial]", func() {
		// Test ping pods template variables
		pingPodsTemplate := filePath.Join(baseDir, "test-ping-pods_template.yaml")
		testPingPodsTemplate := TestPingPodsTemplate{
			ServerNS:    "test-ping-server-86388",
			ClientNS:    "test-ping-client-86388",
			PingTargets: "8.8.8.8",
			Template:    pingPodsTemplate,
		}

		g.By("Deploy FlowCollectorSlice")
		startTime := time.Now()
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testPingPodsTemplate.ClientNS)
		oc.CreateSpecifiedNamespaceAsAdmin(testPingPodsTemplate.ClientNS)
		flowSlice := FlowcollectorSlice{
			Name:      "namespace-slice",
			Namespace: testPingPodsTemplate.ClientNS,
			Sampling:  "3",
			Template:  flowSliceFixturePath,
		}

		defer flowSlice.DeleteFlowcollectorSlice(oc)
		flowSlice.CreateFlowcollectorSlice(oc)

		g.By("Deploy FlowCollector with Slices enabled in AllowList mode")
		flow := Flowcollector{
			Namespace:       namespace,
			LokiNamespace:   lokiStackNS,
			CollectionMode:  "AllowList",
			SlicesEnable:    "true",
			NamespacesAllow: []string{"\"/openshift-.*/\""},
			Template:        flowFixturePath,
		}

		defer flow.DeleteFlowcollector(oc)
		flow.CreateFlowcollector(oc)
		flowSlice.WaitForFlowcollectorSliceReady(oc)

		g.By("Deploy test ping server and client pods")
		defer oc.DeleteSpecifiedNamespaceAsAdmin(testPingPodsTemplate.ServerNS)
		err := testPingPodsTemplate.createPingPods(oc)
		o.Expect(err).NotTo(o.HaveOccurred())
		exutil.AssertAllPodsToBeReady(oc, testPingPodsTemplate.ServerNS)
		exutil.AssertAllPodsToBeReady(oc, testPingPodsTemplate.ClientNS)

		g.By("Wait for a min before logs gets collected and written to loki")
		time.Sleep(60 * time.Second)

		// Scenario1: Ping from namespace where flowCollectorSlice is deployed
		g.By("Verify flows from client NS")
		lokilabels := Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testPingPodsTemplate.ClientNS,
		}
		parameters := []string{"DstAddr=\"8.8.8.8\""}

		flowRecords, err := lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows from client NS > 0")
		for _, r := range flowRecords {
			o.Expect(r.Flowlog.Sampling).Should(o.BeNumerically("==", 3))
		}

		// Scenario2: Ping from namespace where flowCollectorSlice is NOT deployed
		g.By("Verify NO flows are seen from server NS")
		lokilabels = Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testPingPodsTemplate.ServerNS,
		}

		flowRecords, err = lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime, parameters...)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically("==", 0), "expected number of flows from server NS = 0")

		// Scenario3: Flows from namespace in allowedNamespaces section of flowcollector
		g.By("Verify flows are seen to openshift-dns")
		lokilabels = Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: "openshift-dns",
		}

		flowRecords, err = lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows from openshift-dns NS > 0")

		// Scenario4: Flows between namespaces with one in allowedNamespaces section should still be collected
		g.By("Verify flows between namespaces")
		startTime = time.Now()
		// Get server pod IP
		serverPodIP, _ := getPodIP(oc, testPingPodsTemplate.ServerNS, "ping-server", ipStackType)

		// Ping server pod from client pod
		e2eoutput.RunHostCmd(testPingPodsTemplate.ClientNS, "ping-client", "ping -c 30 "+serverPodIP)
		time.Sleep(60 * time.Second)

		lokilabels = Lokilabels{
			App:              "netobserv-flowcollector",
			SrcK8S_Namespace: testPingPodsTemplate.ClientNS,
			DstK8S_Namespace: testPingPodsTemplate.ServerNS,
		}

		flowRecords, err = lokilabels.getLokiFlowLogs(kubeadminToken, ls.Route, startTime)
		o.Expect(err).NotTo(o.HaveOccurred())
		o.Expect(len(flowRecords)).Should(o.BeNumerically(">", 0), "expected number of flows between test namespaces > 0")
	})
})
