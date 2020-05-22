/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"fmt"
	"hash/fnv"

	"github.com/pkg/errors"
	"k8s.io/klog"

	azure "sigs.k8s.io/cluster-api-provider-azure/cloud"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/scope"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/availabilityzones"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/groups"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/internalloadbalancers"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/publicips"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/publicloadbalancers"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/routetables"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/securitygroups"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/subnets"
	"sigs.k8s.io/cluster-api-provider-azure/cloud/services/virtualnetworks"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

// azureClusterReconciler are list of services required by cluster controller
type azureClusterReconciler struct {
	scope            *scope.ClusterScope
	groupsSvc        azure.Service
	vnetSvc          azure.Service
	securityGroupSvc azure.Service
	routeTableSvc    azure.Service
	subnetsSvc       azure.Service
	internalLBSvc    azure.Service
	publicIPSvc      azure.Service
	publicLBSvc      azure.Service
	availZoneSvc     azure.GetterService
}

// newAzureClusterReconciler populates all the services based on input scope
func newAzureClusterReconciler(scope *scope.ClusterScope) *azureClusterReconciler {
	return &azureClusterReconciler{
		scope:            scope,
		groupsSvc:        groups.NewService(scope),
		vnetSvc:          virtualnetworks.NewService(scope),
		securityGroupSvc: securitygroups.NewService(scope),
		routeTableSvc:    routetables.NewService(scope),
		subnetsSvc:       subnets.NewService(scope),
		internalLBSvc:    internalloadbalancers.NewService(scope),
		publicIPSvc:      publicips.NewService(scope),
		publicLBSvc:      publicloadbalancers.NewService(scope),
		availZoneSvc:     availabilityzones.NewService(scope),
	}
}

