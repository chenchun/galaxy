/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
package floatingip

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"tkestack.io/galaxy/pkg/utils/nets"
)

// FloatingIP defines a floating ip
type FloatingIP struct {
	Key       string
	Subnet    string // node subnet, not container ip's subnet
	Attr      string
	IP        net.IP
	Policy    uint16
	UpdatedAt time.Time
}

// FloatingIPPool is FloatingIPPool structure.
type FloatingIPPool struct {
	RoutableSubnet *net.IPNet // the node subnet
	nets.SparseSubnet
	sync.RWMutex
}

// FloatingIPPoolConf is FloatingIP config structure.
type FloatingIPPoolConf struct {
	RoutableSubnet *nets.IPNet `json:"routableSubnet"` // the node subnet
	IPs            []string    `json:"ips"`
	Subnet         *nets.IPNet `json:"subnet"` // the vip subnet
	Gateway        net.IP      `json:"gateway"`
	Vlan           uint16      `json:"vlan,omitempty"`
}

// MarshalJSON can marshal FloatingIPPoolConf to byte slice.
func (fip *FloatingIPPool) MarshalJSON() ([]byte, error) {
	conf := FloatingIPPoolConf{}
	conf.RoutableSubnet = nets.NetsIPNet(fip.RoutableSubnet)
	conf.Subnet = nets.NetsIPNet(fip.IPNet())
	conf.Gateway = fip.Gateway
	conf.Vlan = fip.Vlan
	conf.IPs = make([]string, 0)
	for _, ipr := range fip.IPRanges {
		conf.IPs = append(conf.IPs, ipr.String())
	}
	return json.Marshal(conf)
}

// UnmarshalJSON can unmarshal byte slice to FloatingIPPoolConf
func (fip *FloatingIPPool) UnmarshalJSON(data []byte) error {
	var conf FloatingIPPoolConf
	if err := json.Unmarshal(data, &conf); err != nil {
		return err
	}
	if conf.RoutableSubnet != nil {
		ipNet := conf.RoutableSubnet.ToIPNet()
		fip.RoutableSubnet = &net.IPNet{IP: ipNet.IP.Mask(ipNet.Mask), Mask: ipNet.Mask}
	} else {
		return fmt.Errorf("routable subnet is empty")
	}
	if conf.Gateway != nil {
		fip.Gateway = conf.Gateway
	} else {
		return fmt.Errorf("gateway is empty")
	}
	if conf.Subnet != nil {
		fip.Mask = conf.Subnet.Mask
	} else {
		return fmt.Errorf("subnet is empty")
	}
	fip.Vlan = conf.Vlan
	for _, str := range conf.IPs {
		ipr := nets.ParseIPRange(str)
		if ipr != nil {
			fip.IPRanges = append(fip.IPRanges, *ipr)
		} else {
			return fmt.Errorf("invalid ip range %s", str)
		}
	}
	return fipCheck(fip)
}

func fipCheck(fip *FloatingIPPool) error {
	net := net.IPNet{IP: fip.Gateway, Mask: fip.Mask}
	for i := range fip.IPRanges {
		if !net.Contains(fip.IPRanges[i].First) || !net.Contains(fip.IPRanges[i].Last) {
			return fmt.Errorf("ip range %s not in subnet %s", fip.IPRanges[i].String(), net.String())
		}
		if i != 0 {
			if nets.IPToInt(fip.IPRanges[i].First) <= nets.IPToInt(fip.IPRanges[i-1].Last)+1 {
				return fmt.Errorf("ip range %s and %s can be merge to one or has wrong order",
					fip.IPRanges[i-1].String(), fip.IPRanges[i].String())
			}
		}
	}
	return nil
}

// String can transform FloatingIP to string.
func (fip *FloatingIPPool) String() string {
	data, err := fip.MarshalJSON()
	if err != nil {
		return "<nil>"
	}
	return string(data)
}

// Key transform floatingIP's subnet to string.
func (fip *FloatingIPPool) Key() string {
	return fip.RoutableSubnet.String()
}

// Contains judge whether FloatingIP struct contains a given ip.
func (fip *FloatingIPPool) Contains(ip net.IP) bool {
	for _, ipr := range fip.IPRanges {
		if ipr.Contains(ip) {
			return true
		}
	}
	return false
}

