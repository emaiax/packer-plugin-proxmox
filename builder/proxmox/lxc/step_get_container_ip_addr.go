package proxmoxlxc

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Telmate/proxmox-api-go/proxmox"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

type stepGetContainerIpAddr struct{}

func (s *stepGetContainerIpAddr) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	comm, ok := state.Get("communicator").(packersdk.Communicator)

	ui.Say("Getting container IP address")
	if !ok {
		state.Put("error", "bad")
		ui.Error("could not retrieve communicator from state")
		return multistep.ActionHalt
	}

	vmRef := state.Get("vmRef").(*proxmox.VmRef)

	var buf bytes.Buffer
	maxRetries := 5
	retryCount := 0
	var ip string

	for retryCount < maxRetries {
		buf.Reset()
		cmd := packersdk.RemoteCmd{
			Command: fmt.Sprintf("lxc-info -n %d -i -H", vmRef.VmId()),
			Stdout:  &buf,
			Stderr:  &buf,
		}
		err := cmd.RunWithUi(ctx, comm, ui)

		if err != nil {
			state.Put("error", err)
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		ip = strings.TrimSuffix(buf.String(), "\n")
		if ip != "" {
			break
		}

		retryCount++
		ui.Message("IP address not found yet, retrying...")
		time.Sleep(1 * time.Second)
	}

	if ip == "" {
		err := fmt.Errorf("failed to get IP address after %d retries", maxRetries)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	log.Printf("Container IP: %s", ip)

	state.Put("containerIp", ip)

	return multistep.ActionContinue
}

func (s *stepGetContainerIpAddr) Cleanup(state multistep.StateBag) {
}
