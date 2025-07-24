package system

import (
	"context"
	"fmt"

	"github.com/aws/eks-hybrid/internal/api"
	"github.com/aws/eks-hybrid/internal/validation"
)

const (
	noFileRecommendedLimit uint64 = 1024
	nProcRecommendedLimit  uint64 = 4096
)

// UlimitValidator validates ulimit configuration before nodeadm init
type UlimitValidator struct{}

// NewUlimitValidator creates a new UlimitValidator
func NewUlimitValidator() *UlimitValidator {
	return &UlimitValidator{}
}

// Run validates the ulimit configuration
func (v *UlimitValidator) Run(ctx context.Context, informer validation.Informer, nodeConfig *api.NodeConfig) error {
	var err error
	informer.Starting(ctx, "ulimit", "Validating ulimit configuration")
	defer func() {
		informer.Done(ctx, "ulimit", err)
	}()
	if err = v.Validate(); err != nil {
		return err
	}

	return nil
}

// Validate performs the ulimit validation
func (v *UlimitValidator) Validate() error {
	noFileLimit, nProcLimit, err := getUlimits()
	if err != nil {
		return fmt.Errorf("unable to read ulimit configuration: %w", err)
	}

	issues := v.checkCriticalLimits(noFileLimit, nProcLimit)
	if len(issues) > 0 {
		err := fmt.Errorf("ulimit configuration issues detected: %d issues found", len(issues))
		remediation := "Consider adjusting the following ulimit values for optimal Kubernetes node operation:\n"
		for _, issue := range issues {
			remediation += "  - " + issue + "\n"
		}

		return validation.WithRemediation(err, remediation)
	}

	return nil
}

// checkCriticalLimits checks for ulimit values that could impact Kubernetes operation
func (v *UlimitValidator) checkCriticalLimits(noFileLimit, nProcLimit uint64) []string {
	var issues []string

	if noFileLimit < noFileRecommendedLimit {
		issues = append(issues, fmt.Sprintf("max open file descriptors limit is %d, which is lower than the recommended value of %d", noFileLimit, noFileRecommendedLimit))
	}

	if nProcLimit < nProcRecommendedLimit {
		issues = append(issues, fmt.Sprintf("max processes limit is %d, which is lower than the recommended value of %d", nProcLimit, nProcRecommendedLimit))
	}

	return issues
}
