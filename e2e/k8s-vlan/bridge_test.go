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
package k8s_vlan

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"tkestack.io/galaxy/e2e/helper"
	"tkestack.io/galaxy/pkg/utils"
	"tkestack.io/galaxy/pkg/utils/ips"
)

var _ = Describe("galaxy-k8s-vlan bridge and pure test", func() {
	cni := "galaxy-k8s-vlan"
	ifaceCidr := "192.168.0.66/26"
	containerCidr := "192.168.0.68/26"
	containerId := helper.NewContainerId()
	cidrIPNet, _ := ips.ParseCIDR(ifaceCidr)
	argsStr, _ := helper.IPInfo(containerCidr, 0)

	var cmdAdd = func(netConf []byte) string {
		nsPath := helper.CmdAdd(containerId, ifaceCidr, argsStr, cni,
			`{"cniVersion":"0.2.0","ip4":{"ip":"192.168.0.68/26","gateway":"192.168.0.65","routes":[{"dst":"0.0.0.0/0"}]},"dns":{}}`, netConf)
		_, err := helper.Ping("192.168.0.68")
		Expect(err).NotTo(HaveOccurred())
		return nsPath
	}

	AfterEach(func() {
		helper.CleanupNetNS()
		helper.CleanupDummy()
		helper.CleanupIFace("brtest")
	})
	It("bridge", func() {
		netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-k8s-vlan",
    "device": "dummy0",
    "default_bridge_name": "brtest"
}`)
		nsPath := cmdAdd(netConf)
		// check host iface topology, route, neigh, ip address is expected
		//TODO verify why ifaceCidr is still on dummy0
		//err := (&helper.NetworkTopology{
		//	LeaveDevices: []*helper.LinkDevice{
		//		helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth").SetMaster(
		//			helper.NewLinkDevice(cidrIPNet, "brtest", "bridge"),
		//		),
		//		helper.NewLinkDevice(nil, "dummy0", "dummy").SetMaster(
		//			helper.NewLinkDevice(cidrIPNet, "brtest", "bridge"),
		//		),
		//	},
		//}).Verify()
		//Expect(err).Should(BeNil(), "%v", err)

		// check container iface topology, route, neigh, ip address is expected
		helper.CheckContainerTopology(nsPath, containerCidr, "192.168.0.65")

		// test DEL command
		helper.CmdDel(containerId, cni, netConf)
	})

	It("pure switch", func() {
		netConf := []byte(`{
    "name": "myvlan",
    "type": "galaxy-k8s-vlan",
    "device": "dummy0",
    "switch": "pure"
}`)
		nsPath := cmdAdd(netConf)
		// check host iface topology, route, neigh, ip address is expected
		err := (&helper.NetworkTopology{
			LeaveDevices: []*helper.LinkDevice{
				helper.NewLinkDevice(nil, utils.HostVethName(containerId, ""), "veth"),
				helper.NewLinkDevice(cidrIPNet, "dummy0", "dummy"),
			},
		}).Verify()
		Expect(err).Should(BeNil(), "%v", err)

		// check container iface topology, route, neigh, ip address is expected
		helper.CheckContainerTopology(nsPath, containerCidr, "192.168.0.65")
	})
})
