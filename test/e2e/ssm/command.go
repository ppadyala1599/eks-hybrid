package ssm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	smithytime "github.com/aws/smithy-go/time"
	"github.com/go-logr/logr"

	e2eCommands "github.com/aws/eks-hybrid/test/e2e/commands"
)

const (
	commandExecTimeout           = 10 * time.Minute
	commandWaitTimeout           = commandExecTimeout + time.Minute
	instanceRegisterTimeout      = 5 * time.Minute
	instanceRegisterSleepTimeout = 15 * time.Second
	standardLinuxSSHUser         = "root"
	bottlerocketSSHUser          = "ec2-user"
)

type StandardLinuxSSHOnSSM struct {
	client            *ssm.Client
	jumpboxInstanceId string
	logger            logr.Logger
}

type BottlerocketSSHOnSSM struct {
	client            *ssm.Client
	jumpboxInstanceId string
	logger            logr.Logger
}

func NewStandardLinuxSSHOnSSMCommandRunner(client *ssm.Client, jumpboxInstanceId string, logger logr.Logger) e2eCommands.RemoteCommandRunner {
	return &StandardLinuxSSHOnSSM{
		client:            client,
		jumpboxInstanceId: jumpboxInstanceId,
		logger:            logger,
	}
}

func NewBottlerocketSSHOnSSMCommandRunner(client *ssm.Client, jumpboxInstanceId string, logger logr.Logger) e2eCommands.RemoteCommandRunner {
	return &BottlerocketSSHOnSSM{
		client:            client,
		jumpboxInstanceId: jumpboxInstanceId,
		logger:            logger,
	}
}

func (s *StandardLinuxSSHOnSSM) Run(ctx context.Context, ip string, commands []string) (e2eCommands.RemoteCommandOutput, error) {
	sshCommand := fmt.Sprintf("ssh %s@%s \"%s\"", standardLinuxSSHUser, ip, strings.ReplaceAll(strings.Join(commands, ";"), "\"", "\\\""))
	return RunCommand(ctx, s.client, s.jumpboxInstanceId, sshCommand, s.logger)
}

func (s *BottlerocketSSHOnSSM) Run(ctx context.Context, ip string, commands []string) (e2eCommands.RemoteCommandOutput, error) {
	sshCommand := fmt.Sprintf("ssh %s@%s \"%s\"", bottlerocketSSHUser, ip, strings.ReplaceAll(strings.Join(commands, ";"), "\"", "\\\""))
	return RunCommand(ctx, s.client, s.jumpboxInstanceId, sshCommand, s.logger)
}

func RunCommand(ctx context.Context, client *ssm.Client, instanceId, command string, logger logr.Logger) (e2eCommands.RemoteCommandOutput, error) {
	sanitizedCommand := sanitizeS3PresignedURL(command)
	logger.Info(fmt.Sprintf("Running command: %v\n", sanitizedCommand))
	input := &ssm.SendCommandInput{
		DocumentName: aws.String("AWS-RunShellScript"),
		Parameters: map[string][]string{
			"commands":         {command},
			"executionTimeout": {fmt.Sprintf("%g", commandExecTimeout.Seconds())},
		},
		InstanceIds: []string{instanceId},
	}
	// when running e2e tests in the CI account, we occasionally see throttling errors from
	// the SSM SendCommand operation which exhaust our standard 40 attempts
	// this usually happens when we are running multiple pipelines in parallel
	// we increase the max attempts to 120 and set the max backoff to 40 seconds
	// to give the operation more time to complete
	optsFn := func(opts *ssm.Options) {
		opts.Retryer = retry.NewAdaptiveMode(func(o *retry.AdaptiveModeOptions) {
			o.StandardOptions = []func(*retry.StandardOptions){
				func(o *retry.StandardOptions) {
					o.MaxAttempts = 120
					o.MaxBackoff = 40 * time.Second
				},
			}
		})
	}
	output, err := client.SendCommand(ctx, input, optsFn)
	if err != nil {
		return e2eCommands.RemoteCommandOutput{}, fmt.Errorf("got an error calling SendCommand: %w", err)
	}
	invocationInput := &ssm.GetCommandInvocationInput{
		CommandId:  output.Command.CommandId,
		InstanceId: aws.String(instanceId),
	}
	waiter := ssm.NewCommandExecutedWaiter(client)
	// Will wait on Pending, InProgress, or Cancelling until we reach a terminal status of Success, Cancelled, Failed, TimedOut
	_ = waiter.Wait(ctx, invocationInput, commandWaitTimeout)
	invocationOutput, err := client.GetCommandInvocation(ctx, invocationInput, optsFn)
	if err != nil {
		return e2eCommands.RemoteCommandOutput{}, fmt.Errorf("got an error calling GetCommandInvocation: %w", err)
	}

	commandOutput := e2eCommands.RemoteCommandOutput{
		ResponseCode:   invocationOutput.ResponseCode,
		StandardError:  *invocationOutput.StandardErrorContent,
		StandardOutput: *invocationOutput.StandardOutputContent,
		Status:         string(invocationOutput.Status),
	}

	logger.Info(fmt.Sprintf("Status: %s", commandOutput.Status))
	logger.Info(fmt.Sprintf("ResponseCode: %d", commandOutput.ResponseCode))
	logger.Info(fmt.Sprintf("Stdout: %s", commandOutput.StandardOutput))
	logger.Info(fmt.Sprintf("Stderr: %s", commandOutput.StandardError))

	return commandOutput, nil
}

// sanitizeS3PresignedURL strips off the S3 presigned URL query parameters to avoid exposing sensitive information in logs
func sanitizeS3PresignedURL(command string) string {
	if !strings.Contains(command, "log-collector.sh") {
		return command
	}

	// Find the position of the query string in the URL
	questionMarkPos := strings.Index(command, "?")
	if questionMarkPos == -1 {
		return command
	}

	// if there is a space after the s3 presigned url there are additional arguments, we need to include them in the sanitized command
	var afterS3PresignedURL string
	spaceAfterPos := strings.Index(command[questionMarkPos:], " ")
	if spaceAfterPos != -1 {
		afterS3PresignedURL = command[questionMarkPos+spaceAfterPos:]
	}
	return command[:questionMarkPos] + "?[REDACTED]'" + afterS3PresignedURL
}

// WaitForInstance uses DescribeInstanceInformation in a loop to wait for it be registered
// There is no built in wait for instance to be registered with ssm
// see: https://github.com/aws/aws-cli/issues/4006
func WaitForInstance(ctx context.Context, client *ssm.Client, instanceId string, logger logr.Logger) error {
	waitCtx, cancel := context.WithTimeout(ctx, instanceRegisterTimeout)
	defer cancel()

	logger.Info("Waiting for instance to be registered with SSM", "instanceID", instanceId)
	for {
		output, err := client.DescribeInstanceInformation(waitCtx, &ssm.DescribeInstanceInformationInput{
			Filters: []types.InstanceInformationStringFilter{
				{
					Key:    aws.String("InstanceIds"),
					Values: []string{instanceId},
				},
			},
		})
		if err != nil {
			return err
		}
		if len(output.InstanceInformationList) > 0 {
			logger.Info("Instance is registered with SSM", "instanceID", instanceId)
			return nil
		}
		logger.Info("Instance still not registered with SSM, retrying", "instanceID", instanceId)
		if err := smithytime.SleepWithContext(waitCtx, instanceRegisterSleepTimeout); err != nil {
			return fmt.Errorf("request cancelled while waiting, %w", err)
		}
	}
}
