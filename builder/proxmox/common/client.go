// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// TODO: proxmox-lxc couldn't parse the proxmoxURL correctly, revisit later
package proxmox

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"strings"

	proxmoxapi "github.com/Telmate/proxmox-api-go/proxmox"
)

func newProxmoxClient(config Config) (*proxmoxapi.Client, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.SkipCertValidation,
	}

	if config.proxmoxURL == nil {
		parsedUrl, err := url.Parse(config.ProxmoxURLRaw)
		if err != nil {
			return nil, fmt.Errorf("proxmox_url is wrong, value is: %s", parsedUrl)
		}

		config.proxmoxURL = parsedUrl
		log.Printf("Connecting to Proxmox URL: %s", config.proxmoxURL)
	}

	client, err := proxmoxapi.NewClient(strings.TrimSuffix(strings.TrimSpace(config.proxmoxURL.String()), "/"), nil, "", tlsConfig, "", int(config.TaskTimeout.Seconds()))
	if err != nil {
		return nil, err
	}

	*proxmoxapi.Debug = config.PackerDebug

	if config.Token != "" {
		// configure token auth
		log.Print("using token auth")
		client.SetAPIToken(config.Username, config.Token)
	} else {
		// fallback to login if not using tokens
		log.Print("using password auth")
		err = client.Login(config.Username, config.Password, "")
		if err != nil {
			return nil, err
		}
	}

	return client, nil
}
