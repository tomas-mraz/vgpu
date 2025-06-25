// Copyright (c) 2022, Cogent Core. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"math/rand"
	"runtime"

	"cogentcore.org/core/math32"
	"github.com/tomas-mraz/vgpu"
)

func init() {
	// must lock the main thread for gpu!
	runtime.LockOSThread()
}

func main() {
	if vgpu.InitNoDisplay() != nil {
		return
	}

	gp := vgpu.NewComputeGPU()
	vgpu.Debug = true
	gp.Config("compute1")
	fmt.Printf("Running on GPU: %s\n", gp.DeviceName)

	// gp.PropertiesString(true) // print

	sy := gp.NewComputeSystem("compute1")
	pl := sy.NewPipeline("compute1")
	pl.AddShaderFile("sqvecel", vgpu.ComputeShader, "sqvecel.spv")

	vars := sy.Vars()
	set := vars.AddSet()

	n := 20 // note: not necc to spec up-front, but easier if so

	threads := 64
	nInt := math32.IntMultiple(float32(n), float32(threads))
	n = int(nInt)       // enforce optimal n's -- otherwise requires range checking
	nGps := n / threads // dispatch n
	fmt.Printf("n: %d\n", n)

	inv := set.Add("In", vgpu.Float32Vector4, n, vgpu.Storage, vgpu.ComputeShader)
	outv := set.Add("Out", vgpu.Float32Vector4, n, vgpu.Storage, vgpu.ComputeShader)
	_ = outv

	set.ConfigValues(1) // one val per var
	sy.Config()         // configures vars, allocates vals, configs pipelines..

	ivl, _ := inv.Values.ValueByIndexTry(0)
	idat := ivl.Floats32()
	for i := 0; i < n; i++ {
		idat[i*4+0] = rand.Float32()
		idat[i*4+1] = rand.Float32()
		idat[i*4+2] = rand.Float32()
		idat[i*4+3] = rand.Float32()
	}
	ivl.SetMod()

	sy.Mem.SyncToGPU()

	vars.BindDynValuesAllIndex(0)

	cmd := sy.ComputeCmdBuff()
	sy.ComputeResetBindVars(cmd, 0)
	pl.ComputeDispatch(cmd, nGps, 1, 1)
	sy.ComputeCmdEnd(cmd)
	sy.ComputeSubmitWait(cmd) // if no wait, faster, but validation complains

	sy.Mem.SyncValueIndexFromGPU(0, "Out", 0)
	_, ovl, _ := vars.ValueByIndexTry(0, "Out", 0)

	odat := ovl.Floats32()
	for i := 0; i < n; i++ {
		fmt.Printf("In:  %d\tr: %g\tg: %g\tb: %g\ta: %g\n", i, idat[i*4+0], idat[i*4+1], idat[i*4+2], idat[i*4+3])
		fmt.Printf("Out: %d\tr: %g\tg: %g\tb: %g\ta: %g\n", i, odat[i*4+0], odat[i*4+1], odat[i*4+2], odat[i*4+3])
	}
	fmt.Printf("\n")

	sy.Destroy()
	gp.Destroy()
	vgpu.Terminate()
}
