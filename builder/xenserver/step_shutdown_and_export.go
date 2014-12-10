package xenserver

/* Taken from https://raw.githubusercontent.com/mitchellh/packer/master/builder/qemu/step_prepare_output_dir.go */

import (
	"crypto/tls"
	"fmt"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
	"io"
	"net/http"
	"os"
)

type stepShutdownAndExport struct{}

func downloadFile(url, filename string) (err error) {

	// Create the file
	fh, err := os.Create(filename)
	if err != nil {
		return err
	}

	// Define a new transport which allows self-signed certs
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// Create a client
	client := &http.Client{Transport: tr}

	// Create request and download file

	resp, err := client.Get(url)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	io.Copy(fh, resp.Body)

	return nil
}

func (stepShutdownAndExport) Run(state multistep.StateBag) multistep.StepAction {
	config := state.Get("config").(config)
	ui := state.Get("ui").(packer.Ui)
	client := state.Get("client").(XenAPIClient)
	instance_uuid := state.Get("instance_uuid").(string)

	instance, err := client.GetVMByUuid(instance_uuid)
	if err != nil {
		ui.Error(fmt.Sprintf("Could not get VM with UUID '%s': %s", instance_uuid, err.Error()))
		return multistep.ActionHalt
	}

	ui.Say("Step: Shutdown and export VPX")

	// Shutdown the VM
	ui.Say("Shutting down the VM...")
	err = instance.CleanShutdown()
	if err != nil {
		ui.Error(fmt.Sprintf("Could not shut down VM: %s", err.Error()))
		return multistep.ActionHalt
	}

	//Export the VM

	export_url := fmt.Sprintf("https://%s/export?vm=%s&session_id=%s",
		client.Host,
		instance.Ref,
		client.Session.(string),
	)

	export_filename := fmt.Sprintf("%s/%s.xva", config.OutputDir, config.InstanceName)
	ui.Say("Getting metadata " + export_url)
	downloadFile(export_url, export_filename)

	disks, err := instance.GetDisks()
	if err != nil {
		ui.Error(fmt.Sprintf("Could not get VM disks: %s", err.Error()))
		return multistep.ActionHalt
	}
	for _, disk := range disks {
		disk_uuid, err := disk.GetUuid()
		if err != nil {
			ui.Error(fmt.Sprintf("Could not get disk with UUID '%s': %s", disk_uuid, err.Error()))
			return multistep.ActionHalt
		}

		// Basic auth in URL request is required as session token is not
		// accepted for some reason.
		// @todo: raise with XAPI team.
		disk_export_url := fmt.Sprintf("https://%s:%s@%s/export_raw_vdi?vdi=%s",
			client.Username,
			client.Password,
			client.Host,
			disk_uuid,
		)

		ui.Say("Getting " + disk_export_url)
		disk_export_filename := fmt.Sprintf("%s/%s.raw", config.OutputDir, disk_uuid)
		ui.Say("Downloading " + disk_uuid)
		downloadFile(disk_export_url, disk_export_filename)
	}

	ui.Say("Download completed: " + config.OutputDir)

	return multistep.ActionContinue
}

func (stepShutdownAndExport) Cleanup(state multistep.StateBag) {
}