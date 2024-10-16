// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxlxc

import (
	"context"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
	proxmoxcommon "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

// The unique id for the builder
const BuilderID = "proxmox.lxc"

type Builder struct {
	config Config
}

// Builder implements packersdk.Builder
var _ packersdk.Builder = &Builder{}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	return b.config.Prepare(raws...)
}

// to create a new LXC container without changing the existing common files, we'll bypass the common builder
// and duplicate the code and generate our own coreSteps and vmCreator for now
func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	state := new(multistep.BasicStateBag)

	// state.Put("lxc-config", &b.config)

	coreSteps := []multistep.Step{}
	preSteps := []multistep.Step{}
	postSteps := []multistep.Step{}

	state.Put("template_id", 983724) // placeholder for now

	sb := proxmoxcommon.NewCustomSharedBuilder(BuilderID, b.config.Config, coreSteps, preSteps, postSteps, &lxcVMCreator{})
	return sb.Run(ctx, ui, hook, state)
}

type lxcVMCreator struct{}

func (*lxcVMCreator) Create(vmRef *proxmoxapi.VmRef, config proxmoxapi.ConfigQemu, state multistep.StateBag) error {
	client := state.Get("proxmoxClient").(*proxmoxapi.Client)
	return config.Create(vmRef, client)
}
