// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import "github.com/ethereum/go-ethereum/common"

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Nero network.
var MainnetBootnodes = []string{}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the Testnet
var TestnetBootnodes = []string{
	"enode://4cc86482934f5bdc0cd8f9c4c3a5c02a668846cf19f24fb2a729509738ab1c5c06080fdd46d8ac470a55c9d2c54a4091fbab47aa9913ca79a46c7f1da7e037e7@176.34.25.237:30306",
	"enode://d7c128bf692a8af4379eeaaff1a5513006964eb99c3ead0318727e0cc6abf86d44a168d8ea807fd6bfde9e04935e21eced2b7865fbf58b61ac3bb6b1f80ca437@18.177.99.157:30306",
}

var V5Bootnodes = []string{}

// KnownDNSNetwork returns the address of a public DNS-based node list for the given
// genesis hash and protocol. See https://github.com/ethereum/discv4-dns-lists for more
// information.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	return ""
}
