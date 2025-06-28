package os

import (
	"context"

	"github.com/aws/eks-hybrid/test/e2e/commands"
)

type NodeLogCollector interface {
	Run(ctx context.Context, instanceIP, logBundleUrl string) error
}

type StandardLinuxLogCollector struct {
	Runner commands.RemoteCommandRunner
}

type BottlerocketLogCollector struct {
	Runner commands.RemoteCommandRunner
}

func (s StandardLinuxLogCollector) Run(ctx context.Context, instanceIP, logBundleUrl string) error {
	commands := []string{
		"/tmp/log-collector.sh '" + logBundleUrl + "'",
	}

	output, err := s.Runner.Run(ctx, instanceIP, commands)
	if err != nil {
		return err
	}

	if output.Status != "Success" {
		return err
	}

	return nil
}

func (b BottlerocketLogCollector) Run(ctx context.Context, instanceIP, logBundleUrl string) error {
	commands := []string{
		"sudo /usr/sbin/chroot /.bottlerocket/rootfs/ logdog --output /var/log/eks-hybrid-logs.tar.gz",
		"sudo curl --retry 5 --request PUT --upload-file /.bottlerocket/rootfs/var/log/eks-hybrid-logs.tar.gz '" + logBundleUrl + "'",
		"sudo rm /.bottlerocket/rootfs/var/log/eks-hybrid-logs.tar.gz",
	}

	output, err := b.Runner.Run(ctx, instanceIP, commands)
	if err != nil {
		return err
	}

	if output.Status != "Success" {
		return err
	}

	return nil
}
