package suite

import (
	"context"

	ec2v2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aws/eks-hybrid/test/e2e/commands"
	"github.com/aws/eks-hybrid/test/e2e/constants"
	"github.com/aws/eks-hybrid/test/e2e/ec2"
	"github.com/aws/eks-hybrid/test/e2e/kubernetes"
	"github.com/aws/eks-hybrid/test/e2e/nodeadm"
)

type NodeWaiter interface {
	Run(ctx context.Context, flakeRun FlakeRun)
}

type StandardLinuxNodeWaiter struct {
	ec2Client           *ec2v2.Client
	instanceID          string
	instanceIP          string
	verifyNode          *kubernetes.VerifyNode
	remoteCommandRunner commands.RemoteCommandRunner
	logger              logr.Logger
}

type BottlerocketNodeWaiter struct {
	ec2Client           *ec2v2.Client
	instanceID          string
	instanceIP          string
	verifyNode          *kubernetes.VerifyNode
	remoteCommandRunner commands.RemoteCommandRunner
	logger              logr.Logger
}

func (n *testNode) NewStandardLinuxNodeWaiter() *StandardLinuxNodeWaiter {
	return &StandardLinuxNodeWaiter{
		ec2Client:           n.EC2Client,
		instanceID:          n.peeredInstance.ID,
		instanceIP:          n.peeredInstance.IP,
		verifyNode:          n.verifyNode,
		remoteCommandRunner: n.PeeredNode.RemoteCommandRunner,
		logger:              n.Logger,
	}
}

func (n *testNode) NewBottlerocketNodeWaiter() *BottlerocketNodeWaiter {
	return &BottlerocketNodeWaiter{
		ec2Client:           n.EC2Client,
		instanceID:          n.peeredInstance.ID,
		instanceIP:          n.peeredInstance.IP,
		verifyNode:          n.verifyNode,
		remoteCommandRunner: n.PeeredNode.RemoteCommandRunner,
		logger:              n.Logger,
	}
}

func (w *StandardLinuxNodeWaiter) Run(ctx context.Context, flakeRun FlakeRun) {
	w.logger.Info("Waiting for EC2 Instance to be Running...")
	flakeRun.RetryableExpect(ec2.WaitForEC2InstanceRunning(ctx, w.ec2Client, w.instanceID)).To(Succeed(), "EC2 Instance should have been reached Running status")
	_, err := w.verifyNode.WaitForNodeReady(ctx)

	// if the node is impaired, we want to trigger a retryable expect
	// if the node is not impaired, we run nodeadm debug regardless of whether the node joined the cluster successfully
	// if the node joined successfully and debug fails, the test will fail
	expect := flakeRun.RetryableExpect
	isImpaired := isImpaired(ctx, err, w.ec2Client, w.instanceID, w.logger)
	var debugErr error
	if !isImpaired {
		expect = Expect
		debugErr = nodeadm.RunNodeadmDebug(ctx, w.remoteCommandRunner, w.instanceIP)
	}

	// attempt to get the nodeadm version regardless of previous errors
	version, versionErr := nodeadm.RunNodeadmVersion(ctx, w.remoteCommandRunner, w.instanceIP)
	if versionErr == nil && version != "" {
		AddReportEntry(constants.TestNodeadmVersion, version)
	}

	expect(err).To(Succeed(), "node should have joined the cluster successfully")
	Expect(debugErr).NotTo(HaveOccurred(), "nodeadm debug should have been run successfully")
	Expect(versionErr).NotTo(HaveOccurred(), "nodeadm version should have been retrieved successfully")
	Expect(version).NotTo(BeEmpty(), "nodeadm version should not be empty")
}

func (w *BottlerocketNodeWaiter) Run(ctx context.Context, flakeRun FlakeRun) {
	w.logger.Info("Waiting for EC2 Instance to be Running...")
	flakeRun.RetryableExpect(ec2.WaitForEC2InstanceRunning(ctx, w.ec2Client, w.instanceID)).To(Succeed(), "EC2 Instance should have been reached Running status")
	_, err := w.verifyNode.WaitForNodeReady(ctx)

	// if the node is impaired, we want to trigger a retryable expect
	// if the node is not impaired, we run nodeadm debug regardless of whether the node joined the cluster successfully
	// if the node joined successfully and debug fails, the test will fail
	expect := flakeRun.RetryableExpect
	isImpaired := isImpaired(ctx, err, w.ec2Client, w.instanceID, w.logger)
	if !isImpaired {
		expect = Expect
	}

	expect(err).To(Succeed(), "node should have joined the cluster successfully")

	AddReportEntry(constants.TestNodeadmVersion, "N/A")
}

func isImpaired(ctx context.Context, waitErr error, ec2Client *ec2v2.Client, instanceID string, logger logr.Logger) bool {
	if waitErr == nil {
		return false
	}
	isImpaired, err := ec2.IsEC2InstanceImpaired(ctx, ec2Client, instanceID)
	if err != nil {
		logger.Error(err, "describing instance status")
	}
	return isImpaired
}
