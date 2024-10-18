package proxmoxlxc

import (
	"context"
	"fmt"
	"log"
	"strings"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

var (
	maxDuplicateIDRetries = 3
)

type stepCreateLxc struct {
	vmCreator ProxmoxVMCreator
}

type ProxmoxVMCreator interface {
	Create(*proxmoxapi.VmRef, proxmoxapi.ConfigLxc, multistep.StateBag) error
}

type vmStarter interface {
	CheckVmRef(vmr *proxmoxapi.VmRef) (err error)
	DeleteVm(vmr *proxmoxapi.VmRef) (exitStatus string, err error)
	GetNextID(int) (int, error)
	GetVmConfig(vmr *proxmoxapi.VmRef) (vmConfig map[string]interface{}, err error)
	GetVmRefsByName(vmName string) (vmrs []*proxmoxapi.VmRef, err error)
	SetVmConfig(*proxmoxapi.VmRef, map[string]interface{}) (interface{}, error)
	StartVm(*proxmoxapi.VmRef) (string, error)
}

// Check if the given builder configuration maps to an existing VM template on the Proxmox cluster.
// Returns an empty *proxmoxapi.VmRef when no matching ID or name is found.
func getExistingTemplate(c *Config, client vmStarter) (*proxmoxapi.VmRef, error) {
	vmRef := &proxmoxapi.VmRef{}
	if c.VMID > 0 {
		log.Printf("looking up VM with ID %d", c.VMID)
		vmRef = proxmoxapi.NewVmRef(c.VMID)
		err := client.CheckVmRef(vmRef)
		if err != nil {
			// expect an error if no VM is found
			// the error string is defined in GetVmInfo() of proxmox-api-go
			notFoundError := fmt.Sprintf("vm '%d' not found", c.VMID)
			if err.Error() == notFoundError {
				log.Println(err.Error())
				return &proxmoxapi.VmRef{}, nil
			}
			return &proxmoxapi.VmRef{}, err
		}
		log.Printf("found VM with ID %d", vmRef.VmId())
	} else {
		log.Printf("looking up VMs with name '%s'", c.Hostname)
		vmRefs, err := client.GetVmRefsByName(c.Hostname)
		if err != nil {
			// expect an error if no VMs are found
			// the error string is defined in GetVmRefsByName() of proxmox-api-go
			notFoundError := fmt.Sprintf("vm '%s' not found", c.Hostname)
			if err.Error() == notFoundError {
				log.Println(err.Error())
				return &proxmoxapi.VmRef{}, nil
			}
			return &proxmoxapi.VmRef{}, err
		}
		if len(vmRefs) > 1 {
			vmIDs := []int{}
			for _, vmr := range vmRefs {
				vmIDs = append(vmIDs, vmr.VmId())
			}
			return &proxmoxapi.VmRef{}, fmt.Errorf("found multiple VMs with name '%s', IDs: %v", c.Hostname, vmIDs)
		}
		vmRef = vmRefs[0]
		log.Printf("found VM with name '%s' (ID: %d)", c.Hostname, vmRef.VmId())
	}
	log.Printf("check if VM %d is a template", vmRef.VmId())
	vmConfig, err := client.GetVmConfig(vmRef)
	if err != nil {
		return &proxmoxapi.VmRef{}, err
	}
	log.Printf("VM %d template: %d", vmRef.VmId(), vmConfig["template"])
	if vmConfig["template"] == nil {
		// return &proxmoxapi.VmRef{}, fmt.Errorf("found matching VM (ID: %d, name: %s), but it is not a template", vmRef.VmId(), vmConfig["name"])
		log.Printf("found matching VM (ID: %d, name: %s), but it is not a template, will continue either way", vmRef.VmId(), vmConfig["name"])
	}
	return vmRef, nil
}

func (s *stepCreateLxc) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	client := state.Get("proxmoxClient").(*proxmoxapi.Client)
	c := state.Get("config").(*Config)

	ui.Say("Creating LXC container")

	config := proxmoxapi.NewConfigLxc() // proxmoxapi.ConfigLxc{}

	if c.Arch != "" {
		config.Arch = c.Arch
	}

	// // config.BWLimit = c
	// // config.Clone = c.Clone
	// // config.CloneStorage = c.CloneStorage
	config.CMode = c.CMode
	config.Console = c.Console
	config.Cores = c.Cores
	config.CPULimit = c.CpuLimit
	config.CPUUnits = c.CpuUnits
	config.Description = c.Description
	// config.Features = c.Features // idk-idc
	config.Force = c.Force
	config.Hookscript = c.Hookscript
	config.Hostname = c.Hostname
	config.IgnoreUnpackErrors = c.IgnoreUnpackErrors
	config.Lock = c.Lock
	config.Memory = c.Memory
	config.Mountpoints = generateMountPoints(c.MountPoints, false)
	config.Nameserver = c.Nameserver
	config.Networks = generateNetworkInterfaces(c.NetworkInterfaces)
	config.OnBoot = c.OnBoot
	config.OsType = c.OSType
	config.Ostemplate = c.OsTemplate
	config.Password = c.UserPassword
	pool := proxmoxapi.PoolName(c.Pool) // convert c.Pool to proxmoxapi.PoolName
	config.Pool = &pool                 // set config.Pool to the address of pool
	config.Protection = c.Protection
	config.Restore = c.Restore
	// config.RootFs = generateMountPoints([]MountPointConfig{c.RootFS})[0]
	if c.RootFS != nil {
		config.RootFs = generateMountPoints([]MountPointConfig{*c.RootFS}, true)[0]
	}
	config.SearchDomain = c.SearchDomain
	config.SSHPublicKeys = c.SSHPublicKeys
	config.Start = c.Start
	config.Startup = c.Startup
	config.Storage = c.Storage
	config.Swap = c.Swap
	config.Template = c.Template
	config.Tty = c.TTY
	config.Unique = c.Unique
	config.Unprivileged = c.Unprivileged
	config.Tags = strings.Join(c.Tags, ",")

	log.Printf("CONFIG: %+v", c)

	ui.Say("Checking VM id")
	var vmRef *proxmoxapi.VmRef
	for i := 1; ; i++ {
		id := c.VMID
		if id == 0 {
			ui.Say("No VM ID given, getting next free from Proxmox")
			genID, err := client.GetNextID(0)
			if err != nil {
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			id = genID
			// config.VmID = genID
		}

		ui.Say(fmt.Sprintf("Force: %v", c.Force))
		ui.Say(fmt.Sprintf("PackerForce: %v", c.PackerForce))

		if c.PackerForce || c.Force {
			ui.Say("Force set, checking for existing artifact on PVE cluster")
			vmRef, err := getExistingTemplate(c, client)

			if err != nil {
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
			if vmRef.VmId() != 0 {
				ui.Say(fmt.Sprintf("found existing VM template with ID %d on PVE node %s, deleting it", vmRef.VmId(), vmRef.Node()))
				_, err = client.StopVm(vmRef)
				if err != nil {
					state.Put("error", err)
					ui.Error(fmt.Sprintf("error stopping VM: %s", err.Error()))
					// return multistep.ActionHalt
				}
				_, err = client.DeleteVm(vmRef)
				if err != nil {
					state.Put("error", err)
					ui.Error(fmt.Sprintf("error deleting VM template: %s", err.Error()))
					return multistep.ActionHalt
				}
				ui.Say(fmt.Sprintf("Successfully deleted VM template %d", vmRef.VmId()))
			} else {
				ui.Say("No existing artifact found")
			}
		}

		vmRef = proxmoxapi.NewVmRef(id)
		vmRef.SetNode(c.Node)
		if c.Pool != "" {
			vmRef.SetPool(c.Pool)

			// defined above with other configs
			config.Pool = &pool
		}

		// log.Printf("[rootfs] %v", config.RootFs)
		// log.Printf("[before] config = %v", c)
		log.Printf("ProxmoxClient: %+v", client)

		err := config.CreateLxc(vmRef, client)
		if err == nil {
			break
		}

		// If there's no explicitly configured VMID, and the error is caused
		// by a race condition in someone else using the ID we just got
		// generated, we'll retry up to maxDuplicateIDRetries times.
		if c.VMID == 0 && isDuplicateIDError(err) && i < maxDuplicateIDRetries {
			ui.Say("Generated VM ID was already allocated, retrying")
			continue
		}
		err = fmt.Errorf("error creating VM: %s", err)
		state.Put("error", err)
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	// // Store the vm id for later
	// state.Put("vmRef", vmRef)
	// state.Put("createTemplate", config.Template)

	// log.Printf("config = %v", c)
	// log.Printf("client = %v", client)

	return multistep.ActionContinue
}

func generateNetworkInterfaces(nics []NetworkInterfacesConfig) proxmoxapi.QemuDevices {
	devs := make(proxmoxapi.QemuDevices)
	for idx := range nics {
		nic := nics[idx]
		devs[idx] = make(proxmoxapi.QemuDevice)
		devs[idx] = proxmoxapi.QemuDevice{
			"name":      nic.Name,
			"bridge":    nic.Bridge,
			"firewall":  nic.Firewall,
			"gw":        nic.GatewayIPv4,
			"gw6":       nic.GatewayIPv6,
			"hwaddr":    nic.MACAddress,
			"ip":        nic.IPv4Address,
			"ip6":       nic.IPv6Address,
			"link_down": nic.LinkDown,
			"mtu":       nic.MTU,
			"rate":      nic.RateMbps,
			"tag":       nic.Tag,
			"trunks":    strings.Join(nic.Trunks, ":"),
			"type":      nic.Type,
		}
	}
	return devs
}

func generateMountPoints(disks []MountPointConfig, isRootFs bool) proxmoxapi.QemuDevices {
	devs := make(proxmoxapi.QemuDevices)
	for idx := range disks {
		devs[idx] = make(proxmoxapi.QemuDevice)
		setDeviceParamIfDefined(devs[idx], "storage", disks[idx].StorageId)
		setDeviceParamIfDefined(devs[idx], "volume", disks[idx].Volume)
		devs[idx]["size"] = disks[idx].DiskSize
		if len(disks[idx].MountOptions) > 0 {
			devs[idx]["mountoptions"] = disks[idx].MountOptions
		}
		devs[idx]["quota"] = disks[idx].Quota
		devs[idx]["replicate"] = disks[idx].Replicate
		devs[idx]["ro"] = disks[idx].ReadOnly
		devs[idx]["shared"] = disks[idx].Shared
		if !isRootFs {
			devs[idx]["backup"] = disks[idx].Backup
		}

		// log.Printf("[devs[%v]] %v", idx, devs[idx])
	}

	// log.Printf("[devs] %v", devs)
	return devs
}

func setDeviceParamIfDefined(dev proxmoxapi.QemuDevice, key, value string) {
	if value != "" {
		dev[key] = value
	}
}

func isDuplicateIDError(err error) bool {
	return strings.Contains(err.Error(), "already exists on node")
}

func (s *stepCreateLxc) Cleanup(state multistep.StateBag) {
	vmRefUntyped, ok := state.GetOk("vmRef")
	// If not ok, we probably errored out before creating the VM
	if !ok {
		return
	}
	vmRef := vmRefUntyped.(*proxmoxapi.VmRef)

	// The vmRef will actually refer to the created template if everything
	// finished successfully, so in that case we shouldn't cleanup
	if _, ok := state.GetOk("success"); ok {
		return
	}

	client := state.Get("proxmoxClient").(*proxmoxapi.Client)
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Stopping Container")
	_, err := client.StopVm(vmRef)

	if err != nil {
		ui.Error(fmt.Sprintf("Error stopping VM. Please stop and delete it manually: %s", err))
		return
	}

	ui.Say("Deleting VM")
	_, err = client.DeleteVm(vmRef)
	if err != nil {
		ui.Error(fmt.Sprintf("Error deleting VM. Please delete it manually: %s", err))
		return
	}
}
