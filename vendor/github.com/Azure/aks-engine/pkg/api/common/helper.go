// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT license.

package common

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
	validator "gopkg.in/go-playground/validator.v9"
)

var (
	/* If a new GPU sku becomes available, add a key to this map, but only if you have a confirmation
	   that we have an agreement with NVIDIA for this specific gpu.
	*/
	NvidiaEnabledSKUs = map[string]bool{
		// K80
		"Standard_NC6":   true,
		"Standard_NC12":  true,
		"Standard_NC24":  true,
		"Standard_NC24r": true,
		// M60
		"Standard_NV6":      true,
		"Standard_NV12":     true,
		"Standard_NV12s_v3": true,
		"Standard_NV24":     true,
		"Standard_NV24s_v3": true,
		"Standard_NV24r":    true,
		"Standard_NV48s_v3": true,
		// P40
		"Standard_ND6s":   true,
		"Standard_ND12s":  true,
		"Standard_ND24s":  true,
		"Standard_ND24rs": true,
		// P100
		"Standard_NC6s_v2":   true,
		"Standard_NC12s_v2":  true,
		"Standard_NC24s_v2":  true,
		"Standard_NC24rs_v2": true,
		// V100
		"Standard_NC6s_v3":   true,
		"Standard_NC12s_v3":  true,
		"Standard_NC24s_v3":  true,
		"Standard_NC24rs_v3": true,
		"Standard_ND40s_v3":  true,
		"Standard_ND40rs_v2": true,
		// T4 (still in preview by Sep2020)
		"Standard_NC4as_T4_v3":  true,
		"Standard_NC8as_T4_v3":  true,
		"Standard_NC16as_T4_v3": true,
		"Standard_NC64as_T4_v3": true,
	}
)

// HandleValidationErrors is the helper function to catch validator.ValidationError
// based on Namespace of the error, and return customized error message.
func HandleValidationErrors(e validator.ValidationErrors) error {
	err := e[0]
	ns := err.Namespace()
	switch ns {
	case "Properties.OrchestratorProfile", "Properties.OrchestratorProfile.OrchestratorType",
		"Properties.MasterProfile", "Properties.MasterProfile.DNSPrefix", "Properties.MasterProfile.VMSize",
		"Properties.LinuxProfile", "Properties.ServicePrincipalProfile.ClientID",
		"Properties.WindowsProfile.AdminUsername",
		"Properties.WindowsProfile.AdminPassword":
		return errors.Errorf("missing %s", ns)
	case "Properties.MasterProfile.Count":
		return errors.New("MasterProfile count needs to be 1, 3, or 5")
	case "Properties.MasterProfile.OSDiskSizeGB":
		return errors.Errorf("Invalid os disk size of %d specified.  The range of valid values are [%d, %d]", err.Value().(int), MinDiskSizeGB, MaxDiskSizeGB)
	case "Properties.MasterProfile.IPAddressCount":
		return errors.Errorf("MasterProfile.IPAddressCount needs to be in the range [%d,%d]", MinIPAddressCount, MaxIPAddressCount)
	case "Properties.MasterProfile.StorageProfile":
		return errors.Errorf("Unknown storageProfile '%s'. Specify either %s or %s", err.Value().(string), StorageAccount, ManagedDisks)
	default:
		if strings.HasPrefix(ns, "Properties.AgentPoolProfiles") {
			switch {
			case strings.HasSuffix(ns, ".Name") || strings.HasSuffix(ns, "VMSize"):
				return errors.Errorf("missing %s", ns)
			case strings.HasSuffix(ns, ".Count"):
				return errors.Errorf("AgentPoolProfile count needs to be in the range [%d,%d]", MinAgentCount, MaxAgentCount)
			case strings.HasSuffix(ns, ".OSDiskSizeGB"):
				return errors.Errorf("Invalid os disk size of %d specified.  The range of valid values are [%d, %d]", err.Value().(int), MinDiskSizeGB, MaxDiskSizeGB)
			case strings.Contains(ns, ".Ports"):
				return errors.Errorf("AgentPoolProfile Ports must be in the range[%d, %d]", MinPort, MaxPort)
			case strings.HasSuffix(ns, ".StorageProfile"):
				return errors.Errorf("Unknown storageProfile '%s'. Specify %s, %s, or %s", err.Value().(string), StorageAccount, ManagedDisks, Ephemeral)
			case strings.Contains(ns, ".DiskSizesGB"):
				return errors.Errorf("A maximum of %d disks may be specified, The range of valid disk size values are [%d, %d]", MaxDisks, MinDiskSizeGB, MaxDiskSizeGB)
			case strings.HasSuffix(ns, ".IPAddressCount"):
				return errors.Errorf("AgentPoolProfile.IPAddressCount needs to be in the range [%d,%d]", MinIPAddressCount, MaxIPAddressCount)
			default:
				break
			}
		}
	}
	return errors.Errorf("Namespace %s is not caught, %+v", ns, e)
}

