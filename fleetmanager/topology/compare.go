package topology

import (
	"github.com/Symantec/Dominator/proto/hypervisor"
)

func (left *Topology) equal(right *Topology) bool {
	if left == nil || right == nil {
		return false
	}
	if len(left.machineParents) != len(right.machineParents) {
		return false
	}
	return left.Root.equal(right.Root)
}

func (left *Directory) equal(right *Directory) bool {
	if left.Name != right.Name {
		return false
	}
	if len(left.Directories) != len(right.Directories) {
		return false
	}
	if len(left.Machines) != len(right.Machines) {
		return false
	}
	if len(left.Subnets) != len(right.Subnets) {
		return false
	}
	for index, leftSubdir := range left.Directories {
		if !leftSubdir.equal(right.Directories[index]) {
			return false
		}
	}
	for index, leftMachine := range left.Machines {
		if !leftMachine.Equal(right.Machines[index]) {
			return false
		}
	}
	for index, leftSubnet := range left.Subnets {
		if !leftSubnet.equal(right.Subnets[index]) {
			return false
		}
	}
	return true
}

func (left *Subnet) equal(right *Subnet) bool {
	if !left.Subnet.Equal(&right.Subnet) {
		return false
	}
	return hypervisor.IpListsEqual(left.ReservedIPs, right.ReservedIPs)
}