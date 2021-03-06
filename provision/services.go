package provision

import (
	"code.cloudfoundry.org/cfdev/config"
	"code.cloudfoundry.org/cfdev/runner"
	"code.cloudfoundry.org/cfdev/workspace"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func (c *Controller) WhiteListServices(whiteList string, services []workspace.Service) ([]workspace.Service, error) {
	var whiteListed []workspace.Service

	for _, service := range services {
		if service.Flagname == "always-include" {
			whiteListed = append(whiteListed, service)
		}
	}

	switch strings.TrimSpace(strings.ToLower(whiteList)) {
	case "all":
		return services, nil
	case "", "none":
		return whiteListed, nil
	default:
		for _, service := range services {
			if strings.Contains(strings.ToLower(whiteList), strings.ToLower(service.Flagname)) && !contains(whiteListed, service.Name) {
				whiteListed = append(whiteListed, service)
			}
		}

		return whiteListed, nil
	}
}

func (c *Controller) GetWhiteListedService(serviceName string, whiteList []workspace.Service) (*workspace.Service, error) {
	for _, service := range whiteList {
		if strings.Contains(strings.ToLower(serviceName), strings.ToLower(service.Flagname)) {
			return &service, nil
		}
	}
	return nil, errors.New(fmt.Sprintf("The service '%s' is not a valid service", serviceName))
}

func contains(services []workspace.Service, name string) bool {
	for _, s := range services {
		if s.Name == name {
			return true
		}
	}

	return false
}

func (c *Controller) DeployServices(ui UI, services []workspace.Service, dockerRegistries []string) error {
	var (
		b       = NewBosh(runner.NewBosh(c.Config))
		errChan = make(chan error, 1)
	)

	for _, service := range services {
		start := time.Now()

		ui.Say("Deploying %s...", service.Name)

		go func(s workspace.Service) {
			errChan <- c.DeployService(s, dockerRegistries)
		}(service)

		err := c.report(start, ui, b, service, errChan)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Controller) DeployService(service workspace.Service, dockerRegistries []string) error {
	var cmd *exec.Cmd

	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell.exe", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(c.Config.ServicesDir, service.Script+".ps1"))
	} else {
		cmd = exec.Command(filepath.Join(c.Config.ServicesDir, service.Script))
	}

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, configEnvs(c.Config)...)
	cmd.Env = append(cmd.Env, c.Workspace.Envs()...)

	if strings.HasPrefix(service.Deployment, "cf") {
		cmd.Env = append(cmd.Env, dockerRegistriesAsEnvVar(dockerRegistries))
	}

	logFile, err := os.Create(filepath.Join(c.Config.LogDir, "deploy-"+strings.ToLower(service.Name)+".log"))
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	return cmd.Run()
}

func configEnvs(cfg config.Config) []string {
	return []string{
		"BINARY_DIR=" + cfg.BinaryDir,
		"BOSH_STATE=" + cfg.StateBosh,
		"CF_DOMAIN=" + cfg.CFDomain,
		"SERVICES_DIR=" + cfg.ServicesDir,
	}
}

func dockerRegistriesAsEnvVar(registries []string) string {
	var arr []string
	for _, registry := range registries {
		arr = append(arr, fmt.Sprintf(`%q`, registry))
	}

	return fmt.Sprintf(`DOCKER_REGISTRIES=[%s]`, strings.Join(arr, ","))
}
