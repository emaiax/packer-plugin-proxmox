// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package proxmox

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

// stepSuccess runs after the full build has succeeded.
//
// It sets the success state, which ensures cleanup does not remove the finished template
type StepSuccess struct{}

func (s *StepSuccess) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	// We need to ensure stepStartVM.Cleanup doesn't delete the template (no
	// difference between VMs and templates when deleting)
	state.Put("success", true)

	return multistep.ActionContinue
}

func (s *StepSuccess) Cleanup(state multistep.StateBag) {}
