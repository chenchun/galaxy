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
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	glog "k8s.io/klog"
	"tkestack.io/galaxy/pkg/api/galaxy/constant"
	"tkestack.io/galaxy/pkg/ipam/apis/galaxy/v1alpha1"
)

func (ci *crdIpam) listFloatingIPs() (*v1alpha1.FloatingIPList, error) {
	val, err := ci.ipType.String()
	if err != nil {
		return nil, err
	}
	listOpt := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", constant.IpType, val),
	}
	fips, err := ci.client.GalaxyV1alpha1().FloatingIPs().List(listOpt)
	if err != nil {
		return nil, err
	}
	return fips, nil
}

func (ci *crdIpam) createFloatingIP(allocated *FloatingIP) error {
	glog.V(4).Infof("create floatingIP %v", *allocated)
	fip := &v1alpha1.FloatingIP{
		TypeMeta:   metav1.TypeMeta{Kind: constant.ResourceKind, APIVersion: constant.ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: allocated.IP.String()},
	}
	assign(fip, allocated)
	ipTypeVal, err := ci.ipType.String()
	if err != nil {
		return err
	}
	label := make(map[string]string)
	label[constant.IpType] = ipTypeVal
	fip.Labels = label
	if _, err := ci.client.GalaxyV1alpha1().FloatingIPs().Create(fip); err != nil {
		return err
	}
	return nil
}

func (ci *crdIpam) deleteFloatingIP(name string) error {
	glog.V(4).Infof("delete floatingIP name %s", name)
	return ci.client.GalaxyV1alpha1().FloatingIPs().Delete(name, &metav1.DeleteOptions{})
}

func (ci *crdIpam) getFloatingIP(name string) error {
	_, err := ci.client.GalaxyV1alpha1().FloatingIPs().Get(name, metav1.GetOptions{})
	return err
}

func (ci *crdIpam) updateFloatingIP(toUpdate *FloatingIP) error {
	glog.V(4).Infof("update floatingIP %v", *toUpdate)
	fip, err := ci.client.GalaxyV1alpha1().FloatingIPs().Get(toUpdate.IP.String(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	assign(fip, toUpdate)
	_, err = ci.client.GalaxyV1alpha1().FloatingIPs().Update(fip)
	return err
}

func assign(spec *v1alpha1.FloatingIP, f *FloatingIP) {
	spec.Spec.Key = f.Key
	spec.Spec.Policy = constant.ReleasePolicy(f.Policy)
	spec.Spec.Attribute = f.Attr
	spec.Spec.Subnet = strings.Join(f.Subnets.List(), ",")
	spec.Spec.UpdateTime = metav1.NewTime(f.UpdatedAt)
}
