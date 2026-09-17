//go:build windows

package dashboard

import (
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"
)

var gdi = syscall.NewLazyDLL("gdi32.dll")
var user = syscall.NewLazyDLL("user32.dll")
var createDC = gdi.NewProc("CreateCompatibleDC")
var deleteDC = gdi.NewProc("DeleteDC")
var createDIB = gdi.NewProc("CreateDIBSection")
var selectObject = gdi.NewProc("SelectObject")
var deleteObject = gdi.NewProc("DeleteObject")
var createBrush = gdi.NewProc("CreateSolidBrush")
var fillRect = user.NewProc("FillRect")
var createFont = gdi.NewProc("CreateFontW")
var textOut = gdi.NewProc("TextOutW")
var setColor = gdi.NewProc("SetTextColor")
var setBkMode = gdi.NewProc("SetBkMode")
var flush = gdi.NewProc("GdiFlush")

type canvas struct {
	dc, bitmap, old uintptr
	bits            unsafe.Pointer
	w, h            int
	err             error
	fonts           map[int]uintptr
}

func newCanvas(w, h int) (*canvas, error) {
	// Memory DCs belong to their creating OS thread.
	runtime.LockOSThread()
	dc, _, e := createDC.Call(0)
	if dc == 0 {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("CreateCompatibleDC: %v", e)
	}
	c := &canvas{dc: dc, w: w, h: h, fonts: map[int]uintptr{}}
	var info struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		Used, Important        uint32
	}
	info.Size = 40
	info.Width = int32(w)
	info.Height = -int32(h)
	info.Planes = 1
	info.BitCount = 32
	c.bitmap, _, e = createDIB.Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&c.bits)), 0, 0)
	if c.bitmap == 0 {
		c.close()
		return nil, fmt.Errorf("CreateDIBSection: %v", e)
	}
	c.old, _, e = selectObject.Call(dc, c.bitmap)
	if c.old == 0 || c.old == ^uintptr(0) {
		c.close()
		return nil, fmt.Errorf("SelectObject: %v", e)
	}
	setBkMode.Call(dc, 1)
	return c, nil
}
func (c *canvas) close() {
	if c.old != 0 && c.old != ^uintptr(0) {
		selectObject.Call(c.dc, c.old)
	}
	for _, f := range c.fonts {
		deleteObject.Call(f)
	}
	if c.bitmap != 0 {
		deleteObject.Call(c.bitmap)
	}
	if c.dc != 0 {
		deleteDC.Call(c.dc)
	}
	runtime.UnlockOSThread()
}
func (c *canvas) rect(x, y, w, h int, col uint32) {
	if c.err != nil || w <= 0 || h <= 0 {
		return
	}
	brush, _, e := createBrush.Call(uintptr(col))
	if brush == 0 {
		c.err = fmt.Errorf("CreateSolidBrush: %v", e)
		return
	}
	defer deleteObject.Call(brush)
	rect := [4]int32{int32(x), int32(y), int32(x + w), int32(y + h)}
	if ok, _, e := fillRect.Call(c.dc, uintptr(unsafe.Pointer(&rect)), brush); ok == 0 {
		c.err = fmt.Errorf("FillRect: %v", e)
	}
}
func (c *canvas) text(x, y, size int, col uint32, text string) {
	if c.err != nil {
		return
	}
	font := c.fonts[size]
	if font == 0 {
		name, _ := syscall.UTF16PtrFromString("Segoe UI")
		var e error
		font, _, e = createFont.Call(uintptr(-size), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(name)))
		if font == 0 {
			c.err = fmt.Errorf("CreateFontW: %v", e)
			return
		}
		c.fonts[size] = font
	}
	old, _, e := selectObject.Call(c.dc, font)
	if old == 0 || old == ^uintptr(0) {
		c.err = fmt.Errorf("select font: %v", e)
		return
	}
	defer selectObject.Call(c.dc, old)
	setColor.Call(c.dc, uintptr(col))
	chars := syscall.StringToUTF16(text)
	if ok, _, e := textOut.Call(c.dc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)-1)); ok == 0 {
		c.err = fmt.Errorf("TextOutW: %v", e)
	}
}
func (c *canvas) image() (*image.RGBA, error) {
	if c.err != nil {
		return nil, c.err
	}
	flush.Call()
	src := unsafe.Slice((*byte)(c.bits), c.w*c.h*4)
	im := image.NewRGBA(image.Rect(0, 0, c.w, c.h))
	for i := 0; i < len(src); i += 4 {
		im.Pix[i] = src[i+2]
		im.Pix[i+1] = src[i+1]
		im.Pix[i+2] = src[i]
		im.Pix[i+3] = 255
	}
	return im, nil
}
