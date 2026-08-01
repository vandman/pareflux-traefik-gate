// SPDX-License-Identifier: Apache-2.0

// /////////////////////////////////////////////////////////////////////////////
// PareFlux derived source file
// FR: Fichier dérivé du plugin CrowdSec Traefik v1.6.0 et adapté à PareFlux
//
//	sous Apache-2.0 sans dépendance GPL-3.0.
//
// EN: File derived from CrowdSec Traefik Plugin v1.6.0 and adapted for
//
//	PareFlux under Apache-2.0 without a GPL-3.0 dependency.
//
// Called by: PareFlux Gate runtime or tests
// Calls: package-specific collaborators
// /////////////////////////////////////////////////////////////////////////////
// Package ip implements utility routines to manipulate IP and CIDR.
// It allows searching an IP on a list, and find if an IP is part of a list of CIDR.
package ip

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// CHECKER

// Checker allows to check that addresses are in a trusted IPs.
type Checker struct {
	authorizedIPs    []*net.IP
	authorizedIPsNet []*net.IPNet
}

// NewChecker builds a new Checker given a list of CIDR-Strings to trusted IPs.
func NewChecker(log *slog.Logger, trustedIPs []string) (*Checker, error) {
	checker := &Checker{}

	for _, ipMaskRaw := range trustedIPs {
		ipMask := strings.TrimSpace(ipMaskRaw)
		if ipAddr := net.ParseIP(ipMask); ipAddr != nil {
			checker.authorizedIPs = append(checker.authorizedIPs, &ipAddr)
			log.Debug(fmt.Sprintf("IP %v is trusted", ipAddr))
			continue
		}

		_, ipAddr, err := net.ParseCIDR(ipMask)
		if err != nil {
			return nil, fmt.Errorf("parsing CIDR trusted IPs %s: %w", ipAddr, err)
		}
		checker.authorizedIPsNet = append(checker.authorizedIPsNet, ipAddr)
		log.Debug(fmt.Sprintf("IP network %v is trusted", ipAddr))
	}

	return checker, nil
}

// Contains checks if provided address is in the trusted IPs.
func (ip *Checker) Contains(addr string) (bool, error) {
	if len(addr) == 0 {
		return false, errors.New("Contains:noAddress")
	}

	ipAddr, err := parseIP(addr)
	if err != nil {
		return false, fmt.Errorf("Contains:parseAddress addr:%s %w", addr, err)
	}

	return ip.ContainsIP(ipAddr), nil
}

// ContainsIP checks if provided address is in the trusted IPs.
func (ip *Checker) ContainsIP(addr net.IP) bool {
	for _, authorizedIP := range ip.authorizedIPs {
		if authorizedIP.Equal(addr) {
			return true
		}
	}

	for _, authorizedNet := range ip.authorizedIPsNet {
		if authorizedNet.Contains(addr) {
			return true
		}
	}

	return false
}

// Empty reports whether no trusted proxy address was configured.
// FR: Empêche qu'une liste vide transforme implicitement un en-tête transmis
// en identité réseau fiable.
// EN: Prevents an empty list from implicitly turning a forwarded header into
// a trusted network identity.
// Called by: PoolStrategy.getIP
// Calls: none
func (ip *Checker) Empty() bool {
	return ip == nil || (len(ip.authorizedIPs) == 0 && len(ip.authorizedIPsNet) == 0)
}

func parseIP(addr string) (net.IP, error) {
	userIP := net.ParseIP(addr)
	if userIP == nil {
		return nil, fmt.Errorf("parseIP:parseAddress %s", addr)
	}

	return userIP, nil
}

// STRATEGY

// PoolStrategy is a strategy based on an IP Checker.
// It allows to check whether addresses are in a given pool of IPs.
type PoolStrategy struct {
	Checker *Checker
}

// GetIP checks the list of Forwarded IPs (most recent first) against the
// Checker pool of IPs. It returns the first IP that is not in the pool, or the
// empty string otherwise.
func (s *PoolStrategy) getIP(req *http.Request, customHeader string) (string, error) {
	if s == nil || s.Checker.Empty() || customHeader == "" {
		return "", nil
	}

	peer, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return "", fmt.Errorf("getIP:extractPeer: %w", err)
	}
	peerTrusted, err := s.Checker.Contains(peer)
	if err != nil {
		return "", fmt.Errorf("getIP:parsePeer: %w", err)
	}
	if !peerTrusted {
		return "", nil
	}

	xff := req.Header.Get(customHeader)

	xffs := strings.Split(xff, ",")

	for i := len(xffs) - 1; i >= 0; i-- {
		xffTrimmed := strings.TrimSpace(xffs[i])
		if len(xffTrimmed) == 0 {
			continue
		}
		candidate, err := parseIP(xffTrimmed)
		if err != nil {
			return "", fmt.Errorf("getIP:invalidForwardedAddress: %w", err)
		}
		if !s.Checker.ContainsIP(candidate) {
			return candidate.String(), nil
		}
	}

	return "", nil
}

// GetRemoteIP It returns the first IP that is not in the pool, or the empty string otherwise.
func GetRemoteIP(req *http.Request, strategy *PoolStrategy, customHeader string) (string, error) {
	remoteIP, err := strategy.getIP(req, customHeader)
	if err != nil {
		return "", err
	}
	if len(remoteIP) != 0 {
		return remoteIP, nil
	}
	remoteIP, _, err = net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return "", fmt.Errorf("GetRemoteIP:extractIP: %w", err)
	}
	parsed, err := parseIP(remoteIP)
	if err != nil {
		return "", fmt.Errorf("GetRemoteIP:parsePeer: %w", err)
	}
	return parsed.String(), nil
}
