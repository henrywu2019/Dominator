package main

import (
	"fmt"
	"os"
	"path/filepath"

	imageclient "github.com/Symantec/Dominator/imageserver/client"
	"github.com/Symantec/Dominator/lib/errors"
	"github.com/Symantec/Dominator/lib/log"
	"github.com/Symantec/Dominator/lib/srpc"
	hyper_proto "github.com/Symantec/Dominator/proto/hypervisor"
)

func writeNetbootFilesSubcommand(args []string, logger log.DebugLogger) {
	err := writeNetbootFiles(args[0], args[1], logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing netboot files: %s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func emptyTree(rootDir string) error {
	dir, err := os.Open(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names, err := dir.Readdirnames(-1)
	dir.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := os.Remove(filepath.Join(rootDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func writeNetbootFiles(hostname, dirname string, logger log.DebugLogger) error {
	fmCR := srpc.NewClientResource("tcp",
		fmt.Sprintf("%s:%d", *fleetManagerHostname, *fleetManagerPortNum))
	defer fmCR.ScheduleClose()
	info, err := getInfoForMachine(fmCR, hostname)
	if err != nil {
		return err
	}
	imageName := info.Machine.Tags["RequiredImage"]
	subnets := make([]*hyper_proto.Subnet, 0, len(info.Subnets))
	for _, subnet := range info.Subnets {
		if subnet.VlanId == 0 {
			subnets = append(subnets, subnet)
		}
	}
	if len(subnets) < 1 {
		return errors.New("no non-VLAN subnets known")
	}
	networkEntries := getNetworkEntries(info)
	hostAddresses := getHostAddress(networkEntries)
	if len(hostAddresses) < 1 {
		return errors.New("no IP and MAC addresses known for host")
	}
	if imageName != "" {
		imageClient, err := srpc.DialHTTP("tcp", fmt.Sprintf("%s:%d",
			*imageServerHostname, *imageServerPortNum), 0)
		if err != nil {
			return err
		}
		defer imageClient.Close()
		img, err := imageclient.GetImage(imageClient, imageName)
		if err != nil {
			return err
		}
		if img == nil {
			return fmt.Errorf("image: %s does not exist", imageName)
		}
	}
	configFiles, err := makeConfigFiles(info, imageName, networkEntries)
	if err != nil {
		return err
	}
	if err := emptyTree(dirname); err != nil {
		return err
	}
	return writeConfigFiles(dirname, configFiles)
}
