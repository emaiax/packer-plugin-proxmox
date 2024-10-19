package proxmoxlxc

import (
	"testing"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
)

func TestCommunicator_ImplementsCommunicator(t *testing.T) {
	var raw interface{}
	raw = &PctCommunicator{}
	if _, ok := raw.(packersdk.Communicator); !ok {
		t.Fatalf("Communicator should be a communicator")
	}
}