// ValidateDNSPrefix is a helper function to check that a DNS Prefix is valid
func ValidateDNSPrefix(dnsName string) error {
	dnsNameRegex := `^([A-Za-z][A-Za-z0-9-]{1,43}[A-Za-z0-9])$`
	re, err := regexp.Compile(dnsNameRegex)
	if err != nil {
		return err
	}
	if !re.MatchString(dnsName) {
		return errors.Errorf("DNSPrefix '%s' is invalid. The DNSPrefix must contain between 3 and 45 characters and can contain only letters, numbers, and hyphens.  It must start with a letter and must end with a letter or a number. (length was %d)", dnsName, len(dnsName))
	}
	return nil
}

// IsSgxEnabledSKU determines if an VM SKU has SGX driver support
func IsSgxEnabledSKU(vmSize string) bool {
	switch vmSize {
	case "Standard_DC2s", "Standard_DC4s":
		return true
	}
	return false
}

// GetStorageAccountType returns the support managed disk storage tier for a give VM size
func GetStorageAccountType(sizeName string) (string, error) {
	spl := strings.Split(sizeName, "_")
	if len(spl) < 2 {
		return "", errors.Errorf("Invalid sizeName: %s", sizeName)
	}
	capability := spl[1]
	if strings.Contains(strings.ToLower(capability), "s") {
		return "Premium_LRS", nil
	}
	return "Standard_LRS", nil
}

// GetOrderedEscapedKeyValsString returns an ordered string of escaped, quoted key=val
func GetOrderedEscapedKeyValsString(config map[string]string) string {
	keys := []string{}
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, key := range keys {
		buf.WriteString(fmt.Sprintf("\"%s=%s\", ", key, config[key]))
	}
	return strings.TrimSuffix(buf.String(), ", ")
}

// SliceIntIsNonEmpty is a simple convenience to determine if a []int is non-empty
func SliceIntIsNonEmpty(s []int) bool {
	return len(s) > 0
}

// WrapAsVerbatim formats a string for inserting a literal string into an ARM expression
func WrapAsVerbatim(s string) string {
	return fmt.Sprintf("',%s,'", s)
}

// GetDockerConfig transforms the default docker config with overrides. Overrides may be nil.
func GetDockerConfig(opts map[string]string, overrides []func(*DockerConfig) error) (string, error) {
	config := GetDefaultDockerConfig()

	for i := range overrides {
		if err := overrides[i](&config); err != nil {
			return "", err
		}
	}

	dataDir, ok := opts[ContainerDataDirKey]
	if ok {
		config.DataRoot = dataDir
	}

	b, err := json.MarshalIndent(config, "", "    ")
	return string(b), err
}

// GetContainerdConfig transforms the default containerd config with overrides. Overrides may be nil.
func GetContainerdConfig(opts map[string]string, overrides []func(*ContainerdConfig) error) (string, error) {
	config := GetDefaultContainerdConfig()

	for i := range overrides {
		if err := overrides[i](&config); err != nil {
			return "", err
		}
	}

	dataDir, ok := opts[ContainerDataDirKey]
	if ok {
		config.Root = dataDir
	}

	buf := new(bytes.Buffer)
	err := toml.NewEncoder(buf).Encode(config)
	return buf.String(), err
}

// ContainerdKubenetOverride transforms a containerd config to set details required when using kubenet.
func ContainerdKubenetOverride(config *ContainerdConfig) error {
	config.Plugins.IoContainerdGrpcV1Cri.CNI.ConfTemplate = "/etc/containerd/kubenet_template.conf"
	return nil
}

// ContainerdSandboxImageOverrider produces a function to transform containerd config by setting the SandboxImage.
func ContainerdSandboxImageOverrider(image string) func(*ContainerdConfig) error {
	return func(config *ContainerdConfig) error {
		config.Plugins.IoContainerdGrpcV1Cri.SandboxImage = image
		return nil
	}
}

// DockerNvidiaOverride transforms a docker config to supply nvidia runtime configuration.
func DockerNvidiaOverride(config *DockerConfig) error {
	if config.DockerDaemonRuntimes == nil {
		config.DockerDaemonRuntimes = make(map[string]DockerDaemonRuntime)
	}
	config.DefaultRuntime = "nvidia"
	config.DockerDaemonRuntimes["nvidia"] = DockerDaemonRuntime{
		Path:        "/usr/bin/nvidia-container-runtime",
		RuntimeArgs: []string{},
	}
	return nil
}

// IndentString pads each line of an original string with N spaces and returns the new value.
func IndentString(original string, spaces int) string {
	out := bytes.NewBuffer(nil)
	scanner := bufio.NewScanner(strings.NewReader(original))
	for scanner.Scan() {
		for i := 0; i < spaces; i++ {
			out.WriteString(" ")
		}
		out.WriteString(scanner.Text())
		out.WriteString("\n")
	}
	return out.String()
}

// IsNvidiaEnabledSKU determines if an VM SKU has nvidia driver support
func IsNvidiaEnabledSKU(vmSize string) bool {
	// Trim the optional _Promo suffix.
	vmSize = strings.TrimSuffix(vmSize, "_Promo")
	if _, ok := NvidiaEnabledSKUs[vmSize]; ok {
		return NvidiaEnabledSKUs[vmSize]
	}

	return false
}