// #lizard forgives
// InsertIP can insert a given ip to FloatingIP struct.
func (fip *FloatingIPPool) InsertIP(ip net.IP) bool {
	if !fip.SparseSubnet.IPNet().Contains(ip) {
		return false
	}
	if len(fip.IPRanges) == 0 {
		fip.IPRanges = append(fip.IPRanges, nets.IPtoIPRange(ip))
		return true
	}
	for i := range fip.IPRanges {
		if fip.IPRanges[i].Contains(ip) {
			return false
		}
		ret := Minus(fip.IPRanges[i].First, ip)
		if ret > 1 {
			// ip first-last
			if i == 0 {
				fip.IPRanges = append([]nets.IPRange{nets.IPtoIPRange(ip)}, fip.IPRanges...)
			} else {
				fip.IPRanges = append(fip.IPRanges[:i], append([]nets.IPRange{nets.IPtoIPRange(ip)}, fip.IPRanges[i:]...)...)
			}
			return true
		} else if ret == 1 {
			// ip-last
			fip.IPRanges[i].First = ip
			fip.tryMerge(i - 1)
			return true
		}
		if Minus(fip.IPRanges[i].Last, ip) == -1 {
			// first-ip
			fip.IPRanges[i].Last = ip
			fip.tryMerge(i)
			return true
		}
	}
	//first-last first-last ... ip
	fip.IPRanges = append(fip.IPRanges, nets.IPtoIPRange(ip))
	return true
}

func (fip *FloatingIPPool) tryMerge(i int) {
	if i < 0 || i+1 == len(fip.IPRanges) {
		return
	}
	if Minus(fip.IPRanges[i+1].First, fip.IPRanges[i].Last) == 1 {
		fip.IPRanges[i].Last = fip.IPRanges[i+1].Last
		if i+2 < len(fip.IPRanges) {
			fip.IPRanges = append(fip.IPRanges[0:i+1], fip.IPRanges[i+2:]...)
		} else {
			fip.IPRanges = fip.IPRanges[0 : i+1]
		}
	}
}

// RemoveIP can remove a given ip from FloatingIP struct.
func (fip *FloatingIPPool) RemoveIP(ip net.IP) bool {
	if !fip.IPNet().Contains(ip) {
		return false
	}
	if len(fip.IPRanges) == 0 {
		return false
	}

	for i := range fip.IPRanges {
		ipRange := fip.IPRanges[i]
		if ipRange.Contains(ip) {
			ipn := nets.IPToInt(ip)
			switch {
			case ipRange.First.Equal(ipRange.Last):
				fip.IPRanges = append(fip.IPRanges[:i], fip.IPRanges[i+1:]...)
			case ipRange.First.Equal(ip):
				ipRange.First = nets.IntToIP(nets.IPToInt(ipRange.First) + 1)
				fip.IPRanges[i] = ipRange
			case ipRange.Last.Equal(ip):
				ipRange.Last = nets.IntToIP(nets.IPToInt(ipRange.Last) - 1)
				fip.IPRanges[i] = ipRange
			default:
				fip.IPRanges = append(fip.IPRanges[:i+1], append([]nets.IPRange{ipRange}, fip.IPRanges[i+1:]...)...)
				fip.IPRanges[i].Last = nets.IntToIP(ipn - 1)
				fip.IPRanges[i+1].First = nets.IntToIP(ipn + 1)
			}
			return true
		}
	}
	return false
}

// Minus compute how many ips between two given ip.
func Minus(a, b net.IP) int64 {
	return int64(nets.IPToInt(a)) - int64(nets.IPToInt(b))
}

type FloatingIPSlice []*FloatingIPPool

// Len returns number of FloatingIPSlice.
func (s FloatingIPSlice) Len() int {
	return len(s)
}

// Swap can swap two ip in FloatingIPSlice.
func (s FloatingIPSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less compares two given ip.
func (s FloatingIPSlice) Less(i, j int) bool {
	i1, _ := nets.FirstAndLastIP(s[i].RoutableSubnet)
	j1, _ := nets.FirstAndLastIP(s[j].RoutableSubnet)
	return i1 < j1
}
