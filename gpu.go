// Copyright (c) 2022, Cogent Core. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This is initially adapted from https://github.com/vulkan-go/asche
// Copyright © 2017 Maxim Kupriianov <max@kc.vc>, under the MIT License

package vgpu

//go:generate core generate

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"unsafe"

	"log/slog"

	"cogentcore.org/core/base/reflectx"
	vk "github.com/goki/vulkan"
	"github.com/tomas-mraz/vgpu/vkinit"
)

// Key docs: https://gpuopen.com/learn/understanding-vulkan-objects/

// Debug is a global flag for turning on debug mode
// Set to true prior to GPU.Config to get validation debugging.
var Debug = false

// DefaultOpts are default GPU config options that can be set by any app
// prior to initializing the GPU object -- this may be easier than passing
// options in from the app during the Config call.  Any such options take
// precedence over these options (usually best to avoid direct conflits --
// monitor Debug output to see).
var DefaultOpts *GPUOpts

// GPU represents the GPU hardware
type GPU struct {

	// handle for the vulkan driver instance
	Instance vk.Instance

	// handle for the vulkan physical GPU hardware
	GPU vk.PhysicalDevice

	// options passed in during config
	UserOpts *GPUOpts

	// set of enabled options set post-Config
	EnabledOpts GPUOpts

	// name of the physical GPU device
	DeviceName string

	// name of application -- set during Config and used in init of GPU
	AppName string

	// version of vulkan API to target
	APIVersion vk.Version

	// version of application -- optional
	AppVersion vk.Version

	// use Add method to add required instance extentions prior to calling Config
	InstanceExts []string

	// use Add method to add required device extentions prior to calling Config
	DeviceExts []string

	// set Add method to add required validation layers prior to calling Config
	ValidationLayers []string

	// physical device features required -- set per platform as needed
	DeviceFeaturesNeeded *vk.PhysicalDeviceVulkan12Features

	// this is used for computing, not graphics
	Compute bool

	// our custom debug callback
	DebugCallback vk.DebugReportCallback

	// properties of physical hardware -- populated after Config
	GPUProperties vk.PhysicalDeviceProperties

	// features of physical hardware -- populated after Config
	GPUFeats vk.PhysicalDeviceFeatures

	// properties of device memory -- populated after Config
	MemoryProperties vk.PhysicalDeviceMemoryProperties

	// maximum number of compute threads per compute shader invokation, for a 1D number of threads per Warp, which is generally greater than MaxComputeWorkGroup, which allows for the and maxima as well.  This is not defined anywhere in the formal spec, unfortunately, but has been determined empirically for Mac and NVIDIA which are two of the most relevant use-cases.  If not a known case, the MaxComputeWorkGroupvalue is used, which can significantly slow down compute processing if more could actually be used.  Please file an issue or PR for other GPUs with known larger values.
	MaxComputeWorkGroupCount1D int
}

