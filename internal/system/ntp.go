package system

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"go.uber.org/zap"

	"github.com/aws/eks-hybrid/internal/api"
	"github.com/aws/eks-hybrid/internal/validation"
)

var localhostReferenceIDs = []string{
	"(00000000)",
	"(0.0.0.0)",
	"(127.0.0.1)",
}

// NTPValidator validates NTP synchronization status
type NTPValidator struct {
	logger *zap.Logger
}

// NewNTPValidator creates a new NTP validator
func NewNTPValidator(logger *zap.Logger) *NTPValidator {
	return &NTPValidator{
		logger: logger,
	}
}

// Run validates NTP synchronization
func (v *NTPValidator) Run(ctx context.Context, informer validation.Informer, nodeConfig *api.NodeConfig) error {
	var err error
	informer.Starting(ctx, "ntp-sync", "Validating NTP synchronization status")
	defer func() {
		informer.Done(ctx, "ntp-sync", err)
	}()
	if err = v.Validate(); err != nil {
		return err
	}

	return nil
}

// Validate performs the actual NTP validation
func (v *NTPValidator) Validate() error {
	if v.commandExists("chronyc") {
		if chronySynchronized, err := v.checkChronyd(); err != nil {
			v.logger.Error("NTP synchronization validation via chronyd failed", zap.Error(err))
			if !chronySynchronized {
				return validation.WithRemediation(err,
					"Ensure the hybrid node is synchronized with NTP through chronyd services. "+
						"Verify NTP server configuration in /etc/chrony.conf. "+
						"If using airgapped networks, ensure chrony is configured with local NTP sources and adjusted manually.",
				)
			}
			return err
		}
	}

	if v.commandExists("timedatectl") {
		if systemdTimesyncSynchronized, err := v.checkSystemdTimesyncd(); err != nil {
			v.logger.Error("NTP synchronization validation via timedatectl failed", zap.Error(err))
			if !systemdTimesyncSynchronized {
				return validation.WithRemediation(err,
					"Ensure the hybrid node is synchronized with NTP through systemd-timesyncd services. "+
						"Verify NTP server configuration in /etc/systemd/timesyncd.conf. ",
				)
			}
			return err
		}
	}

	return nil
}

// checkChronyd checks if chronyd is synchronized
func (v *NTPValidator) checkChronyd() (bool, error) {
	cmd := exec.Command("chronyc", "tracking")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, fmt.Errorf("failed to get chrony tracking status: %s, error: %w", output, err)
	}

	// Parse the output to check if synchronized
	lines := strings.Split(string(output), "\n")
	hasReference := false
	leapStatusNormal := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for reference ID (indicates time source)
		if strings.Contains(line, "Reference ID") {
			parts := strings.Fields(line)
			if len(parts) >= 5 && !slices.Contains(localhostReferenceIDs, parts[4]) {
				hasReference = true
			}
		}

		if strings.Contains(line, "Leap status") && strings.Contains(line, "Normal") {
			leapStatusNormal = true
		}
	}

	if !hasReference || !leapStatusNormal {
		return false, fmt.Errorf("chronyd not synchronized")
	}

	return true, nil
}

// checkSystemdTimesyncd checks if systemd-timesyncd is synchronized
func (v *NTPValidator) checkSystemdTimesyncd() (bool, error) {
	cmd := exec.Command("timedatectl", "status")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return true, fmt.Errorf("failed to get timedatectl status: %s, error: %w", output, err)
	}

	lines := strings.Split(string(output), "\n")
	ntpSynchronized := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "System clock synchronized: yes") {
			ntpSynchronized = true
		}
	}

	if !ntpSynchronized {
		return false, fmt.Errorf("systemd-timesyncd not synchronized")
	}

	return true, nil
}

func (v *NTPValidator) commandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}
