package system

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/aws/eks-hybrid/internal/api"
	"github.com/aws/eks-hybrid/internal/validation"
)

// SwapValidator validates swap configuration before nodeadm init
type SwapValidator struct {
	logger *zap.Logger
}

// NewSwapValidator creates a new SwapValidator
func NewSwapValidator(logger *zap.Logger) *SwapValidator {
	return &SwapValidator{
		logger: logger,
	}
}

// Run validates the swap configuration
func (v *SwapValidator) Run(ctx context.Context, informer validation.Informer, nodeConfig *api.NodeConfig) error {
	informer.Starting(ctx, "swap", "Checking swap configuration...")

	err := v.Validate()
	informer.Done(ctx, "swap", err)
	return err
}

// Validate performs the swap validation
func (v *SwapValidator) Validate() error {
	swapfiles, err := getSwapfilePaths()
	if err != nil {
		v.logger.Error("Failed to get swap file paths", zap.Error(err))
		return fmt.Errorf("unable to read swap configuration: %w", err)
	}

	// Check for partition-type swap that would cause init to fail
	hasPartitionSwap, err := partitionSwapExists(swapfiles)
	if err != nil {
		v.logger.Error("Failed to check swap configuration", zap.Error(err))
		return fmt.Errorf("unable to check swap configuration: %w", err)
	}

	if hasPartitionSwap {
		err := fmt.Errorf("partition swap detected on host")
		v.logger.Error("Swap validation failed", zap.Error(err))
		return validation.WithRemediation(err,
			"Nodeadm can only disable file-based swap automatically. Please manually disable partition swap before running nodeadm init. "+
				"Run 'sudo swapoff -a' to disable swap and remove swap entries from /etc/fstab to make the change persistent.")
	}

	// Check for any remaining swap (both partition and file types)
	if len(swapfiles) > 0 {
		swapTypes := make([]string, 0, len(swapfiles))
		swapPaths := make([]string, 0, len(swapfiles))

		for _, swap := range swapfiles {
			swapTypes = append(swapTypes, swap.swapType)
			swapPaths = append(swapPaths, swap.filePath)
		}

		err := fmt.Errorf("swap still active on host: %d swap entries found", len(swapfiles))
		v.logger.Error("Swap validation failed", zap.Error(err), zap.Strings("swap_paths", swapPaths), zap.Strings("swap_types", swapTypes))

		if len(swapfiles) == 1 && swapfiles[0].swapType == swapTypeFile {
			return validation.WithRemediation(err,
				"File-based swap should have been disabled during nodeadm init. This may indicate init did not complete successfully. "+
					"Try running 'sudo swapoff "+swapfiles[0].filePath+"' and re-run nodeadm init if needed.")
		} else {
			return validation.WithRemediation(err,
				"All swap should be disabled before running debug. For partition swap, run 'sudo swapoff -a' and remove entries from /etc/fstab. "+
					"For file swap, this indicates nodeadm init may not have completed successfully.")
		}
	}

	v.logger.Info("No swap detected")
	return nil
}
