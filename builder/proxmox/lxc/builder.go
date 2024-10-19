// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmoxlxc

import (
	"context"
	"errors"
	"fmt"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/hcl/v2/hcldec"
	common "github.com/hashicorp/packer-plugin-proxmox/builder/proxmox/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

// The unique id for the builder
const BuilderID = "proxmox.lxc"

type Builder struct {
	id            string
	config        Config
	coreSteps     []multistep.Step // override entire coreSteps
	preSteps      []multistep.Step
	postSteps     []multistep.Step
	runner        multistep.Runner
	proxmoxClient *proxmox.Client
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

	var err error

	b.proxmoxClient, err = common.NewProxmoxClient(b.config.Config)
	if err != nil {
		return nil, err
	}

	// Set up the state
	state.Put("config", &b.config)
	state.Put("proxmoxClient", b.proxmoxClient)
	state.Put("hook", hook)
	state.Put("ui", ui)

	hostComm := &b.config.Comm

	b.coreSteps = []multistep.Step{
		&stepCreateLxc{},
		&communicator.StepConnect{
			Config:    hostComm,
			Host:      commHost(hostComm.Host()),
			SSHConfig: (*hostComm).SSHConfigFunc(),
		},
		&stepGetContainerIpAddr{},
	}
	b.preSteps = []multistep.Step{}
	b.postSteps = []multistep.Step{}

	steps := b.coreSteps
	steps = append(b.preSteps, steps...)
	steps = append(steps, b.postSteps...)

	// placeholder to successfully run the builder
	state.Put("template_id", 983724)
	// sb := proxmoxcommon.NewCustomSharedBuilder(BuilderID, b.config.Config, coreSteps, preSteps, postSteps, &ProxmoxVMCreator{})
	// return sb.Run(ctx, ui, hook, state)

	// Run the steps
	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	// If we were interrupted or cancelled, then just exit.
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, errors.New("build was cancelled")
	}

	// Verify that the template_id was set properly, otherwise we didn't progress through the last step
	templateId, ok := state.Get("template_id").(int)
	if !ok {
		return nil, fmt.Errorf("template ID could not be determined")
	}

	artifact := common.NewArtifact(
		b.id,
		templateId,
		b.proxmoxClient,
		map[string]interface{}{"generated_data": state.Get("generated_data")},
	)

	return artifact, nil
}

// type lxcVMCreator struct{}

// func (*lxcVMCreator) Create(vmRef *proxmox.VmRef, config proxmox.ConfigLxc, state multistep.StateBag) error {
// 	client := state.Get("proxmoxClient").(*proxmox.Client)
// 	return config.CreateLxc(vmRef, client)
// }

// Returns ssh_host or winrm_host (see communicator.Config.Host) config
// parameter when set, otherwise gets the host IP from running VM
func commHost(host string) func(state multistep.StateBag) (string, error) {
	return func(state multistep.StateBag) (string, error) {
		if host == "" {
			return "", errors.New("no host set")
		}
		return host, nil
	}
}
