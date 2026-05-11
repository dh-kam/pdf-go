package main

import (
	"fmt"
	"image"
	"image/color"
	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
)

func main(){
	src:=image.NewRGBA(image.Rect(0,0,16,16))
	for y:=0;y<16;y++{for x:=0;x<16;x++{src.SetRGBA(x,y,color.RGBA{255,0,0,255})}}
	c:=canvas.NewImageCanvas(image.Rect(0,0,8,8)).(*canvas.ImageCanvas)
	c.Transform([6]float64{8,0,0,8,0,0})
	if err:=c.DrawImageWithPhase(src,0,0,16,16,false,0.5,0.5); err!=nil {panic(err)}
	img:=c.Image().(*image.RGBA)
	nz:=0
	for i:=3;i<len(img.Pix);i+=4{ if img.Pix[i]!=0 {nz++}}
	fmt.Println("non-zero alpha", nz)
	for y:=0;y<8;y++{
		for x:=0;x<8;x++{fmt.Printf("%v", bool(img.RGBAAt(x,y).A!=0))}
		fmt.Println()
	}
}
