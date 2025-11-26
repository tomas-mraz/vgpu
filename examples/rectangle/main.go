package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"runtime"
	"time"

	vk "github.com/goki/vulkan"

	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/tomas-mraz/vgpu"
	"github.com/tomas-mraz/vgpu/vdraw"
)

func init() {
	// must lock the main thread for gpu!
	runtime.LockOSThread()
}

func main() {
	glfw.Init()
	vk.SetGetInstanceProcAddr(glfw.GetVulkanGetInstanceProcAddress())
	vk.Init()

	glfw.WindowHint(glfw.ClientAPI, glfw.NoAPI)
	glfw.WindowHint(glfw.Resizable, glfw.False)
	window, err := glfw.CreateWindow(1024, 768, "vDraw Test", nil, nil)
	vgpu.IfPanic(err)

	// note: for graphics, require these instance extensions before init gpu!
	instanceExtensions := window.GetRequiredInstanceExtensions()
	gpu := vgpu.NewGPU()
	gpu.AddInstanceExt(instanceExtensions...)
	vgpu.Debug = true // add console output
	err = gpu.Config("vDraw test")
	vgpu.IfPanic(err)

	surfacePtr, err := window.CreateWindowSurface(gpu.Instance, nil)
	vgpu.IfPanic(err)

	surface := vgpu.NewSurface(gpu, vk.SurfaceFromPointer(surfacePtr))
	fmt.Printf("format: %s\n", surface.Format.String())

	drw := &vdraw.Drawer{}
	drw.YIsDown = true
	drw.ConfigSurface(surface, 16) // requires 2 NDesc

	destroy := func() {
		vk.DeviceWaitIdle(surface.Device.Device)
		drw.Destroy()
		surface.Destroy()
		gpu.Destroy()
		window.Destroy()
		vgpu.Terminate()
	}

	//red := color.RGBA{255, 0, 0, 255}
	//green := color.RGBA{0, 255, 0, 255}
	blue := color.RGBA{0, 0, 255, 255}
	//color.White
	//color.Black

	fillRnd := func() {
		drw.StartFill()
		sp := image.Point{50, 50}
		sz := image.Point{100, 100}
		drw.FillRect(blue, image.Rectangle{Min: sp, Max: sp.Add(sz)}, draw.Src)
		drw.EndFill()
	}

	// start values
	frameCount := 0
	stTime := time.Now()

	renderFrame := func() {
		fillRnd()
		frameCount++
		eTime := time.Now()
		dur := float64(eTime.Sub(stTime)) / float64(time.Second) // počet vteřin v novém čekacím okně
		if dur > 10 {
			fps := float64(frameCount) / dur
			fmt.Printf("fps: %.0f\n", fps)
			frameCount = 0 // resetuje čítač
			stTime = eTime // nastaví nový čas od kterého se začne znovu počítat
		}
	}

	exitC := make(chan struct{}, 2)
	fpsDelay := time.Second / 60
	fpsTicker := time.NewTicker(fpsDelay)
	for {
		select {
		case <-exitC:
			fpsTicker.Stop()
			destroy()
			return
		case <-fpsTicker.C:
			if window.ShouldClose() {
				exitC <- struct{}{}
				continue
			}
			glfw.PollEvents()
			renderFrame()
		}
	}
}
