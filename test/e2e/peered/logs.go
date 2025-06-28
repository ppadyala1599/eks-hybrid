package peered

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/go-logr/logr"

	"github.com/aws/eks-hybrid/test/e2e/constants"
	e2eS3 "github.com/aws/eks-hybrid/test/e2e/s3"
	e2eSSM "github.com/aws/eks-hybrid/test/e2e/ssm"
)

// JumpboxLogCollection holds the configuration for jumpbox log collection
type JumpboxLogCollection struct {
	JumpboxInstanceID string
	LogsBucket        string
	ClusterName       string
	S3Client          *s3.Client
	SSMClient         *ssm.Client
	Logger            logr.Logger
}

// CollectJumpboxLogs collects logs from the jumpbox instance and uploads them to S3
func CollectJumpboxLogs(ctx context.Context, config JumpboxLogCollection) error {
	if config.LogsBucket == "" {
		return nil
	}

	s3Key := generateJumpboxLogsS3Key(config.ClusterName, constants.JumpboxLogBundleFileName)

	url, err := e2eS3.GeneratePutLogsPreSignedURL(ctx, config.S3Client, config.LogsBucket, s3Key, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("generating pre-signed URL for jumpbox logs: %w", err)
	}

	err = executeLogCollectorScript(ctx, config.SSMClient, config.JumpboxInstanceID, url, config.Logger)
	if err != nil {
		return fmt.Errorf("executing log collector script on jumpbox: %w", err)
	}
	return nil
}

// generateJumpboxLogsS3Key generates the S3 key for the jumpbox logs
func generateJumpboxLogsS3Key(clusterName, bundleName string) string {
	return fmt.Sprintf("%s/%s/%s", constants.TestS3LogsFolder, clusterName, bundleName)
}

// executeLogCollectorScript executes the log collector script on the jumpbox itself
func executeLogCollectorScript(ctx context.Context, ssmClient *ssm.Client, instanceID, url string, logger logr.Logger) error {
	additionalLogs := []string{
		"/var/log/amazon/ssm/errors.log",
		"/var/log/amazon/ssm/amazon-ssm-agent.log",
	}
	command := fmt.Sprintf("/tmp/log-collector.sh '%s' %s", url, strings.Join(additionalLogs, " "))

	output, err := e2eSSM.RunCommand(ctx, ssmClient, instanceID, command, logger)
	if err != nil {
		return fmt.Errorf("executing log collector script: %w", err)
	}

	if output.Status != "Success" {
		return fmt.Errorf("log collector script execution failed, status: %s", output.Status)
	}

	return nil
}
