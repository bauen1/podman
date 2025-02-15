package util

import (
	"net"

	"github.com/containers/podman/v3/libpod/network/types"
	"github.com/containers/podman/v3/libpod/network/util"
	"github.com/pkg/errors"
)

// ValidateSubnet will validate a given Subnet. It checks if the
// given gateway and lease range are part of this subnet. If the
// gateway is empty and addGateway is true it will get the first
// available ip in the subnet assigned.
func ValidateSubnet(s *types.Subnet, addGateway bool, usedNetworks []*net.IPNet) error {
	if s == nil {
		return errors.New("subnet is nil")
	}
	if s.Subnet.IP == nil {
		return errors.New("subnet ip is nil")
	}

	// Reparse to ensure subnet is valid.
	// Do not use types.ParseCIDR() because we want the ip to be
	// the network address and not a random ip in the subnet.
	_, net, err := net.ParseCIDR(s.Subnet.String())
	if err != nil {
		return errors.Wrap(err, "subnet invalid")
	}

	// check that the new subnet does not conflict with existing ones
	if NetworkIntersectsWithNetworks(net, usedNetworks) {
		return errors.Errorf("subnet %s is already used on the host or by another config", net.String())
	}

	s.Subnet = types.IPNet{IPNet: *net}
	if s.Gateway != nil {
		if !s.Subnet.Contains(s.Gateway) {
			return errors.Errorf("gateway %s not in subnet %s", s.Gateway, &s.Subnet)
		}
		util.NormalizeIP(&s.Gateway)
	} else if addGateway {
		ip, err := util.FirstIPInSubnet(net)
		if err != nil {
			return err
		}
		s.Gateway = ip
	}

	if s.LeaseRange != nil {
		if s.LeaseRange.StartIP != nil {
			if !s.Subnet.Contains(s.LeaseRange.StartIP) {
				return errors.Errorf("lease range start ip %s not in subnet %s", s.LeaseRange.StartIP, &s.Subnet)
			}
			util.NormalizeIP(&s.LeaseRange.StartIP)
		}
		if s.LeaseRange.EndIP != nil {
			if !s.Subnet.Contains(s.LeaseRange.EndIP) {
				return errors.Errorf("lease range end ip %s not in subnet %s", s.LeaseRange.EndIP, &s.Subnet)
			}
			util.NormalizeIP(&s.LeaseRange.EndIP)
		}
	}
	return nil
}

// ValidateSubnets will validate the subnets for this network.
// It also sets the gateway if the gateway is empty and it sets
// IPv6Enabled to true if at least one subnet is ipv6.
func ValidateSubnets(network *types.Network, usedNetworks []*net.IPNet) error {
	for i := range network.Subnets {
		err := ValidateSubnet(&network.Subnets[i], !network.Internal, usedNetworks)
		if err != nil {
			return err
		}
		if util.IsIPv6(network.Subnets[i].Subnet.IP) {
			network.IPv6Enabled = true
		}
	}
	return nil
}

func ValidateSetupOptions(n NetUtil, namespacePath string, options types.SetupOptions) error {
	if namespacePath == "" {
		return errors.New("namespacePath is empty")
	}
	if options.ContainerID == "" {
		return errors.New("ContainerID is empty")
	}
	if len(options.Networks) == 0 {
		return errors.New("must specify at least one network")
	}
	for name, netOpts := range options.Networks {
		network, err := n.Network(name)
		if err != nil {
			return err
		}
		err = validatePerNetworkOpts(network, netOpts)
		if err != nil {
			return err
		}
	}
	return nil
}

// validatePerNetworkOpts checks that all given static ips are in a subnet on this network
func validatePerNetworkOpts(network *types.Network, netOpts types.PerNetworkOptions) error {
	if netOpts.InterfaceName == "" {
		return errors.Errorf("interface name on network %s is empty", network.Name)
	}
outer:
	for _, ip := range netOpts.StaticIPs {
		for _, s := range network.Subnets {
			if s.Subnet.Contains(ip) {
				continue outer
			}
		}
		return errors.Errorf("requested static ip %s not in any subnet on network %s", ip.String(), network.Name)
	}
	return nil
}
