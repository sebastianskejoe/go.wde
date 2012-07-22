/*
   Copyright 2012 the go.wde authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package wayland

import (
	"github.com/sebastianskejoe/gowl"
	"github.com/skelterjohn/go.wde"

	"image"
	"image/draw"

	"fmt"
	"strings"
	"syscall"
)

func init() {
	wde.BackendNewWindow = func(width, height int) (w wde.Window, err error) {
		w, err = NewWindow(width, height)
		return
	}

	// XXX Really does nothing
	ch := make(chan struct{}, 1)
	wde.BackendRun = func() {
		<-ch
	}
	wde.BackendStop = func() {
		ch <- struct{}{}
	}
}

type Window struct {
	display      *gowl.Display
	compositor   *gowl.Compositor
	surface      *gowl.Surface
	shell        *gowl.Shell
	shellsurface *gowl.ShellSurface
	shm          *gowl.Shm
	pool         *gowl.ShmPool
	buffer       *gowl.Buffer
	seat         *gowl.Seat
	pointer      *gowl.Pointer
	keyboard	 *gowl.Keyboard
	ddm          *gowl.DataDeviceManager
	dd           *gowl.DataDevice

	screen    *image.RGBA
	eventchan chan interface{}
}

func NewWindow(width, height int) (w *Window, err error) {
	w = new(Window)
	w.eventchan = make(chan interface{})

	// Create display and connect to wayland server
	w.display = gowl.NewDisplay()
	err = w.display.Connect()
	if err != nil {
		w = nil
		return
	}

	// Allocate other components
	w.compositor	= gowl.NewCompositor()
	w.surface		= gowl.NewSurface()
	w.shell			= gowl.NewShell()
	w.shellsurface	= gowl.NewShellSurface()

	w.shm		= gowl.NewShm()
	w.pool		= gowl.NewShmPool()
	w.buffer	= gowl.NewBuffer()

	w.seat		= gowl.NewSeat()
	w.pointer	= gowl.NewPointer()
	w.keyboard	= gowl.NewKeyboard()
	w.ddm		= gowl.NewDataDeviceManager()
	w.dd		= gowl.NewDataDevice()

	// Listen for global events from display
	globals := make(chan interface{})
	go func() {
		for event := range globals {
			global := event.(gowl.DisplayGlobal)
			switch strings.TrimSpace(global.Iface) {
			case "wl_compositor":
				w.display.Bind(global.Name, global.Iface, global.Version, w.compositor)
			case "wl_shm":
				w.display.Bind(global.Name, global.Iface, global.Version, w.shm)
			case "wl_shell":
				w.display.Bind(global.Name, global.Iface, global.Version, w.shell)
			case "wl_seat":
				w.display.Bind(global.Name, global.Iface, global.Version, w.seat)
				w.ddm.GetDataDevice(w.dd, w.seat)
				w.seat.GetPointer(w.pointer)
			case "wl_data_device_manager":
				w.display.Bind(global.Name, global.Iface, global.Version, w.ddm)
			}
		}
	}()
	w.display.AddGlobalListener(globals)

	// Iterate until we are sync'ed
	err = w.display.Iterate()
	if err != nil {
		w = nil
		return
	}
	waitForSync(w.display)

	// Create memory map
	w.screen = image.NewRGBA(image.Rect(0, 0, width, height))
	size := w.screen.Stride * w.screen.Rect.Dy()
	fd := gowl.CreateTmp(int64(size))
	mmap, err := syscall.Mmap(int(fd), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		w = nil
		return
	}
	w.screen.Pix = mmap

	// Create pool and buffer
	w.shm.CreatePool(w.pool, fd, int32(size))
	w.pool.CreateBuffer(w.buffer, 0, int32(width), int32(height), int32(w.screen.Stride), 1) // 1 = RGBA format
	w.pool.Destroy()

	// Ask compositor to create surface
	w.compositor.CreateSurface(w.surface)
	w.shell.GetShellSurface(w.shellsurface, w.surface)
	w.shellsurface.SetToplevel()

	// Make shell surface respond to pings
	pings := make(chan interface{})
	w.shellsurface.AddPingListener(pings)
	go func() {
		for p := range pings {
			ping := p.(gowl.ShellSurfacePing)
			w.shellsurface.Pong(ping.Serial)
		}
	}()

	go handleEvents(w)

	// Iterate
	go func () {
		for {
			w.display.Iterate()
		}
	} ()

	return
}

func (w *Window) SetTitle(title string) {
	w.shellsurface.SetTitle(title)
}

func (w *Window) SetSize(width, height int) {
}

func (w *Window) Size() (int, int) {
	return w.screen.Rect.Dx(), w.screen.Rect.Dy()
}

func (w *Window) Show() {
}

func (w *Window) Screen() draw.Image {
	return w.screen
}

func (w *Window) FlushImage(bounds ...image.Rectangle) {
	w.surface.Attach(w.buffer, 0, 0)

	for _, b := range bounds {
		w.surface.Damage(int32(b.Min.X), int32(b.Min.Y), int32(b.Dx()), int32(b.Dy()))
	}

	// Wait for redraw to finish
	cb := gowl.NewCallback()
	done := make(chan interface{})
	cb.AddDoneListener(done)
	w.surface.Frame(cb)
	func() {
		for {
			select {
			case <-done:
				return
			default:
				w.display.Iterate()
			}
		}
	}()
}

func (w *Window) EventChan() <-chan interface{} {
	return w.eventchan
}

func (w *Window) Close() error {
	return nil
}

func waitForSync(display *gowl.Display) {
	cb := gowl.NewCallback()
	done := make(chan interface{})
	cb.AddDoneListener(done)
	display.Sync(cb)
	func() {
		for {
			select {
			case <-done:
				return
			default:
				display.Iterate()
			}
		}
	}()
}