// Reconcile reconciles all the services in pre determined order
func (r *azureClusterReconciler) Reconcile() error {
	klog.V(2).Infof("reconciling cluster %s", r.scope.Name())
	r.createOrUpdateNetworkAPIServerIP()

	if err := r.setFailureDomainsForLocation(); err != nil {
		return errors.Wrapf(err, "failed to get availability zones for cluster %s in location %s", r.scope.Name(), r.scope.Location())
	}

	if err := r.groupsSvc.Reconcile(r.scope.Context, nil); err != nil {
		return errors.Wrapf(err, "failed to reconcile resource group for cluster %s", r.scope.Name())
	}

	vnetCidrs := []string{r.scope.Vnet().CidrBlock}
	if r.scope.Vnet().IPv6Enabled {
		vnetCidrs = append(vnetCidrs, r.scope.Vnet().IPv6CidrBlock)
	}

	vnetSpec := &virtualnetworks.Spec{
		ResourceGroup: r.scope.Vnet().ResourceGroup,
		Name:          r.scope.Vnet().Name,
		CIDRs:         vnetCidrs,
	}

	if err := r.vnetSvc.Reconcile(r.scope.Context, vnetSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile virtual network for cluster %s", r.scope.Name())
	}

	sgSpec := &securitygroups.Spec{
		Name:           r.scope.ControlPlaneSubnet().SecurityGroup.Name,
		IsControlPlane: true,
	}
	if err := r.securityGroupSvc.Reconcile(r.scope.Context, sgSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile control plane network security group for cluster %s", r.scope.Name())
	}

	sgSpec = &securitygroups.Spec{
		Name:           r.scope.NodeSubnet().SecurityGroup.Name,
		IsControlPlane: false,
	}
	if err := r.securityGroupSvc.Reconcile(r.scope.Context, sgSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile node network security group for cluster %s", r.scope.Name())
	}

	rtSpec := &routetables.Spec{
		Name: r.scope.NodeSubnet().RouteTable.Name,
	}
	if err := r.routeTableSvc.Reconcile(r.scope.Context, rtSpec); err != nil {

		return errors.Wrapf(err, "failed to reconcile route table %s for cluster %s", r.scope.NodeSubnet().RouteTable.Name, r.scope.Name())
	}

	cpSubnetCidrs := []string{r.scope.ControlPlaneSubnet().CidrBlock}
	if r.scope.Vnet().IPv6Enabled {
		cpSubnetCidrs = append(cpSubnetCidrs, r.scope.ControlPlaneSubnet().IPv6CidrBlock)
	}

	subnetSpec := &subnets.Spec{
		Name:                r.scope.ControlPlaneSubnet().Name,
		CIDRs:               cpSubnetCidrs,
		VnetName:            r.scope.Vnet().Name,
		SecurityGroupName:   r.scope.ControlPlaneSubnet().SecurityGroup.Name,
		Role:                r.scope.ControlPlaneSubnet().Role,
		RouteTableName:      r.scope.ControlPlaneSubnet().RouteTable.Name,
		InternalLBIPAddress: r.scope.ControlPlaneSubnet().InternalLBIPAddress,
	}
	if err := r.subnetsSvc.Reconcile(r.scope.Context, subnetSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile control plane subnet for cluster %s", r.scope.Name())
	}

	nodeSubnetCidrs := []string{r.scope.NodeSubnet().CidrBlock}
	if r.scope.Vnet().IPv6Enabled {
		nodeSubnetCidrs = append(nodeSubnetCidrs, r.scope.NodeSubnet().IPv6CidrBlock)
	}

	subnetSpec = &subnets.Spec{
		Name:              r.scope.NodeSubnet().Name,
		CIDRs:             nodeSubnetCidrs,
		VnetName:          r.scope.Vnet().Name,
		SecurityGroupName: r.scope.NodeSubnet().SecurityGroup.Name,
		RouteTableName:    r.scope.NodeSubnet().RouteTable.Name,
		Role:              r.scope.NodeSubnet().Role,
	}
	if err := r.subnetsSvc.Reconcile(r.scope.Context, subnetSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile node subnet for cluster %s", r.scope.Name())
	}

	internalLBSpec := &internalloadbalancers.Spec{
		Name:       azure.GenerateInternalLBName("control-plane"),
		SubnetName: r.scope.ControlPlaneSubnet().Name,
		SubnetCidr: r.scope.ControlPlaneSubnet().CidrBlock,
		VnetName:   r.scope.Vnet().Name,
		IPAddress:  r.scope.ControlPlaneSubnet().InternalLBIPAddress,
	}
	if err := r.internalLBSvc.Reconcile(r.scope.Context, internalLBSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile control plane internal load balancer for cluster %s", r.scope.Name())
	}

	publicIPSpec := &publicips.Spec{
		Name:    r.scope.Network().APIServerIP.Name,
		DNSName: r.scope.Network().APIServerIP.DNSName,
		IsIPV6:  false,
	}
	if err := r.publicIPSvc.Reconcile(r.scope.Context, publicIPSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile control plane public ip for cluster %s", r.scope.Name())
	}

	publicLBSpec := &publicloadbalancers.Spec{
		Name:         azure.GeneratePublicLBName("control-plane"),
		PublicIPName: r.scope.Network().APIServerIP.Name,
		IsAPIServer:  true,
	}
	if err := r.publicLBSvc.Reconcile(r.scope.Context, publicLBSpec); err != nil {
		return errors.Wrapf(err, "failed to reconcile control plane public load balancer for cluster %s", r.scope.Name())
	}

	// If ipv6 is enabled then a load balancer needs to be created for outbound traffic to work
	if r.scope.IsIPV6Enabled() {
		h := fnv.New32a()
		h.Write([]byte(fmt.Sprintf("%s/%s/%s", r.scope.SubscriptionID, r.scope.ResourceGroup(), r.scope.Name())))
		ipname := azure.GeneratePublicIPName("cluster", fmt.Sprintf("%x", h.Sum32()))

		clusteripv4 := &publicips.Spec{
			Name:   ipname,
			IsIPV6: false,
		}
		if err := r.publicIPSvc.Reconcile(r.scope.Context, clusteripv4); err != nil {
			return errors.Wrapf(err, "failed to reconcile control plane public ip for cluster %s", r.scope.Name())
		}

		clusterLBSpec := &publicloadbalancers.Spec{
			Name:         azure.GeneratePublicLBName("cluster"),
			PublicIPName: ipname,
		}
		if err := r.publicLBSvc.Reconcile(r.scope.Context, clusterLBSpec); err != nil {
			return errors.Wrapf(err, "failed to reconcile control plane public load balancer for cluster %s", r.scope.Name())
		}
	}

	return nil
}