// InitNoDisplay initializes vulkan system for a purely compute-based
// or headless operation, without any display (i.e., without using glfw).
// Call before doing any vgpu stuff.
// Loads the vulkan library and sets the Vulkan instance proc addr and calls Init.
// IMPORTANT: must be called on the main initial thread!
func InitNoDisplay() error {

	err := vkinit.LoadVulkan()
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

// Defaults sets up default parameters, with the graphics flag
// determining whether graphics-relevant items are added.
func (gp *GPU) Defaults(graphics bool) {
	gp.APIVersion = vk.Version(vk.MakeVersion(1, 2, 0))
	gp.AppVersion = vk.Version(vk.MakeVersion(1, 0, 0))
	// TODO: these don't work on mobile, but might be needed on desktop
	// gp.DeviceExts = []string{"VK_EXT_descriptor_indexing"}
	// gp.InstanceExts = []string{"VK_KHR_get_physical_device_properties2"}
	if graphics {
		gp.DeviceExts = append(gp.DeviceExts, []string{"VK_KHR_swapchain"}...)
	} else {
		gp.Compute = true
	}
	PlatformDefaults(gp)
}

// NewGPU returns a new GPU struct with Graphics Defaults set
// configure any additional defaults before calling Config.
// Use NewComputeGPU for a compute-only GPU that doesn't load graphics extensions.
func NewGPU() *GPU {
	gp := &GPU{}
	gp.Defaults(true)
	return gp
}

// NewComputeGPU returns a new GPU struct with Compute Defaults set
// configure any additional defaults before calling Config.
// Use NewGPU for a graphics enabled GPU.
func NewComputeGPU() *GPU {
	gp := &GPU{}
	gp.Defaults(false)
	return gp
}

// FindString returns index of string if in list, else -1
func FindString(str string, strs []string) int {
	for i, s := range strs {
		if str == s {
			return i
		}
	}
	return -1
}

// AddInstanceExt adds given extension(s), only if not already set
// returns true if added.
func (gp *GPU) AddInstanceExt(ext ...string) bool {
	for _, ex := range ext {
		i := FindString(ex, gp.InstanceExts)
		if i >= 0 {
			continue
		}
		gp.InstanceExts = append(gp.InstanceExts, ex)
	}
	return true
}

// AddDeviceExt adds given extension(s), only if not already set
// returns true if added.
func (gp *GPU) AddDeviceExt(ext ...string) bool {
	for _, ex := range ext {
		i := FindString(ex, gp.DeviceExts)
		if i >= 0 {
			continue
		}
		gp.DeviceExts = append(gp.DeviceExts, ex)
	}
	return true
}

// AddValidationLayer adds given validation layer, only if not already set
// returns true if added.
func (gp *GPU) AddValidationLayer(ext string) bool {
	i := FindString(ext, gp.ValidationLayers)
	if i >= 0 {
		return false
	}
	gp.ValidationLayers = append(gp.ValidationLayers, ext)
	return true
}

// Config configures the GPU given the extensions set in InstanceExts,
// DeviceExts, and ValidationLayers, and the given GPUOpts options.
// Only the first such opts will be used -- the variable args is used to enable
// no options to be passed by default.
func (gp *GPU) Config(name string, opts ...*GPUOpts) error {
	gp.AppName = name
	gp.UserOpts = DefaultOpts
	if len(opts) > 0 {
		if gp.UserOpts == nil {
			gp.UserOpts = opts[0]
		} else {
			gp.UserOpts.CopyFrom(opts[0])
		}
	}
	if Debug {
		gp.AddValidationLayer("VK_LAYER_KHRONOS_validation")
		gp.AddInstanceExt("VK_EXT_debug_report") // note _utils is not avail yet
	}

	// Select instance extensions
	requiredInstanceExts := SafeStrings(gp.InstanceExts)
	actualInstanceExts, err := InstanceExts()
	IfPanic(err)
	instanceExts, missing := CheckExisting(actualInstanceExts, requiredInstanceExts)
	if missing > 0 {
		log.Println("vgpu: warning: missing", missing, "required instance extensions during Config")
	}
	if Debug {
		log.Printf("vgpu: enabling %d instance extensions", len(instanceExts))
	}

	// Select instance layers
	var validationLayers []string
	if len(gp.ValidationLayers) > 0 {
		requiredValidationLayers := SafeStrings(gp.ValidationLayers)
		actualValidationLayers, err := ValidationLayers()
		IfPanic(err)
		validationLayers, missing = CheckExisting(actualValidationLayers, requiredValidationLayers)
		if missing > 0 {
			log.Println("vgpu: warning: missing", missing, "required validation layers during Config")
		}
	}

	// Create instance
	var instance vk.Instance
	ret := vk.CreateInstance(&vk.InstanceCreateInfo{
		SType: vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo: &vk.ApplicationInfo{
			SType:              vk.StructureTypeApplicationInfo,
			ApiVersion:         uint32(gp.APIVersion),
			ApplicationVersion: uint32(gp.AppVersion),
			PApplicationName:   SafeString(gp.AppName),
			PEngineName:        "vgpu\x00",
		},
		EnabledExtensionCount:   uint32(len(instanceExts)),
		PpEnabledExtensionNames: instanceExts,
		EnabledLayerCount:       uint32(len(validationLayers)),
		PpEnabledLayerNames:     validationLayers,
		Flags:                   vk.InstanceCreateFlags(vk.InstanceCreateEnumeratePortabilityBit),
	}, nil, &instance)
	IfPanic(NewError(ret))
	gp.Instance = instance

	vk.InitInstance(instance)

	// Find a suitable GPU
	var gpuCountU uint32
	ret = vk.EnumeratePhysicalDevices(gp.Instance, &gpuCountU, nil)
	IfPanic(NewError(ret))
	if gpuCountU == 0 {
		return errors.New("vgpu: error: no GPU devices found")
	}
	gpuCount := int(gpuCountU)
	gpus := make([]vk.PhysicalDevice, gpuCount)
	ret = vk.EnumeratePhysicalDevices(gp.Instance, &gpuCountU, gpus)
	IfPanic(NewError(ret))

	gpIndex := gp.SelectGPU(gpus, gpuCount)
	gp.GPU = gpus[gpIndex]

	vk.GetPhysicalDeviceFeatures(gp.GPU, &gp.GPUFeats)
	gp.GPUFeats.Deref()
	if !gp.CheckGPUOpts(&gp.GPUFeats, gp.UserOpts, true) {
		return errors.New("vgpu: fatal config error found, see messages above")
	}

	vk.GetPhysicalDeviceProperties(gp.GPU, &gp.GPUProperties)
	gp.GPUProperties.Deref()
	gp.GPUProperties.Limits.Deref()
	vk.GetPhysicalDeviceMemoryProperties(gp.GPU, &gp.MemoryProperties)
	gp.MemoryProperties.Deref()

	gp.MaxComputeWorkGroupCount1D = int(gp.GPUProperties.Limits.MaxComputeWorkGroupCount[0])
	// note: unclear what the limit is here.
	// if gp.MaxComputeWorkGroupCount1D == 0 { // otherwise set per-platform in defaults (DARWIN)
	// if strings.Contains(gp.DeviceName, "NVIDIA") {
	// 	// according to: https://vulkan.gpuinfo.org/displaydevicelimit.php?name=maxComputeWorkGroupInvocations&platform=all
	// 	// all NVIDIA are either 1 << 31 or -1 of that.
	// 	gp.MaxComputeWorkGroupCount1D = (1 << 31) - 1 // according to vgpu
	// } else {
	// note: if known to be higher for any specific case, please file an issue or PR
	// }
	// }

	// Select device extensions
	requiredDeviceExts := SafeStrings(gp.DeviceExts)
	actualDeviceExts, err := DeviceExts(gp.GPU)
	IfPanic(err)
	deviceExts, missing := CheckExisting(actualDeviceExts, requiredDeviceExts)
	if missing > 0 {
		log.Println("vgpu: warning: missing", missing, "required device extensions during Config")
	}
	if Debug {
		log.Printf("vgpu: enabling %d device extensions", len(deviceExts))
	}

	if Debug {
		var debugCallback vk.DebugReportCallback
		// Register a debug callback
		ret := vk.CreateDebugReportCallback(gp.Instance, &vk.DebugReportCallbackCreateInfo{
			SType:       vk.StructureTypeDebugReportCallbackCreateInfo,
			Flags:       vk.DebugReportFlags(vk.DebugReportErrorBit | vk.DebugReportWarningBit | vk.DebugReportInformationBit),
			PfnCallback: dbgCallbackFunc,
		}, nil, &debugCallback)
		IfPanic(NewError(ret))
		log.Println("vgpu: DebugReportCallback enabled by application")
		gp.DebugCallback = debugCallback
	}

	return nil
}

func (gp *GPU) GetDeviceName(properties *vk.PhysicalDeviceProperties, idx int) string {
	nm := CleanString(string(properties.DeviceName[:]))
	return fmt.Sprintf("%s: id=%d idx=%d", nm, properties.DeviceID, idx)
}

func (gp *GPU) SelectGPU(gpus []vk.PhysicalDevice, gpuCount int) int {
	if gpuCount == 1 {
		var properties vk.PhysicalDeviceProperties
		vk.GetPhysicalDeviceProperties(gpus[0], &properties)
		properties.Deref()
		gp.DeviceName = gp.GetDeviceName(&properties, 0)
		if Debug {
			log.Printf("vgpu: selected only device named: %s\n", gp.DeviceName)
		}
		return 0
	}
	trgDevNm := ""
	if ev := os.Getenv("MESA_VK_DEVICE_SELECT"); ev != "" {
		trgDevNm = ev
	} else if ev := os.Getenv("VK_DEVICE_SELECT"); ev != "" {
		trgDevNm = ev
	}
	if gp.Compute {
		if ev := os.Getenv("VK_COMPUTE_DEVICE_SELECT"); ev != "" {
			trgDevNm = ev
		}
	}

	if trgDevNm != "" {
		idx, err := strconv.Atoi(trgDevNm)
		if err == nil && idx >= 0 && idx < gpuCount {
			curIndex := 0
			for gi := 0; gi < gpuCount; gi++ {
				var properties vk.PhysicalDeviceProperties
				vk.GetPhysicalDeviceProperties(gpus[gi], &properties)
				properties.Deref()
				if properties.DeviceType == vk.PhysicalDeviceTypeDiscreteGpu {
					if curIndex == idx {
						gp.DeviceName = gp.GetDeviceName(&properties, gi)
						if Debug {
							log.Printf("vgpu: selected device named: %s, specified by index in *_DEVICE_SELECT environment variable, index: %d\n", gp.DeviceName, gi)
						}
						return gi
					} else {
						curIndex++
					}
				}
			}
			panic(fmt.Sprintf("vgpu: device specified by index in *_DEVICE_SELECT environment variable, index: %d, NOT FOUND\n", idx))
		}
		for gi := 0; gi < gpuCount; gi++ {
			var properties vk.PhysicalDeviceProperties
			vk.GetPhysicalDeviceProperties(gpus[gi], &properties)
			properties.Deref()
			if bytes.Contains(properties.DeviceName[:], []byte(trgDevNm)) {
				devNm := gp.GetDeviceName(&properties, gi)
				if Debug {
					log.Printf("vgpu: selected device named: %s, specified in *_DEVICE_SELECT environment variable, index: %d\n", devNm, gi)
				}
				gp.DeviceName = devNm
				return gi
			}
		}
		if Debug {
			log.Printf("vgpu: unable to find device named: %s, specified in *_DEVICE_SELECT environment variable\n", trgDevNm)
		}
	}

	devNm := ""
	maxSz := 0
	maxIndex := 0
	for gi := 0; gi < gpuCount; gi++ {
		// note: we could potentially check for the optional features here
		// but generally speaking the discrete device is going to be the most
		// feature-full, so the practical benefit is unlikely to be significant.
		var properties vk.PhysicalDeviceProperties
		vk.GetPhysicalDeviceProperties(gpus[gi], &properties)
		properties.Deref()
		dnm := gp.GetDeviceName(&properties, gi)
		if properties.DeviceType == vk.PhysicalDeviceTypeDiscreteGpu {
			var memProperties vk.PhysicalDeviceMemoryProperties
			vk.GetPhysicalDeviceMemoryProperties(gpus[gi], &memProperties)
			memProperties.Deref()
			if Debug {
				log.Printf("vgpu: %d: evaluating discrete device named: %s\n", gi, dnm)
			}
			for mi := uint32(0); mi < memProperties.MemoryHeapCount; mi++ {
				heap := &memProperties.MemoryHeaps[mi]
				heap.Deref()
				// if heap.Flags&vk.MemoryHeapFlags(vk.MemoryHeapDeviceLocalBit) != 0 {
				sz := int(heap.Size)
				if sz > maxSz {
					devNm = gp.GetDeviceName(&properties, gi)
					maxSz = sz
					maxIndex = gi
				}
				// }
			}
		} else {
			if Debug {
				log.Printf("vgpu: %d: skipping device named: %s -- not discrete\n", gi, dnm)
			}
		}
	}
	gp.DeviceName = devNm
	if Debug {
		log.Printf("vgpu: %d selected device named: %s, memory size: %d\n", maxIndex, devNm, maxSz)
	}

	return maxIndex
}

// Destroy destroys GPU resources -- call after everything else has been destroyed
func (gp *GPU) Destroy() {
	if gp.DebugCallback != vk.NullDebugReportCallback {
		vk.DestroyDebugReportCallback(gp.Instance, gp.DebugCallback, nil)
	}
	if gp.Instance != nil {
		vk.DestroyInstance(gp.Instance, nil)
		gp.Instance = nil
	}
}

// NewComputeSystem returns a new system initialized for this GPU,
// for Compute, not graphics functionality.
func (gp *GPU) NewComputeSystem(name string) *System {
	sy := &System{}
	sy.InitCompute(gp, name)
	return sy
}

// NewGraphicsSystem returns a new system initialized for this GPU,
// for graphics functionality, using Device from the Surface or
// RenderFrame depending on the target of rendering.
func (gp *GPU) NewGraphicsSystem(name string, dev *Device) *System {
	sy := &System{}
	sy.InitGraphics(gp, name, dev)
	return sy
}

// PropertiesString returns a human-readable summary of the GPU properties.
func (gp *GPU) PropertiesString(print bool) string {
	ps := "\n\n######## GPU Properties\n"
	prs := reflectx.StringJSON(&gp.GPUProperties)
	devnm := `  "DeviceName": `
	ps += prs[:strings.Index(prs, devnm)]
	ps += devnm + string(gp.GPUProperties.DeviceName[:]) + "\n"
	ps += prs[strings.Index(prs, `  "Limits":`):]
	// ps += "\n\n######## GPU Memory Properties\n" // not really useful
	// ps += reflectx.StringJSON(&gp.MemoryProperties)
	ps += "\n"
	if print {
		fmt.Println(ps)
	}
	return ps
}

func dbgCallbackFunc(flags vk.DebugReportFlags, objectType vk.DebugReportObjectType,
	object uint64, location uint64, messageCode int32, pLayerPrefix string,
	pMessage string, pUserData unsafe.Pointer) vk.Bool32 {

	switch {
	case flags&vk.DebugReportFlags(vk.DebugReportInformationBit) != 0:
		if !(strings.Contains(pLayerPrefix, "Loader") && strings.Contains(pMessage, "Device Extension")) {
			slog.Info("["+pLayerPrefix+"]", "Code", messageCode, "Message", pMessage)
		}
	case flags&vk.DebugReportFlags(vk.DebugReportWarningBit) != 0:
		slog.Warn("["+pLayerPrefix+"]", "Code", messageCode, "Message", pMessage)
	case flags&vk.DebugReportFlags(vk.DebugReportPerformanceWarningBit) != 0:
		slog.Warn("PERFORMANCE: ["+pLayerPrefix+"]", "Code", messageCode, "Message", pMessage)
	case flags&vk.DebugReportFlags(vk.DebugReportErrorBit) != 0:
		slog.Error("["+pLayerPrefix+"]", "Code", messageCode, "Message", pMessage)
	case flags&vk.DebugReportFlags(vk.DebugReportDebugBit) != 0:
		slog.Debug("["+pLayerPrefix+"]", "Code", messageCode, "Message", pMessage)
	default:
		if !(strings.Contains(pLayerPrefix, "Loader") && strings.Contains(pMessage, "Device Extension")) {
			slog.Info("["+pLayerPrefix+"]", "Code", messageCode, "Message", pMessage)
		}
	}
	return vk.Bool32(vk.False)
}

// InstanceExts gets a list of instance extensions available on the platform.
func InstanceExts() (names []string, err error) {
	defer CheckErr(&err)

	var count uint32
	ret := vk.EnumerateInstanceExtensionProperties("", &count, nil)
	IfPanic(NewError(ret))
	list := make([]vk.ExtensionProperties, count)
	ret = vk.EnumerateInstanceExtensionProperties("", &count, list)
	IfPanic(NewError(ret))
	for _, ext := range list {
		ext.Deref()
		names = append(names, vk.ToString(ext.ExtensionName[:]))
	}
	return names, err
}

// DeviceExts gets a list of instance extensions available on the provided physical device.
func DeviceExts(gpu vk.PhysicalDevice) (names []string, err error) {
	defer CheckErr(&err)

	var count uint32
	ret := vk.EnumerateDeviceExtensionProperties(gpu, "", &count, nil)
	IfPanic(NewError(ret))
	list := make([]vk.ExtensionProperties, count)
	ret = vk.EnumerateDeviceExtensionProperties(gpu, "", &count, list)
	IfPanic(NewError(ret))
	for _, ext := range list {
		ext.Deref()
		names = append(names, vk.ToString(ext.ExtensionName[:]))
	}
	return names, err
}

// ValidationLayers gets a list of validation layers available on the platform.
func ValidationLayers() (names []string, err error) {
	defer CheckErr(&err)

	var count uint32
	ret := vk.EnumerateInstanceLayerProperties(&count, nil)
	IfPanic(NewError(ret))
	list := make([]vk.LayerProperties, count)
	ret = vk.EnumerateInstanceLayerProperties(&count, list)
	IfPanic(NewError(ret))
	for _, layer := range list {
		layer.Deref()
		names = append(names, vk.ToString(layer.LayerName[:]))
	}
	return names, err
}

// NoDisplayGPU Initializes the Vulkan GPU and returns that
// and the graphics GPU device, with given name, without connecting
// to the display.
func NoDisplayGPU(nm string) (*GPU, *Device, error) {
	if err := InitNoDisplay(); err != nil {
		return nil, nil, err
	}
	gp := NewGPU()
	if err := gp.Config(nm, nil); err != nil {
		return nil, nil, err
	}
	dev, err := NewGraphicsDevice(gp)
	return gp, dev, err
}
