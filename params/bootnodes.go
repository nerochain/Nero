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
var MainnetBootnodes = []string{
	"enode://7cf89853f348831e84b48dd81d3242cb2c410bd94d9f5ed15c4a3b22b60790317ffaedbbddf86ab58b215dec12fc315327d60f51f6fc5c9698815bf41f196251@34.85.119.231:30306",
	"enode://7317318d3bffaf9b5fc0b413a06987ed497efa349484a1bd10bb80aa96ecf7a29b510e486bc968187339229cae3757abd64ef42a67f51fb4b72571b6b8aab3f8@34.146.179.136:30306",
	"enode://2340318298e056141221ef47b45ecdfdb9d92deb32e9777a937ab8694bc37539f31acdde47184ab95a0e57ef99e3a00a422d79e0e6dea13d913c469b477c8166@34.146.62.154:30306",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the Testnet
var TestnetBootnodes = []string{
	"enode://fc92bd94648fe76f94eefbd7fc39ce697ccf59b9c8b4adfc8a84e97a355bab4d26de560ba5e55b5b722f0ff1e201bdf493b3083b7e9aa2f8721d965a1a1cf594@34.146.54.11:30306",
	"enode://c886b20b3ec5e43c6a701999221230f80ae0e90f0cec208146fd7f8a6a1cb4e20800b980ea322e20378fed73563d08f9ff182a1583257bd3230bb08c72020899@34.146.242.44:30306",
}

var V5Bootnodes = []string{}

// KnownDNSNetwork returns the address of a public DNS-based node list for the given
// genesis hash and protocol. See https://github.com/ethereum/discv4-dns-lists for more
// information.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	return ""
}