// Delete reconciles all the services in pre determined order
func (r *azureClusterReconciler) Delete() error {
	if err := r.deleteLB(); err != nil {
		return errors.Wrap(err, "failed to delete load balancer")
	}

	if err := r.deleteSubnets(); err != nil {
		return errors.Wrap(err, "failed to delete subnets")
	}

	rtSpec := &routetables.Spec{
		Name: r.scope.NodeSubnet().RouteTable.Name,
	}
	if err := r.routeTableSvc.Delete(r.scope.Context, rtSpec); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to delete route table %s for cluster %s", r.scope.NodeSubnet().RouteTable.Name, r.scope.Name())
		}
	}

	if err := r.deleteNSG(); err != nil {
		return errors.Wrap(err, "failed to delete network security group")
	}

	vnetSpec := &virtualnetworks.Spec{
		ResourceGroup: r.scope.Vnet().ResourceGroup,
		Name:          r.scope.Vnet().Name,
	}
	if err := r.vnetSvc.Delete(r.scope.Context, vnetSpec); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to delete virtual network %s for cluster %s", r.scope.Vnet().Name, r.scope.Name())
		}
	}

	if err := r.groupsSvc.Delete(r.scope.Context, nil); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to delete resource group for cluster %s", r.scope.Name())
		}
	}

	return nil
}

func (r *azureClusterReconciler) deleteLB() error {
	publicLBSpec := &publicloadbalancers.Spec{
		Name: azure.GeneratePublicLBName(r.scope.Name()),
	}
	if err := r.publicLBSvc.Delete(r.scope.Context, publicLBSpec); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to delete lb %s for cluster %s", azure.GeneratePublicLBName(r.scope.Name()), r.scope.Name())
		}
	}
	publicIPSpec := &publicips.Spec{
		Name: r.scope.Network().APIServerIP.Name,
	}
	if err := r.publicIPSvc.Delete(r.scope.Context, publicIPSpec); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to delete public ip %s for cluster %s", r.scope.Network().APIServerIP.Name, r.scope.Name())
		}
	}

	internalLBSpec := &internalloadbalancers.Spec{
		Name: azure.GenerateInternalLBName(r.scope.Name()),
	}
	if err := r.internalLBSvc.Delete(r.scope.Context, internalLBSpec); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to internal load balancer %s for cluster %s", azure.GenerateInternalLBName(r.scope.Name()), r.scope.Name())
		}
	}

	return nil
}

func (r *azureClusterReconciler) deleteSubnets() error {
	for _, s := range r.scope.Subnets() {
		subnetSpec := &subnets.Spec{
			Name:     s.Name,
			VnetName: r.scope.Vnet().Name,
		}
		if err := r.subnetsSvc.Delete(r.scope.Context, subnetSpec); err != nil {
			if !azure.ResourceNotFound(err) {
				return errors.Wrapf(err, "failed to delete %s subnet for cluster %s", s.Name, r.scope.Name())
			}
		}
	}
	return nil
}

func (r *azureClusterReconciler) deleteNSG() error {
	sgSpec := &securitygroups.Spec{
		Name: r.scope.NodeSubnet().SecurityGroup.Name,
	}
	if err := r.securityGroupSvc.Delete(r.scope.Context, sgSpec); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to delete security group %s for cluster %s", r.scope.NodeSubnet().SecurityGroup.Name, r.scope.Name())
		}
	}
	sgSpec = &securitygroups.Spec{
		Name: r.scope.ControlPlaneSubnet().SecurityGroup.Name,
	}
	if err := r.securityGroupSvc.Delete(r.scope.Context, sgSpec); err != nil {
		if !azure.ResourceNotFound(err) {
			return errors.Wrapf(err, "failed to delete security group %s for cluster %s", r.scope.ControlPlaneSubnet().SecurityGroup.Name, r.scope.Name())
		}
	}

	return nil
}

// CreateOrUpdateNetworkAPIServerIP creates or updates public ip name and dns name
func (r *azureClusterReconciler) createOrUpdateNetworkAPIServerIP() {
	if r.scope.Network().APIServerIP.Name == "" {
		h := fnv.New32a()
		h.Write([]byte(fmt.Sprintf("%s/%s/%s", r.scope.SubscriptionID, r.scope.ResourceGroup(), r.scope.Name())))
		r.scope.Network().APIServerIP.Name = azure.GeneratePublicIPName(r.scope.Name(), fmt.Sprintf("%x", h.Sum32()))
	}

	r.scope.Network().APIServerIP.DNSName = azure.GenerateFQDN(r.scope.Network().APIServerIP.Name, r.scope.Location())
}

func (r *azureClusterReconciler) setFailureDomainsForLocation() error {

	spec := &availabilityzones.Spec{}

	zonesInterface, err := r.availZoneSvc.Get(r.scope.Context, spec)
	if err != nil {
		return err
	}

	zones := zonesInterface.([]string)
	for _, zone := range zones {
		r.scope.SetFailureDomain(zone, clusterv1.FailureDomainSpec{
			ControlPlane: true,
		})
	}

	return nil
}
