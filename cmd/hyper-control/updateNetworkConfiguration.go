package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/Symantec/Dominator/lib/log"
)

func updateNetworkConfigurationSubcommand(args []string,
	logger log.DebugLogger) error {
	err := updateNetworkConfiguration(logger)
	if err != nil {
		return fmt.Errorf("Error updating network configuration: %s", err)
	}
	return nil
}

func updateNetworkConfiguration(logger log.DebugLogger) error {
	netconf, err := getNetworkConfiguration(logger)
	if err != nil {
		return err
	}
	if changed, err := netconf.Update("/", logger); err != nil {
		return err
	} else if !changed {
		return nil
	}
	logger.Println("restarting hypervisor")
	cmd := exec.Command("service", "hypervisor", "restart")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
