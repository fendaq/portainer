package plugins

import (
	"bytes"
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/asaskevich/govalidator"
	httperror "github.com/portainer/libhttp/error"
	"github.com/portainer/libhttp/request"
	"github.com/portainer/libhttp/response"
	"github.com/portainer/portainer"
	"github.com/portainer/portainer/archive"
	"github.com/portainer/portainer/http/client"
)

type pluginUpdatePayload struct {
	Version string
}

func (payload *pluginUpdatePayload) Validate(r *http.Request) error {
	if govalidator.IsNull(payload.Version) {
		return portainer.Error("Invalid plugin version")
	}

	return nil
}

func (handler *Handler) pluginUpdate(w http.ResponseWriter, r *http.Request) *httperror.HandlerError {
	pluginIdentifier, err := request.RetrieveNumericRouteVariableValue(r, "id")
	if err != nil {
		return &httperror.HandlerError{http.StatusBadRequest, "Invalid plugin identifier route variable", err}
	}
	pluginID := portainer.PluginID(pluginIdentifier)

	var payload pluginUpdatePayload
	err = request.DecodeAndValidateJSONPayload(r, &payload)
	if err != nil {
		return &httperror.HandlerError{http.StatusBadRequest, "Invalid request payload", err}
	}

	plugin, err := handler.PluginService.Plugin(pluginID)
	if err == portainer.ErrObjectNotFound {
		return &httperror.HandlerError{http.StatusNotFound, "Unable to find a plugin with the specified identifier inside the database", err}
	} else if err != nil {
		return &httperror.HandlerError{http.StatusInternalServerError, "Unable to find a plugin with the specified identifier inside the database", err}
	}

	// TODO: remove existing plugin and upgrade to the new version

	err = handler.PluginService.Persist(plugin)
	if err != nil {
		return &httperror.HandlerError{http.StatusInternalServerError, "Unable to persist plugin status inside the database", err}
	}

	return response.Empty(w)
}

func (handler *Handler) updatePlugin(plugin *portainer.Plugin) error {

	// TODO: switch case on plugin identifier to download/enable correct plugin
	switch plugin.ID {
	case portainer.RegistryManagementPlugin:
		return handler.updateRegistryManagementPlugin(plugin)
	default:
		return errors.New("Unsupported plugin identifier")
	}

	// syscall.Exec replaces the process, ForkExec could be tried?
	// Also should be relocated to another package
	// err = syscall.ForkExec("/plugins/plugin-registry-management", []string{"plugin-registry-management"}, os.Environ())
	// cmd := exec.Command("/data/bin/plugin-registry-management")
	// // cmd.Start will not share logs with the main Portainer container.
	// err := cmd.Start()
	// if err != nil {
	// 	return err
	// }

	return nil
}

func (handler *Handler) updateRegistryManagementPlugin(plugin *portainer.Plugin) error {
	// Download/untar
	// TODO: replace location + constant for base (download.portainer.io ?)
	// based on current platform+arch, should download the according zip (plugin-registry-management-linux-amd64-1.0.1.zip)
	data, err := client.Get("https://github.com/deviantony/xtrabackup-scripts/releases/download/3.1.5/rm01.zip", 30)
	if err != nil {
		return err
	}

	// TODO: shoudd be relocated to another package, also use data folder constant (windows/linux differs)
	err = archive.UnzipArchive(data, "/data/bin")
	if err != nil {
		return err
	}

	// TODO: if license check fails, need to be updated to use flags
	// should probably download and use a specific license-checker binary

	licenseValidationCommand := exec.Command("/data/bin/plugin-registry-management", license, "--check")
	cmdOutput := &bytes.Buffer{}
	licenseValidationCommand.Stdout = cmdOutput

	err = licenseValidationCommand.Run()
	if err != nil {
		return portainer.Error("Invalid license")
	}

	output := string(cmdOutput.Bytes())
	licenseDetails := strings.Split(output, "|")
	plugin.LicenseCompany = licenseDetails[0]
	plugin.LicenseExpiration = licenseDetails[1]
	plugin.Version = licenseDetails[2]

	// syscall.Exec replaces the process, ForkExec could be tried?
	// Also should be relocated to another package
	// err = syscall.ForkExec("/plugins/plugin-registry-management", []string{"plugin-registry-management"}, os.Environ())
	cmd := exec.Command("/data/bin/plugin-registry-management", license)

	// cmd.Start will not share logs with the main Portainer container.
	err = cmd.Start()
	if err != nil {
		return err
	}

	return handler.ProxyManager.CreatePluginProxy(plugin.ID)
}
